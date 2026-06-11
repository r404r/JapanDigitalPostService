package sync

import (
	"context"
	"errors"
	"log/slog"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"github.com/robfig/cron/v3"
)

// Scheduler 用进程内 cron 周期触发同步（适合单实例/小规模；多实例可改用
// cmd/batch + 外部调度器，二者共用同一 Engine 与 DB 锁）。
type Scheduler struct {
	engine *Engine
	cron   *cron.Cron
	logger *slog.Logger
}

// NewScheduler 按 cron 表达式（如 "0 3 * * *"）注册每日同步任务。
func NewScheduler(engine *Engine, spec string, logger *slog.Logger) (*Scheduler, error) {
	if logger == nil {
		logger = slog.Default()
	}
	c := cron.New()
	s := &Scheduler{engine: engine, cron: c, logger: logger}
	_, err := c.AddFunc(spec, func() {
		// auto：DB 空走 full，否则 diff。并发由 DB 锁兜底，重叠触发返回 sync_running。
		if _, err := engine.Run(context.Background(), domain.SyncAuto, domain.TriggerSchedule); err != nil {
			if errors.Is(err, domain.ErrSyncRunning) {
				logger.Info("scheduled sync skipped: already running")
				return
			}
			logger.Error("scheduled sync error", "err", err)
		}
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Start 启动调度（非阻塞）。
func (s *Scheduler) Start() { s.cron.Start() }

// Stop 停止调度并等待在跑任务结束。
func (s *Scheduler) Stop() context.Context { return s.cron.Stop() }
