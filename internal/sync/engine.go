package sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	stdsync "sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

const maxUploadCSVBytes int64 = 128 << 20

var (
	ErrUnsupportedUploadFile = errors.New("unsupported upload file type")
	ErrUploadCSVTooLarge     = errors.New("uploaded csv too large")
	ErrUploadEncoding        = errors.New("uploaded csv must be UTF-8")
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

// SettingsResolver 解析每次同步运行前的有效配置（DB 覆盖 > env > 默认）。由
// internal/settings.Service 实现并注入，使管理画面配置的全量 URL / 下载重试次数
// 无需重启即可在 batch / 手动触发 / 上传等路径生效，避免启动期把配置冻死。
type SettingsResolver interface {
	ResolveSyncSettings(ctx context.Context) (domain.EffectiveSyncSettings, error)
}

// retryConfigurable 是 Fetcher 的可选能力：运行时可重置下载重试次数（HTTPFetcher
// 实现）。引擎在每次运行前据有效配置注入；测试用的 fake fetcher 未实现，则不受影响。
type retryConfigurable interface {
	SetMaxRetry(n int)
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

	// resolver 可选；非空时在每次运行前解析有效配置（全量 URL / 下载重试），
	// 为 nil 时回退到构造时的静态 opt / fetcher 默认值（测试与降级路径）。
	resolver SettingsResolver

	// now / newID 可注入，便于测试确定性。
	now   func() time.Time
	newID func() string

	asyncCtx    context.Context
	asyncCancel context.CancelFunc
	asyncWG     stdsync.WaitGroup
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
	asyncCtx, asyncCancel := context.WithCancel(context.Background())
	return &Engine{
		addresses:   addresses,
		runs:        runs,
		locker:      locker,
		fetcher:     fetcher,
		opt:         opt,
		logger:      logger,
		now:         time.Now,
		newID:       func() string { return uuid.NewString() },
		asyncCtx:    asyncCtx,
		asyncCancel: asyncCancel,
	}
}

// UseSettingsResolver 注入运行时配置解析器（管理画面可配的全量 URL / 下载重试）。
// 注入后，引擎在每次同步运行前解析有效配置，无需重启即生效。返回 e 便于链式装配。
func (e *Engine) UseSettingsResolver(r SettingsResolver) *Engine {
	e.resolver = r
	return e
}

// Run 执行一次同步（同步阻塞至完成）。reqType 为 auto/full/diff；trigger 区分
// 调度/手动。并发触发返回 domain.ErrSyncRunning。返回写入的 SyncRun（含计数与
// 状态）。供 cmd/batch 与进程内调度等可阻塞等待的调用方使用。
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

	syncType, err := e.resolveType(ctx, reqType)
	if err != nil {
		return nil, err
	}
	run, start, err := e.beginRun(ctx, syncType, trigger)
	if err != nil {
		return nil, err
	}
	return run, e.execute(ctx, run, syncType, start)
}

// TriggerAsync 异步触发一次同步：同步地获取锁、判定类型并创建 sync_run 记录，
// 然后在后台 goroutine 中执行下载/解析/应用并落库——立即返回创建的运行记录
// （status=running），供 HTTP 触发端点在不占住请求（全量可达分钟级）的前提下
// 回传 run id。已有同步在运行时返回 domain.ErrSyncRunning（映射到 409）。
//
// 后台执行不受触发请求生命周期影响，但受 Engine.Shutdown 取消与等待；锁在后台
// 执行结束后释放。返回值是创建时刻的快照，与后台 goroutine 各持一份，避免并发读写。
func (e *Engine) TriggerAsync(reqType domain.SyncType, trigger domain.SyncTrigger) (*domain.SyncRun, error) {
	ctx := e.asyncCtx
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	holder := e.holder()
	release, ok, err := e.locker.Acquire(ctx, holder)
	if err != nil {
		return nil, fmt.Errorf("acquire sync lock: %w", err)
	}
	if !ok {
		return nil, domain.ErrSyncRunning
	}

	releaseOnce := func() {
		if rerr := release(); rerr != nil {
			e.logger.Error("release sync lock", "err", rerr)
		}
	}

	syncType, err := e.resolveType(ctx, reqType)
	if err != nil {
		releaseOnce()
		return nil, err
	}
	run, start, err := e.beginRun(ctx, syncType, trigger)
	if err != nil {
		releaseOnce()
		return nil, err
	}

	snapshot := *run // 创建时刻快照，返回给调用方；后台 goroutine 持原指针，二者互不共享。
	e.asyncWG.Add(1)
	go func() {
		defer e.asyncWG.Done()
		defer releaseOnce()
		_ = e.execute(ctx, run, syncType, start)
	}()
	return &snapshot, nil
}

// UploadFull applies an uploaded Japan Post utf_ken_all CSV as a full rebuild.
// It shares the same DB lock and sync_runs lifecycle as scheduled/manual sync.
func (e *Engine) UploadFull(ctx context.Context, filename string, data []byte) (*domain.SyncRun, error) {
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

	run, start, err := e.beginRun(ctx, domain.SyncFull, domain.TriggerUpload)
	if err != nil {
		return nil, err
	}
	run.SourceURL = "upload:" + filename
	run.FileSize = int64(len(data))
	sum := sha256.Sum256(data)
	run.FileChecksum = hex.EncodeToString(sum[:])

	csv, err := uploadCSV(filename, data)
	if err != nil {
		return run, e.finishUpload(ctx, run, start, ApplyResult{}, err)
	}
	return run, e.executeUpload(ctx, run, start, bytes.NewReader(csv))
}

// Shutdown 取消 TriggerAsync 启动的后台同步任务，并等待它们退出。用于 server
// 优雅关闭；cmd/batch 的同步 Run 由调用方传入的 context 控制，不归这里管理。
func (e *Engine) Shutdown(ctx context.Context) error {
	e.asyncCancel()
	done := make(chan struct{})
	go func() {
		e.asyncWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// resolveType 把 auto/空 判定为 full（库空）或 diff（库非空）；显式 full/diff 原样返回。
func (e *Engine) resolveType(ctx context.Context, reqType domain.SyncType) (domain.SyncType, error) {
	syncType := reqType
	if syncType == domain.SyncAuto || syncType == "" {
		count, cerr := e.addresses.Count(ctx)
		if cerr != nil {
			return "", fmt.Errorf("count addresses: %w", cerr)
		}
		if count == 0 {
			syncType = domain.SyncFull
		} else {
			syncType = domain.SyncDiff
		}
	}
	return syncType, nil
}

// beginRun 创建一条 status=running 的运行记录并返回（含起始时刻）。调用方须已持锁。
func (e *Engine) beginRun(ctx context.Context, syncType domain.SyncType, trigger domain.SyncTrigger) (*domain.SyncRun, time.Time, error) {
	start := e.now()
	run := &domain.SyncRun{
		ID:        e.newID(),
		Type:      syncType,
		Status:    domain.StatusRunning,
		Trigger:   trigger,
		StartedAt: start,
	}
	if err := e.runs.Create(ctx, run); err != nil {
		return nil, start, fmt.Errorf("create sync run: %w", err)
	}
	return run, start, nil
}

// execute 执行同步主体（下载/解析/应用）并把最终状态与计数落库。调用方须已持锁，
// 并在完成后释放锁。
func (e *Engine) execute(ctx context.Context, run *domain.SyncRun, syncType domain.SyncType, start time.Time) error {
	e.logger.Info("sync started", "run_id", run.ID, "type", syncType, "trigger", run.Trigger)

	// 每次运行前解析有效配置（DB 覆盖 > env > 默认），避免启动期把 URL / 重试冻死。
	settings := e.resolveRuntime(ctx)
	applyOpt, optErr := applyOptions(settings.TownSkipRegex, "")

	var res ApplyResult
	var err error = optErr
	switch syncType {
	case domain.SyncFull:
		if err == nil {
			res, err = e.runFull(ctx, run, settings.FullURL, withSourceType(applyOpt, "full"))
		}
	case domain.SyncDiff:
		if err == nil {
			res, err = e.runDiff(ctx, run, settings.FullURL, applyOpt)
		}
	default:
		err = fmt.Errorf("unknown sync type %q", syncType)
	}

	finished := e.now()
	run.FinishedAt = &finished
	run.DurationMs = finished.Sub(start).Milliseconds()
	run.RowsAdded = res.Added
	run.RowsUpdated = res.Updated
	run.RowsDeleted = res.Deleted
	run.RowsSkipped = res.Skipped
	run.RowsTotal = res.Total
	persistCtx, cancel := persistContext(ctx)
	defer cancel()
	e.attachRunIDAndTime(run.ID, finished, res.SkippedRows)
	if serr := e.runs.CreateSkippedRows(persistCtx, res.SkippedRows); serr != nil {
		e.logger.Error("create skipped rows", "run_id", run.ID, "err", serr)
		if err == nil {
			err = serr
		}
	}
	if err != nil {
		run.Status = domain.StatusFailed
		run.ErrorMessage = err.Error()
		e.logger.Error("sync failed", "run_id", run.ID, "type", run.Type, "err", err)
	} else {
		run.Status = domain.StatusSuccess
		e.logger.Info("sync succeeded", "run_id", run.ID, "type", run.Type,
			"added", res.Added, "updated", res.Updated, "deleted", res.Deleted, "skipped", res.Skipped, "total", res.Total)
	}
	if uerr := e.runs.Update(persistCtx, run); uerr != nil {
		e.logger.Error("update sync run", "run_id", run.ID, "err", uerr)
		if err == nil {
			err = uerr
		}
	}
	return err
}

type runtimeSyncSettings struct {
	FullURL       string
	TownSkipRegex string
}

// resolveRuntime 解析本次运行的有效全量 URL，并把有效下载重试次数注入 fetcher。
// 无 resolver（测试/降级）或解析失败时，回退到构造时的静态配置，且不打断同步。
func (e *Engine) resolveRuntime(ctx context.Context) runtimeSyncSettings {
	out := runtimeSyncSettings{FullURL: e.opt.FullURL}
	if e.resolver == nil {
		return out
	}
	eff, err := e.resolver.ResolveSyncSettings(ctx)
	if err != nil {
		e.logger.Warn("resolve runtime settings failed; using static config", "err", err)
		return out
	}
	if eff.ScrapeFullURL != "" {
		out.FullURL = eff.ScrapeFullURL
	}
	out.TownSkipRegex = strings.TrimSpace(eff.TownSkipRegex)
	if rc, ok := e.fetcher.(retryConfigurable); ok {
		rc.SetMaxRetry(eff.DownloadMaxRetry)
	}
	return out
}

func (e *Engine) executeUpload(ctx context.Context, run *domain.SyncRun, start time.Time, csv io.Reader) error {
	e.logger.Info("sync upload started", "run_id", run.ID, "source", run.SourceURL)

	settings := e.resolveRuntime(ctx)
	applyOpt, err := applyOptions(settings.TownSkipRegex, "full")
	var res ApplyResult
	if err == nil {
		res, err = ApplyFullWithOptions(ctx, e.addresses, csv, e.opt.BatchSize, e.opt.FullPrune, e.opt.FullMinRows, applyOpt)
	}
	return e.finishUpload(ctx, run, start, res, err)
}

func (e *Engine) finishUpload(ctx context.Context, run *domain.SyncRun, start time.Time, res ApplyResult, err error) error {
	finished := e.now()
	run.FinishedAt = &finished
	run.DurationMs = finished.Sub(start).Milliseconds()
	run.RowsAdded = res.Added
	run.RowsUpdated = res.Updated
	run.RowsDeleted = res.Deleted
	run.RowsSkipped = res.Skipped
	run.RowsTotal = res.Total
	persistCtx, cancel := persistContext(ctx)
	defer cancel()
	e.attachRunIDAndTime(run.ID, finished, res.SkippedRows)
	if serr := e.runs.CreateSkippedRows(persistCtx, res.SkippedRows); serr != nil {
		e.logger.Error("create skipped rows", "run_id", run.ID, "err", serr)
		if err == nil {
			err = serr
		}
	}
	if err != nil {
		run.Status = domain.StatusFailed
		run.ErrorMessage = err.Error()
		e.logger.Error("sync upload failed", "run_id", run.ID, "err", err)
	} else {
		run.Status = domain.StatusSuccess
		e.logger.Info("sync upload succeeded", "run_id", run.ID,
			"added", res.Added, "updated", res.Updated, "deleted", res.Deleted, "skipped", res.Skipped, "total", res.Total)
	}
	if uerr := e.runs.Update(persistCtx, run); uerr != nil {
		e.logger.Error("update sync upload run", "run_id", run.ID, "err", uerr)
		if err == nil {
			err = uerr
		}
	}
	return err
}

func uploadCSV(filename string, data []byte) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".csv":
		if int64(len(data)) > maxUploadCSVBytes {
			return nil, ErrUploadCSVTooLarge
		}
		if !utf8.Valid(data) {
			return nil, ErrUploadEncoding
		}
		return data, nil
	case ".zip":
		rc, err := openZipCSVWithLimit(data, maxUploadCSVBytes)
		if err != nil {
			return nil, fmt.Errorf("open uploaded zip: %w", err)
		}
		defer rc.Close()
		csv, err := io.ReadAll(io.LimitReader(rc, maxUploadCSVBytes+1))
		if err != nil {
			return nil, fmt.Errorf("read uploaded csv: %w", err)
		}
		if int64(len(csv)) > maxUploadCSVBytes {
			return nil, ErrUploadCSVTooLarge
		}
		if !utf8.Valid(csv) {
			return nil, ErrUploadEncoding
		}
		return csv, nil
	default:
		return nil, ErrUnsupportedUploadFile
	}
}

func (e *Engine) runFull(ctx context.Context, run *domain.SyncRun, fullURL string, opt ApplyOptions) (ApplyResult, error) {
	src, err := e.fetcher.Fetch(ctx, fullURL)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("download full: %w", err)
	}
	defer src.CSV.Close()
	run.SourceURL = src.URL
	run.FileChecksum = src.Checksum
	run.FileSize = src.Size
	return ApplyFullWithOptions(ctx, e.addresses, src.CSV, e.opt.BatchSize, e.opt.FullPrune, e.opt.FullMinRows, withSourceType(opt, "full"))
}

func (e *Engine) runDiff(ctx context.Context, run *domain.SyncRun, fullURL string, opt ApplyOptions) (ApplyResult, error) {
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

		res, aerr := ApplyDiffWithOptions(ctx, e.addresses, addCSV, delCSV, e.opt.BatchSize, withSourceType(opt, "add"))
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
		agg.Skipped += res.Skipped
		agg.Total += res.Total
		agg.SkippedRows = append(agg.SkippedRows, res.SkippedRows...)
		applied++
		lastPeriod = ym
	}

	if applied == 0 {
		if e.opt.DiffFallbackFull {
			e.logger.Warn("no diff available in window, falling back to full", "months", months)
			run.Type = domain.SyncFull
			return e.runFull(ctx, run, fullURL, withSourceType(opt, "full"))
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

func applyOptions(pattern, sourceType string) (ApplyOptions, error) {
	pattern = strings.TrimSpace(pattern)
	opt := ApplyOptions{TownSkipPattern: pattern, SourceType: sourceType}
	if pattern == "" {
		return opt, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return opt, fmt.Errorf("compile town skip regex: %w", err)
	}
	opt.TownSkipRegex = re
	return opt, nil
}

func withSourceType(opt ApplyOptions, sourceType string) ApplyOptions {
	opt.SourceType = sourceType
	return opt
}

func persistContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx.Err() == nil {
		return ctx, func() {}
	}
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func (e *Engine) attachRunIDAndTime(runID string, createdAt time.Time, rows []domain.SyncSkippedRow) {
	for i := range rows {
		rows[i].RunID = runID
		rows[i].CreatedAt = createdAt
	}
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
