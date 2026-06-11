package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
	syncpkg "github.com/r404r/JapanDigitalPostService/internal/sync"
)

const maxUploadBytes int64 = 64 << 20

// ---- 对外 JSON 形态（snake_case，契约见 api/openapi.yaml）----

// syncStatusDTO 对齐 openapi SyncStatus。
type syncStatusDTO struct {
	TotalAddresses int        `json:"total_addresses"`
	Running        bool       `json:"running"`
	LastSuccessAt  *time.Time `json:"last_success_at"`
	LastType       *string    `json:"last_type"`
}

// syncRunDTO 对齐 openapi SyncRun。diff_period / error_message 以空串表示"无"，
// 序列化时映射为 null（openapi 标注 nullable）。
type syncRunDTO struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	Status       string     `json:"status"`
	Trigger      string     `json:"trigger"`
	SourceURL    string     `json:"source_url"`
	FileChecksum string     `json:"file_checksum"`
	FileSize     int64      `json:"file_size"`
	DiffPeriod   *string    `json:"diff_period"`
	RowsAdded    int64      `json:"rows_added"`
	RowsUpdated  int64      `json:"rows_updated"`
	RowsDeleted  int64      `json:"rows_deleted"`
	RowsTotal    int64      `json:"rows_total"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	DurationMs   int64      `json:"duration_ms"`
	ErrorMessage *string    `json:"error_message"`
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
		DiffPeriod:   nilIfEmpty(r.DiffPeriod),
		RowsAdded:    r.RowsAdded,
		RowsUpdated:  r.RowsUpdated,
		RowsDeleted:  r.RowsDeleted,
		RowsTotal:    r.RowsTotal,
		StartedAt:    r.StartedAt,
		FinishedAt:   r.FinishedAt,
		DurationMs:   r.DurationMs,
		ErrorMessage: nilIfEmpty(r.ErrorMessage),
	}
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// syncStatus 处理 GET /v1/sync/status（read|admin）。
func (h *handlers) syncStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	total, err := h.reader.CountAll(ctx)
	if err != nil {
		h.writeStatusError(w, r, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	running, err := h.runs.CountRunning(ctx)
	if err != nil {
		h.writeStatusError(w, r, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	last, err := h.runs.LatestSuccess(ctx)
	if err != nil {
		h.writeStatusError(w, r, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	dto := syncStatusDTO{TotalAddresses: total, Running: running > 0}
	if last != nil {
		// 成功运行的完成时刻即"最近一次成功同步时间"；FinishedAt 理应非空，缺省回退到起始时刻。
		if last.FinishedAt != nil {
			dto.LastSuccessAt = last.FinishedAt
		} else {
			t := last.StartedAt
			dto.LastSuccessAt = &t
		}
		lt := string(last.Type)
		dto.LastType = &lt
	}
	writeJSON(w, http.StatusOK, dto)
}

// syncRuns 处理 GET /v1/sync/runs?limit=&offset=（read|admin）。
func (h *handlers) syncRuns(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	limit, ok := parseIntParam(r, "limit")
	if !ok {
		h.writeStatusError(w, r, http.StatusBadRequest, "invalid_request", "limit must be an integer")
		return
	}
	offset, ok := parseIntParam(r, "offset")
	if !ok {
		h.writeStatusError(w, r, http.StatusBadRequest, "invalid_request", "offset must be an integer")
		return
	}

	runs, err := h.runs.List(ctx, limit, offset)
	if err != nil {
		h.writeStatusError(w, r, http.StatusInternalServerError, "internal_error", "internal error")
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

// syncTrigger 处理 POST /v1/sync/trigger（admin）。异步触发并以 202 返回创建的运行记录。
func (h *handlers) syncTrigger(w http.ResponseWriter, r *http.Request) {
	var req triggerRequest
	if err := decodeJSON(r, &req); err != nil {
		h.writeStatusError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	st := domain.SyncType(req.Type)
	// auto/full/diff 均为合法入参；auto 由引擎按库空与否解析为 full/diff，落库的 run 始终是 full 或 diff。
	if st != domain.SyncFull && st != domain.SyncDiff && st != domain.SyncAuto {
		h.writeStatusError(w, r, http.StatusBadRequest, "invalid_request", "type must be auto, full or diff")
		return
	}

	run, err := h.trigger.TriggerAsync(st, domain.TriggerManual)
	if err != nil {
		if errors.Is(err, domain.ErrSyncRunning) {
			h.writeStatusError(w, r, http.StatusConflict, "sync_running", "a sync is already running")
			return
		}
		h.logger.ErrorContext(r.Context(), "trigger sync failed",
			"path", r.URL.Path, "trace_id", requestIDFrom(r.Context()), "err", err)
		h.writeStatusError(w, r, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusAccepted, toSyncRunDTO(*run))
}

// syncUpload handles POST /v1/sync/upload (admin). It accepts one multipart
// part named "file" containing Japan Post utf_ken_all as .zip or UTF-8 .csv.
func (h *handlers) syncUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		h.writeStatusError(w, r, http.StatusBadRequest, "invalid_request", "アップロードできるファイルサイズを超えています。")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		h.writeStatusError(w, r, http.StatusBadRequest, "invalid_request", "file フィールドに zip または csv ファイルを指定してください。")
		return
	}
	defer file.Close()

	name := header.Filename
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, ".zip") && !strings.HasSuffix(lower, ".csv") {
		h.writeStatusError(w, r, http.StatusBadRequest, "unsupported_file", "zip または csv ファイルのみアップロードできます。")
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		h.writeStatusError(w, r, http.StatusBadRequest, "invalid_request", "アップロードファイルを読み取れませんでした。")
		return
	}
	if int64(len(data)) > maxUploadBytes {
		h.writeStatusError(w, r, http.StatusBadRequest, "invalid_request", "アップロードできるファイルサイズを超えています。")
		return
	}

	run, err := h.uploader.UploadFull(r.Context(), name, data)
	if err != nil {
		if errors.Is(err, domain.ErrSyncRunning) {
			h.writeStatusError(w, r, http.StatusConflict, "sync_running", "同期が実行中です。完了後に再度アップロードしてください。")
			return
		}
		status, msg := uploadError(err)
		code := http.StatusUnprocessableEntity
		if status == "unsupported_file" {
			code = http.StatusBadRequest
		}
		h.writeStatusError(w, r, code, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, toSyncRunDTO(*run))
}

func uploadError(err error) (string, string) {
	switch {
	case errors.Is(err, syncpkg.ErrUnsupportedUploadFile):
		return "unsupported_file", "zip または csv ファイルのみアップロードできます。"
	case errors.Is(err, syncpkg.ErrUploadCSVTooLarge):
		return "invalid_request", "CSV の展開サイズが上限を超えています。"
	case errors.Is(err, syncpkg.ErrUploadEncoding):
		return "csv_format_error", "UTF-8 の utf_ken_all CSV のみ対応しています。Shift-JIS 版は利用できません。"
	case strings.Contains(err.Error(), "open uploaded zip"):
		return "unzip_failed", "zip ファイルを解凍できませんでした。日本郵政の utf_ken_all zip を指定してください。"
	case strings.Contains(err.Error(), "parse full"):
		return "csv_format_error", "CSV の形式が utf_ken_all と一致しません。"
	default:
		return "import_failed", "同期データの取り込みに失敗しました。"
	}
}

// writeStatusError 落地统一错误体（spec §7），带 trace id；5xx 同时记服务端日志。
func (h *handlers) writeStatusError(w http.ResponseWriter, r *http.Request, httpCode int, status, msg string) {
	traceID := requestIDFrom(r.Context())
	if httpCode >= 500 {
		h.logger.WarnContext(r.Context(), "sync handler error",
			"http", httpCode, "status", status, "path", r.URL.Path, "trace_id", traceID, "msg", msg)
	}
	writeJSON(w, httpCode, errorDTO{Status: status, Message: msg, TraceID: traceID})
}

// decodeJSON 解码请求体（限制大小，拒绝未知字段）。
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
