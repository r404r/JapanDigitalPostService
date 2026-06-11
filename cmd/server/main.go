// Command server 启动 JapanDigitalPostService 的 HTTP API。
//
// 已装配：GET /v1/health（公开）、地址查询读路径（read 鉴权）、同步状态/历史
// （read 鉴权）与手动触发（admin 鉴权，异步执行）、token 管理端点（admin 鉴权）、
// 可选的应用层载荷加密中间件、可选的进程内同步调度，以及优雅关闭。读路径、同步
// 引擎与触发端点共享同一 Store / 连接池。
//
// 全部 /v1 路由经 internal/server.NewRouter 统一装配（与 internal/e2e 一致），
// 查询/同步端点已接入真实 Bearer 鉴权（spec §5.1），不再有占位放行中间件。
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
		Driver:             cfg.DBDriver,
		DSN:                cfg.DBDSN,
		ConnectTimeout:     cfg.DBConnectTimeout,
		MaxRetry:           cfg.DBMaxRetry,
		RetryBackoff:       cfg.DBRetryBackoff,
		MaxOpenConns:       cfg.DBMaxOpenConns,
		MaxIdleConns:       cfg.DBMaxIdleConns,
		ConnMaxLifetime:    cfg.DBConnMaxLifetime,
		LockReleaseTimeout: cfg.SyncLockReleaseTimeout,
	})
	if err != nil {
		logger.Error("database init failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()
	if err := app.CleanupStaleRunningSyncs(context.Background(), st, logger); err != nil {
		logger.Error("cleanup stale sync runs failed", "err", err)
		os.Exit(1)
	}

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

	// 同步引擎：手动触发端点（POST /v1/sync/trigger）与进程内调度共用同一引擎，
	// 与读路径共享同一 Store / 连接池。无论调度是否开启都需构造，供触发端点使用。
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

	// 统一装配全部 /v1 路由：health（公开）+ 查询（read）+ 同步状态/历史（read）+
	// 手动触发（admin）+ token 管理（admin）。鉴权与 token 处理器经 Options 注入，
	// 保持 server 包与 auth 包解耦；与 internal/e2e 共用同一装配入口。
	reader := store.NewAddressReadRepoForDriver(sqlDB, cfg.DBDriver)
	svc := query.NewService(reader, cfg.FuzzyLimit, cfg.MaxTotal)
	// 管理画面运行时设置：与同步引擎共享同一基线默认值，DB 覆盖值持久化、重启后保留。
	settingsAPI := settingsAdapter{svc: app.BuildSettings(st, cfg)}
	router := server.NewRouter(server.Options{
		QueryService:  svc,
		QueryTimeout:  cfg.QueryTimeout,
		Logger:        logger,
		AddressReader: reader,
		SyncRuns:      st.SyncRuns(),
		SyncTrigger:   engine,
		SyncUploader:  engine,
		Auth:          authSvc,
		TokenHandlers: tokenHandlers,
		Settings:      settingsAPI,
	})

	var rootHandler http.Handler = router
	if cfg.StaticDir != "" {
		rootHandler = server.WithStaticFiles(router, cfg.StaticDir)
	}

	// 可选载荷加密对所有路由生效（none 时为零开销直通）。
	handler := cipher.Middleware(rootHandler)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
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
	syncCtx, syncCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer syncCancel()
	if err := engine.Shutdown(syncCtx); err != nil {
		logger.Error("sync engine shutdown failed", "err", err)
		os.Exit(1)
	}
}
