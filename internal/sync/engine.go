package sync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// Options 是同步引擎的行为参数（来自 internal/config，docs/architecture.md §9）。
type Options struct {
	FullURL            string // 全量 zip URL
	AddURLTemplate     string // 差分新增 URL 模板，含一个 %s 占位 YYMM
	DelURLTemplate     string // 差分废止 URL 模板，含一个 %s 占位 YYMM
	BatchSize          int    // upsert 批大小
	FullPrune          bool   // 全量后剪除已消失地址
	FullMinRows        int64  // 全量剪枝安全下限
	DiffFallbackFull   bool   // 差分窗口内无可用文件时回退全量
	DiffLookbackMonths int    // 每次差分回看的月份窗口（含当月）
}

// Engine 编排一次端到端同步：判定 full/diff → 下载 → 解析 → 幂等应用 →
// 写 sync_runs → 并发锁保护。
type Engine struct {
	addresses domain.AddressRepository
	runs      domain.SyncRunRepository
	locker    domain.Locker
	fetcher   Fetcher
	opt       Options
	logger    *slog.Logger

	// now / newID 可注入，便于测试确定性。
	now   func() time.Time
	newID func() string
}

// NewEngine 构造引擎。
func NewEngine(addresses domain.AddressRepository, runs domain.SyncRunRepository, locker domain.Locker, fetcher Fetcher, opt Options, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	if opt.BatchSize <= 0 {
		opt.BatchSize = 1000
	}
	if opt.DiffLookbackMonths <= 0 {
		opt.DiffLookbackMonths = 3
	}
	return &Engine{
		addresses: addresses,
		runs:      runs,
		locker:    locker,
		fetcher:   fetcher,
		opt:       opt,
		logger:    logger,
		now:       time.Now,
		newID:     func() string { return uuid.NewString() },
	}
}

// Run 执行一次同步。reqType 为 auto/full/diff；trigger 区分调度/手动。并发触发
// 返回 domain.ErrSyncRunning。返回写入的 SyncRun（含计数与状态）。
func (e *Engine) Run(ctx context.Context, reqType domain.SyncType, trigger domain.SyncTrigger) (*domain.SyncRun, error) {
	holder := e.holder()
	release, ok, err := e.locker.Acquire(ctx, holder)
	if err != nil {
		return nil, fmt.Errorf("acquire sync lock: %w", err)
	}
	if !ok {
		return nil, domain.ErrSyncRunning
	}
	defer func() {
		if rerr := release(); rerr != nil {
			e.logger.Error("release sync lock", "err", rerr)
		}
	}()

	// 判定同步类型。
	syncType := reqType
	if syncType == domain.SyncAuto || syncType == "" {
		count, cerr := e.addresses.Count(ctx)
		if cerr != nil {
			return nil, fmt.Errorf("count addresses: %w", cerr)
		}
		if count == 0 {
			syncType = domain.SyncFull
		} else {
			syncType = domain.SyncDiff
		}
	}

	start := e.now()
	run := &domain.SyncRun{
		ID:        e.newID(),
		Type:      syncType,
		Status:    domain.StatusRunning,
		Trigger:   trigger,
		StartedAt: start,
	}
	if err := e.runs.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("create sync run: %w", err)
	}
	e.logger.Info("sync started", "run_id", run.ID, "type", syncType, "trigger", trigger)

	var res ApplyResult
	switch syncType {
	case domain.SyncFull:
		res, err = e.runFull(ctx, run)
	case domain.SyncDiff:
		res, err = e.runDiff(ctx, run)
	default:
		err = fmt.Errorf("unknown sync type %q", syncType)
	}

	finished := e.now()
	run.FinishedAt = &finished
	run.DurationMs = finished.Sub(start).Milliseconds()
	run.RowsAdded = res.Added
	run.RowsUpdated = res.Updated
	run.RowsDeleted = res.Deleted
	run.RowsTotal = res.Total
	if err != nil {
		run.Status = domain.StatusFailed
		run.ErrorMessage = err.Error()
		e.logger.Error("sync failed", "run_id", run.ID, "type", run.Type, "err", err)
	} else {
		run.Status = domain.StatusSuccess
		e.logger.Info("sync succeeded", "run_id", run.ID, "type", run.Type,
			"added", res.Added, "updated", res.Updated, "deleted", res.Deleted, "total", res.Total)
	}
	if uerr := e.runs.Update(ctx, run); uerr != nil {
		e.logger.Error("update sync run", "run_id", run.ID, "err", uerr)
		if err == nil {
			err = uerr
		}
	}
	return run, err
}

func (e *Engine) runFull(ctx context.Context, run *domain.SyncRun) (ApplyResult, error) {
	src, err := e.fetcher.Fetch(ctx, e.opt.FullURL)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("download full: %w", err)
	}
	defer src.CSV.Close()
	run.SourceURL = src.URL
	run.FileChecksum = src.Checksum
	run.FileSize = src.Size
	return ApplyFull(ctx, e.addresses, src.CSV, e.opt.BatchSize, e.opt.FullPrune, e.opt.FullMinRows)
}

func (e *Engine) runDiff(ctx context.Context, run *domain.SyncRun) (ApplyResult, error) {
	months := monthsWindow(e.now(), e.opt.DiffLookbackMonths)
	var agg ApplyResult
	applied := 0
	var lastPeriod, lastChecksum, lastURL string
	var lastSize int64

	for _, ym := range months { // oldest → newest，按时序应用保证改名顺序正确
		addURL := fmt.Sprintf(e.opt.AddURLTemplate, ym)
		delURL := fmt.Sprintf(e.opt.DelURLTemplate, ym)

		addSrc, addErr := e.fetcher.Fetch(ctx, addURL)
		if addErr != nil && !errors.Is(addErr, ErrSourceNotFound) {
			return agg, fmt.Errorf("download add %s: %w", ym, addErr)
		}
		delSrc, delErr := e.fetcher.Fetch(ctx, delURL)
		if delErr != nil && !errors.Is(delErr, ErrSourceNotFound) {
			if addSrc != nil {
				addSrc.CSV.Close()
			}
			return agg, fmt.Errorf("download del %s: %w", ym, delErr)
		}
		if addSrc == nil && delSrc == nil {
			continue // 该月差分未发布，跳过
		}

		var addCSV, delCSV io.Reader
		if addSrc != nil {
			addCSV = addSrc.CSV
			lastChecksum, lastURL, lastSize = addSrc.Checksum, addSrc.URL, addSrc.Size
		}
		if delSrc != nil {
			delCSV = delSrc.CSV
		}

		res, aerr := ApplyDiff(ctx, e.addresses, addCSV, delCSV, e.opt.BatchSize)
		if addSrc != nil {
			addSrc.CSV.Close()
		}
		if delSrc != nil {
			delSrc.CSV.Close()
		}
		if aerr != nil {
			return agg, fmt.Errorf("apply diff %s: %w", ym, aerr)
		}
		agg.Added += res.Added
		agg.Updated += res.Updated
		agg.Deleted += res.Deleted
		agg.Total += res.Total
		applied++
		lastPeriod = ym
	}

	if applied == 0 {
		if e.opt.DiffFallbackFull {
			e.logger.Warn("no diff available in window, falling back to full", "months", months)
			run.Type = domain.SyncFull
			return e.runFull(ctx, run)
		}
		return agg, fmt.Errorf("no diff source available for months %v (fallback disabled)", months)
	}

	run.DiffPeriod = lastPeriod
	run.SourceURL = lastURL
	run.FileChecksum = lastChecksum
	run.FileSize = lastSize
	return agg, nil
}

func (e *Engine) holder() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("%s/%s", host, e.newID())
}

// monthsWindow 返回从 (now - (n-1) 月) 到 now 的 YYMM 列表，按时间升序。
//
// 月份回退先归一到当月 1 日再做 AddDate：直接对 29–31 日做 AddDate(0,-i,0) 会因
// 短月归一化跳月/重复（如 2026-03-31 回退 1 月得 03-03，漏掉 2602），归一到月初可
// 规避所有月末与闰月边界问题。
func monthsWindow(now time.Time, n int) []string {
	if n <= 0 {
		n = 1
	}
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	out := make([]string, 0, n)
	for i := n - 1; i >= 0; i-- {
		m := first.AddDate(0, -i, 0)
		out = append(out, fmt.Sprintf("%02d%02d", m.Year()%100, int(m.Month())))
	}
	return out
}
