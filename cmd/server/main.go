// Command server 启动 JapanDigitalPostService 的 HTTP API。
//
// 当前已装配：GET /v1/health（无需认证）、地址查询读路径（read scope）、
// 同步状态/历史/手动触发端点（/v1/sync/status、/v1/sync/runs 需 read，
// /v1/sync/trigger 需 admin）、token 管理端点（admin scope）、可选的应用层
// 载荷加密中间件、可选的进程内同步调度，以及优雅关闭。读路径、同步状态查询
// 与同步引擎共享同一 Store / 连接池。
//
// 鉴权：查询与同步状态端点经 auth.Service.RequireScope(read) 做真实 Bearer
// token 校验（缺失/无效/过期/吊销 → 401）；写端点（token 管理、手动触发）
// 要求 admin scope（见 spec §5.1）。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/app"
	"github.com/r404r/JapanDigitalPostService/internal/auth"
	"github.com/r404r/JapanDigitalPostService/internal/config"
	"github.com/r404r/JapanDigitalPostService/internal/crypto"
	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"github.com/r404r/JapanDigitalPostService/internal/query"
	"github.com/r404r/JapanDigitalPostService/internal/server"
	"github.com/r404r/JapanDigitalPostService/internal/store"
	syncpkg "github.com/r404r/JapanDigitalPostService/internal/sync"
	"github.com/r404r/JapanDigitalPostService/internal/version"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	cfg := config.Load()

	// 读路径与同步共享同一 Store；DB 打开失败即退出（查询 API 离开 DB 无意义）。
	st, err := store.Open(context.Background(), store.Options{
		Driver:         cfg.DBDriver,
		DSN:            cfg.DBDSN,
		ConnectTimeout: cfg.DBConnectTimeout,
		MaxRetry:       cfg.DBMaxRetry,
		RetryBackoff:   cfg.DBRetryBackoff,
	})
	if err != nil {
		logger.Error("database init failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()

	sqlDB, err := st.DB().DB()
	if err != nil {
		logger.Error("database handle failed", "err", err)
		os.Exit(1)
	}

	// 可选示例数据（默认关；本地开发用，避免示例行使 auto 同步误判为 diff）。
	if cfg.SeedSample {
		n, err := store.SeedSampleIfEmpty(context.Background(), sqlDB)
		if err != nil {
			logger.Error("seed sample data failed", "err", err)
			os.Exit(1)
		}
		if n > 0 {
			logger.Info("seeded sample addresses", "rows", n)
		}
	}

	// 同步引擎：与读路径共享同一 Store / 连接池。供手动触发端点
	// (/v1/sync/trigger) 与可选的进程内调度复用，保证单写者与行为一致。
	engine := app.BuildEngine(st, cfg, logger)

	// 可选的进程内同步调度：开启时按 SYNC_CRON 周期触发 auto 同步。
	if cfg.SyncSchedulerOn {
		if sch, err := syncpkg.NewScheduler(engine, cfg.SyncCron, logger); err != nil {
			logger.Error("sync scheduler disabled: bad SYNC_CRON", "spec", cfg.SyncCron, "err", err)
		} else {
			sch.Start()
			logger.Info("sync scheduler started", "spec", cfg.SyncCron)
			defer func() { <-sch.Stop().Done() }()
		}
	}

	// 可选载荷加密器（默认 none = 仅 TLS）。配置非法则启动失败，
	// 错误文案不含密钥本身。
	cipher, err := crypto.New(crypto.Mode(cfg.PayloadEncryption), cfg.PayloadEncKey, cfg.PayloadEncKeyID)
	if err != nil {
		logger.Error("init payload encryption", "err", err)
		os.Exit(1)
	}

	// 认证服务用持久化 token 仓储（task-0002 GORM store，GHO-34 接入）：进程重启后
	// token 不再丢失。EnsureBootstrap 仍按 hash 幂等注入引导 token。
	authSvc := auth.NewService(st.Tokens(), time.Now)
	if err := authSvc.EnsureBootstrap(context.Background(), cfg.AdminBootstrapToken); err != nil {
		logger.Error("bootstrap admin token", "err", err)
		os.Exit(1)
	}
	tokenHandlers := auth.NewHandlers(authSvc)

	// 查询读路径（health + /v1/addresses*）。数据端点套真实 Bearer 校验（read scope）；
	// health 始终免认证。
	reader := store.NewAddressReadRepo(sqlDB)
	svc := query.NewService(reader, cfg.FuzzyLimit, cfg.MaxTotal)
	router := server.NewRouter(server.Options{
		QueryService: svc,
		QueryTimeout: cfg.QueryTimeout,
		Logger:       logger,
		AuthMiddleware: func(next http.Handler) http.Handler {
			return authSvc.RequireScope(domain.ScopeRead, next)
		},
	})

	mux := http.NewServeMux()
	mux.Handle("GET /v1/health", router)
	mux.Handle("GET /v1/addresses", router)
	mux.Handle("GET /v1/addresses/{zipcode}", router)

	// 同步状态/历史/触发端点：status/runs 需 read，trigger 需 admin。
	syncHandlers := server.NewSyncHandlers(server.SyncOptions{
		Runs:         st.SyncRuns(),
		Reader:       reader,
		Runner:       engine,
		QueryTimeout: cfg.QueryTimeout,
		Logger:       logger,
	})
	mux.Handle("GET /v1/sync/status", authSvc.RequireScope(domain.ScopeRead, http.HandlerFunc(syncHandlers.GetStatus)))
	mux.Handle("GET /v1/sync/runs", authSvc.RequireScope(domain.ScopeRead, http.HandlerFunc(syncHandlers.ListRuns)))
	mux.Handle("POST /v1/sync/trigger", authSvc.RequireScope(domain.ScopeAdmin, http.HandlerFunc(syncHandlers.Trigger)))

	// Token 管理端点：均要求 admin scope。
	mux.Handle("POST /v1/tokens", authSvc.RequireScope(domain.ScopeAdmin, http.HandlerFunc(tokenHandlers.CreateToken)))
	mux.Handle("GET /v1/tokens", authSvc.RequireScope(domain.ScopeAdmin, http.HandlerFunc(tokenHandlers.ListTokens)))
	mux.Handle("DELETE /v1/tokens/{id}", authSvc.RequireScope(domain.ScopeAdmin, http.HandlerFunc(tokenHandlers.RevokeToken)))

	// 可选载荷加密对所有路由生效（none 时为零开销直通）。
	handler := cipher.Middleware(mux)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("server starting",
			"addr", cfg.HTTPAddr,
			"version", version.Version,
			"payload_encryption", cfg.PayloadEncryption,
			"bootstrap_admin", cfg.AdminBootstrapToken != "",
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	// 优雅关闭。
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Info("server shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
}
