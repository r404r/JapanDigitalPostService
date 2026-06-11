// Command server 启动 JapanDigitalPostService 的 HTTP API。
//
// 当前提供 GET /v1/health、优雅关闭，以及可选的进程内同步调度（task-0004）。
// 业务路由（查询/同步状态/token）由 task-0005/0006/0008 在 internal/server 装配后接入。
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

	"github.com/r404r/JapanDigitalPostService/internal/app"
	"github.com/r404r/JapanDigitalPostService/internal/config"
	syncpkg "github.com/r404r/JapanDigitalPostService/internal/sync"
	"github.com/r404r/JapanDigitalPostService/internal/version"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	// 可选的进程内同步调度：开启时按 SYNC_CRON 周期触发 auto 同步。DB 初始化失败
	// 不阻断健康检查（读写解耦），仅记录并跳过调度。
	if cfg.SyncSchedulerOn {
		if a, err := app.BuildSync(context.Background(), cfg, logger); err != nil {
			logger.Error("sync scheduler disabled: build failed", "err", err)
		} else if sch, err := syncpkg.NewScheduler(a.Engine, cfg.SyncCron, logger); err != nil {
			logger.Error("sync scheduler disabled: bad SYNC_CRON", "spec", cfg.SyncCron, "err", err)
			_ = a.Close()
		} else {
			sch.Start()
			logger.Info("sync scheduler started", "spec", cfg.SyncCron)
			defer func() { <-sch.Stop().Done(); _ = a.Close() }()
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": version.Version,
		})
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("server starting", "addr", cfg.HTTPAddr, "version", version.Version)
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
