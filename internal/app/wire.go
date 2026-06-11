// Package app 组装跨入口（cmd/server 进程内调度、cmd/batch 独立批处理）复用的
// 装配逻辑，确保二者共享同一 Store / Engine 配置与 DB 锁。
package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/config"
	"github.com/r404r/JapanDigitalPostService/internal/settings"
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
		return nil, err
	}
	if err := CleanupStaleRunningSyncs(ctx, st, logger); err != nil {
		_ = st.Close()
		return nil, err
	}
	return &SyncApp{Store: st, Engine: BuildEngine(st, cfg, logger)}, nil
}

// CleanupStaleRunningSyncs 将上个进程遗留的 running 同步记录标记为 failed。
// DB 锁有 TTL 可恢复写入互斥；sync_runs 也需要在新进程启动时收敛到终态，
// 避免状态 API 与管理画面长期显示“运行中”。
func CleanupStaleRunningSyncs(ctx context.Context, st *store.Store, logger *slog.Logger) error {
	const message = "process stopped before sync completed"
	n, err := st.SyncRuns().MarkRunningFailed(ctx, message, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("cleanup stale running sync runs: %w", err)
	}
	if n > 0 {
		if logger == nil {
			logger = slog.Default()
		}
		logger.Warn("stale running sync runs marked failed", "count", n)
	}
	return nil
}

// BuildSettings 构造运行时设置服务：基线默认值取自配置（env 优先，缺省落到代码
// 常量），DB 覆盖值由 store 持久化。同步引擎与管理 API 共享同一服务，保证
// 全量 URL / 下载重试的有效值在两处一致。
func BuildSettings(st *store.Store, cfg config.Config) *settings.Service {
	return settings.NewService(st.Settings(), settings.Defaults{
		ScrapeFullURL:    cfg.SyncFullURL,
		DownloadMaxRetry: cfg.DownloadMaxRetry,
	}, time.Now)
}

// BuildEngine 在一个已打开的 Store 上构造同步 Engine，供与读路径共享同一
// 连接池的入口（cmd/server）复用。引擎绑定运行时设置解析器，使管理画面配置的
// 全量 URL / 下载重试在每次同步前生效（batch / 手动触发 / 进程内调度三路径一致）。
func BuildEngine(st *store.Store, cfg config.Config, logger *slog.Logger) *syncpkg.Engine {
	fetcher := syncpkg.NewHTTPFetcher(cfg.DownloadTimeout, cfg.DownloadMaxRetry, cfg.DownloadBackoff, logger)
	eng := syncpkg.NewEngine(st.Addresses(), st.SyncRuns(), st.Locker(), fetcher, syncpkg.Options{
		FullURL:            cfg.SyncFullURL,
		AddURLTemplate:     cfg.SyncAddURLTemplate,
		DelURLTemplate:     cfg.SyncDelURLTemplate,
		BatchSize:          cfg.SyncBatchSize,
		FullPrune:          cfg.SyncFullPrune,
		FullMinRows:        int64(cfg.SyncFullMinRows),
		DiffFallbackFull:   cfg.SyncDiffFallback,
		DiffLookbackMonths: cfg.SyncDiffLookback,
	}, logger)
	return eng.UseSettingsResolver(BuildSettings(st, cfg))
}
