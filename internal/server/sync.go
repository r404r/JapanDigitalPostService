package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// decodeJSONBody 解码请求体（限制大小，拒绝未知字段），用于带 body 的写端点。
func decodeJSONBody(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// SyncRunner 抽象手动触发同步所需的引擎能力（internal/sync.Engine 实现之）。
// 用接口隔离，使 internal/server 不直接依赖同步引擎的具体类型与下载/解析细节。
type SyncRunner interface {
	// Run 执行一次同步。并发触发返回 domain.ErrSyncRunning。
	Run(ctx context.Context, reqType domain.SyncType, trigger domain.SyncTrigger) (*domain.SyncRun, error)
}

// SyncOptions 配置同步状态/历史/触发端点的依赖。
type SyncOptions struct {
	Runs         domain.SyncRunRepository
	Reader       domain.AddressReader // 提供 total_addresses 统计
	Runner       SyncRunner           // 手动触发；为 nil 时 /sync/trigger 返回 500
	QueryTimeout time.Duration        // status/runs 只读查询超时
	Logger       *slog.Logger
}

// SyncHandlers 暴露 /v1/sync/* 端点。鉴权由外层 RequireScope 包裹（status/runs 需
// read，trigger 需 admin），handler 本身不做认证。
type SyncHandlers struct {
	runs    domain.SyncRunRepository
	reader  domain.AddressReader
	runner  SyncRunner
	timeout time.Duration
	logger  *slog.Logger
}

// NewSyncHandlers 构造同步端点处理器。
func NewSyncHandlers(opts SyncOptions) *SyncHandlers {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	timeout := opts.QueryTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &SyncHandlers{
		runs:    opts.Runs,
		reader:  opts.Reader,
		runner:  opts.Runner,
		timeout: timeout,
		logger:  logger,
	}
}

// ---- 对外 JSON 形态（契约见 api/openapi.yaml 的 SyncStatus / SyncRun）----

type syncStatusDTO struct {
	TotalAddresses int        `json:"total_addresses"`
	Running        bool       `json:"running"`
	LastSuccessAt  *time.Time `json:"last_success_at"`
	LastType       *string    `json:"last_type"`
}

type syncRunDTO struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	Status       string     `json:"status"`
	Trigger      string     `json:"trigger"`
	SourceURL    string     `json:"source_url"`
	FileChecksum string     `json:"file_checksum"`
	FileSize     int64      `json:"file_size"`
	DiffPeriod   string     `json:"diff_period"`
	RowsAdded    int64      `json:"rows_added"`
	RowsUpdated  int64      `json:"rows_updated"`
	RowsDeleted  int64      `json:"rows_deleted"`
	RowsTotal    int64      `json:"rows_total"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	DurationMs   int64      `json:"duration_ms"`
	ErrorMessage string     `json:"error_message"`
}

func toSyncRunDTO(r domain.SyncRun) syncRunDTO {
	return syncRunDTO{
		ID:           r.ID,
		Type:         string(r.Type),
		Status:       string(r.Status),
		Trigger:      string(r.Trigger),
		SourceURL:    r.SourceURL,
		FileChecksum: r.FileChecksum,
		FileSize:     r.FileSize,
		DiffPeriod:   r.DiffPeriod,
		RowsAdded:    r.RowsAdded,
		RowsUpdated:  r.RowsUpdated,
		RowsDeleted:  r.RowsDeleted,
		RowsTotal:    r.RowsTotal,
		StartedAt:    r.StartedAt,
		FinishedAt:   r.FinishedAt,
		DurationMs:   r.DurationMs,
		ErrorMessage: r.ErrorMessage,
	}
}

// GetStatus 处理 GET /v1/sync/status：当前数据量、是否运行中、最近成功时间/类型。
func (h *SyncHandlers) GetStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	total, err := h.reader.CountAll(ctx)
	if err != nil {
		h.fail(w, r, "count addresses", err)
		return
	}
	running, err := h.runs.CountRunning(ctx)
	if err != nil {
		h.fail(w, r, "count running", err)
		return
	}
	last, err := h.runs.LatestSuccess(ctx)
	if err != nil {
		h.fail(w, r, "latest success", err)
		return
	}

	out := syncStatusDTO{TotalAddresses: total, Running: running > 0}
	if last != nil {
		if last.FinishedAt != nil {
			out.LastSuccessAt = last.FinishedAt
		} else {
			out.LastSuccessAt = &last.StartedAt
		}
		lt := string(last.Type)
		out.LastType = &lt
	}
	writeJSON(w, http.StatusOK, out)
}

// ListRuns 处理 GET /v1/sync/runs：分页历史（按 started_at 倒序）。
func (h *SyncHandlers) ListRuns(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	limit, ok := parseIntParam(r, "limit")
	if !ok || limit < 0 {
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", "limit must be a non-negative integer")
		return
	}
	offset, ok := parseIntParam(r, "offset")
	if !ok || offset < 0 {
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", "offset must be a non-negative integer")
		return
	}

	runs, err := h.runs.List(ctx, limit, offset)
	if err != nil {
		h.fail(w, r, "list runs", err)
		return
	}
	out := make([]syncRunDTO, 0, len(runs))
	for _, run := range runs {
		out = append(out, toSyncRunDTO(run))
	}
	writeJSON(w, http.StatusOK, out)
}

type triggerRequest struct {
	Type string `json:"type"`
}

// Trigger 处理 POST /v1/sync/trigger（admin）：手动触发一次同步并返回 run。
// 同步同步执行（与 cmd/batch 一致），返回时 run 已完成；并发触发返回 409 sync_running。
//
// 实际同步用 context.Background()，使客户端断连不会中断进行中的写入/落库（与
// 进程内调度、cmd/batch 的 context 选择一致）；请求 context 仅用于解析 body。
func (h *SyncHandlers) Trigger(w http.ResponseWriter, r *http.Request) {
	if h.runner == nil {
		h.fail(w, r, "trigger", errors.New("sync runner not configured"))
		return
	}

	var req triggerRequest
	if err := decodeJSONBody(r, &req); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	var t domain.SyncType
	switch req.Type {
	case "full":
		t = domain.SyncFull
	case "diff":
		t = domain.SyncDiff
	default:
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", "type must be full or diff")
		return
	}

	run, err := h.runner.Run(context.Background(), t, domain.TriggerManual)
	if err != nil {
		if errors.Is(err, domain.ErrSyncRunning) {
			h.writeError(w, r, http.StatusConflict, "sync_running", "a sync is already running")
			return
		}
		h.fail(w, r, "trigger sync", err)
		return
	}
	writeJSON(w, http.StatusAccepted, toSyncRunDTO(*run))
}

// fail 记录服务端日志并写出统一 500（不向客户端泄露内部细节）。
func (h *SyncHandlers) fail(w http.ResponseWriter, r *http.Request, op string, err error) {
	h.logger.ErrorContext(r.Context(), "sync endpoint failed",
		"op", op, "path", r.URL.Path, "trace_id", requestIDFrom(r.Context()), "err", err)
	h.writeError(w, r, http.StatusInternalServerError, "internal_error", "internal error")
}

// writeError 写出 spec §7 统一错误体，并带上 trace id。
func (h *SyncHandlers) writeError(w http.ResponseWriter, r *http.Request, code int, status, message string) {
	writeJSON(w, code, errorDTO{
		Status:  status,
		Message: message,
		TraceID: requestIDFrom(r.Context()),
	})
}
