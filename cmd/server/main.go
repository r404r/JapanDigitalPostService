// Command server 启动 JapanDigitalPostService 的 HTTP API。
//
// 当前已接入地址查询读路径（task-0005）：/v1/health 与 /v1/addresses[/{zipcode}]。
// 同步（task-0004）、认证中间件（task-0006）、token 管理等端点在后续 task 接入。
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/config"
	"github.com/r404r/JapanDigitalPostService/internal/query"
	"github.com/r404r/JapanDigitalPostService/internal/server"
	"github.com/r404r/JapanDigitalPostService/internal/store"
	"github.com/r404r/JapanDigitalPostService/internal/version"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	db, err := openDB(logger, cfg)
	if err != nil {
		logger.Error("database init failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	repo := store.NewAddressRepo(db)
	svc := query.NewService(repo, cfg.FuzzyLimit, cfg.MaxTotal)

	router := server.NewRouter(server.Options{
		QueryService: svc,
		QueryTimeout: cfg.QueryTimeout,
		Logger:       logger,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
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

// openDB 打开数据库、执行迁移并按配置可选地播种示例数据。
// 当前仅 sqlite 驱动已实现；postgres/mysql 由 task-0002 接入。
func openDB(logger *slog.Logger, cfg config.Config) (*sql.DB, error) {
	if cfg.DBDriver != "sqlite" {
		return nil, errors.New("DB_DRIVER=" + cfg.DBDriver + " not yet supported (task-0002 adds postgres/mysql); use sqlite")
	}
	db, err := store.OpenSQLite(cfg.DBDSN)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if cfg.SeedSample {
		n, err := store.SeedSampleIfEmpty(ctx, db)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		if n > 0 {
			logger.Info("seeded sample addresses", "rows", n)
		}
	}
	return db, nil
}
