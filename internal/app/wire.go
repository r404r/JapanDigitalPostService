// Package app 组装跨入口（cmd/server 进程内调度、cmd/batch 独立批处理）复用的
// 装配逻辑，确保二者共享同一 Store / Engine 配置与 DB 锁。
package app

import (
	"context"
	"log/slog"

	"github.com/r404r/JapanDigitalPostService/internal/config"
	"github.com/r404r/JapanDigitalPostService/internal/store"
	syncpkg "github.com/r404r/JapanDigitalPostService/internal/sync"
)

// SyncApp 持有同步引擎及其底层 Store。
type SyncApp struct {
	Store  *store.Store
	Engine *syncpkg.Engine
}

// Close 释放底层资源。
func (a *SyncApp) Close() error { return a.Store.Close() }

// BuildSync 由配置打开 Store 并构造同步 Engine。
func BuildSync(ctx context.Context, cfg config.Config, logger *slog.Logger) (*SyncApp, error) {
	st, err := store.Open(ctx, store.Options{
		Driver:         cfg.DBDriver,
		DSN:            cfg.DBDSN,
		ConnectTimeout: cfg.DBConnectTimeout,
		MaxRetry:       cfg.DBMaxRetry,
		RetryBackoff:   cfg.DBRetryBackoff,
	})
	if err != nil {
		return nil, err
	}
	return &SyncApp{Store: st, Engine: BuildEngine(st, cfg, logger)}, nil
}

// BuildEngine 在一个已打开的 Store 上构造同步 Engine，供与读路径共享同一
// 连接池的入口（cmd/server）复用。
func BuildEngine(st *store.Store, cfg config.Config, logger *slog.Logger) *syncpkg.Engine {
	fetcher := syncpkg.NewHTTPFetcher(cfg.DownloadTimeout, cfg.DownloadMaxRetry, cfg.DownloadBackoff, logger)
	return syncpkg.NewEngine(st.Addresses(), st.SyncRuns(), st.Locker(), fetcher, syncpkg.Options{
		FullURL:            cfg.SyncFullURL,
		AddURLTemplate:     cfg.SyncAddURLTemplate,
		DelURLTemplate:     cfg.SyncDelURLTemplate,
		BatchSize:          cfg.SyncBatchSize,
		FullPrune:          cfg.SyncFullPrune,
		FullMinRows:        int64(cfg.SyncFullMinRows),
		DiffFallbackFull:   cfg.SyncDiffFallback,
		DiffLookbackMonths: cfg.SyncDiffLookback,
	}, logger)
}
