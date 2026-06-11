// Command server 启动 JapanDigitalPostService 的 HTTP API。
//
// 当前已装配：GET /v1/health（无需认证）、token 管理端点（admin scope）、
// Bearer 认证中间件、可选的应用层载荷加密中间件，以及优雅关闭。
// 查询/同步路由由 task-0005/0008 在 internal/server 装配后接入。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/auth"
	"github.com/r404r/JapanDigitalPostService/internal/config"
	"github.com/r404r/JapanDigitalPostService/internal/crypto"
	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"github.com/r404r/JapanDigitalPostService/internal/version"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	cfg := config.Load()

	// 可选载荷加密器（默认 none = 仅 TLS）。配置非法则启动失败，
	// 错误文案不含密钥本身。
	cipher, err := crypto.New(crypto.Mode(cfg.PayloadEncryption), cfg.PayloadEncKey, cfg.PayloadEncKeyID)
	if err != nil {
		logger.Error("init payload encryption", "err", err)
		os.Exit(1)
	}

	// 认证服务。当前用进程内 token 仓储（默认实现），待 task-0002 的 GORM
	// store 落地同一 domain.TokenRepository 接口后替换。
	authSvc := auth.NewService(auth.NewMemoryStore(), time.Now)
	if err := authSvc.EnsureBootstrap(context.Background(), cfg.AdminBootstrapToken); err != nil {
		logger.Error("bootstrap admin token", "err", err)
		os.Exit(1)
	}
	tokenHandlers := auth.NewHandlers(authSvc)

	mux := http.NewServeMux()

	// 健康检查：无需认证。
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": version.Version,
		})
	})

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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
