package server

import (
	"context"
	"errors"
	"net/http"
)

// SettingsService 是 server 消费的“管理画面运行时设置”能力。具体实现由
// cmd/server 注入（适配 internal/settings.Service），保持 server 包与 settings 包
// 的装配解耦（与 Authorizer/SyncTrigger 同一惯例）。
type SettingsService interface {
	Get(ctx context.Context) (SettingsView, error)
	Update(ctx context.Context, in SettingsUpdate) (SettingsView, error)
}

// SettingsView 是设置项的解析视图：每个键的有效值、默认值与是否被覆盖。
type SettingsView struct {
	DownloadMaxRetry        int
	DownloadMaxRetryDefault int
	DownloadMaxRetryOver    bool
	ScrapeFullURL           string
	ScrapeFullURLDefault    string
	ScrapeFullURLOver       bool
	TownSkipRegex           string
	TownSkipRegexDefault    string
	TownSkipRegexOver       bool
}

// SettingsUpdate 是归一化的更新入参：指针字段为 nil=不变；ResetToDefault 列出的键
// 恢复默认（删除覆盖）。
type SettingsUpdate struct {
	DownloadMaxRetry *int
	ScrapeFullURL    *string
	TownSkipRegex    *string
	ResetToDefault   []string
}

// SettingsError 区分客户端输入错误（400，Message 为日语提示）与内部错误（500）。
// 注入的 SettingsService 实现把校验失败包装成本类型，handler 据此选 HTTP 码。
type SettingsError struct {
	Validation bool
	Message    string
}

func (e *SettingsError) Error() string { return e.Message }

func settingsValidationMessage(err error) (string, bool) {
	var se *SettingsError
	if errors.As(err, &se) && se.Validation {
		return se.Message, true
	}
	return "", false
}

// ---- 对外 JSON 形态（snake_case，契约见 api/openapi.yaml）----

// settingItemDTO 是单个设置项的对外形态：当前有效值、默认值（“恢复默认”将回退到此）、
// 是否被覆盖。value/default 的具体类型按键而定（重试为整数、URL 为字符串）。
type settingItemDTO[T any] struct {
	Value      T    `json:"value"`
	Default    T    `json:"default"`
	Overridden bool `json:"overridden"`
}

// settingsDTO 对齐 openapi AdminSettings。
type settingsDTO struct {
	DownloadMaxRetry settingItemDTO[int]    `json:"download_max_retry"`
	ScrapeFullURL    settingItemDTO[string] `json:"scrape_full_url"`
	TownSkipRegex    settingItemDTO[string] `json:"town_skip_regex"`
}

// settingsUpdateDTO 是 PUT /v1/admin/settings 的请求体。指针字段缺省=不变；
// reset_to_default 列出的键删除其覆盖值（恢复默认）。
type settingsUpdateDTO struct {
	DownloadMaxRetry *int     `json:"download_max_retry"`
	ScrapeFullURL    *string  `json:"scrape_full_url"`
	TownSkipRegex    *string  `json:"town_skip_regex"`
	ResetToDefault   []string `json:"reset_to_default"`
}

// getSettings 处理 GET /v1/admin/settings（admin）。
func (h *handlers) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	view, err := h.settings.Get(ctx)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "get settings failed",
			"path", r.URL.Path, "trace_id", requestIDFrom(r.Context()), "err", err)
		h.writeStatusError(w, r, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toSettingsDTO(view))
}

// putSettings 处理 PUT /v1/admin/settings（admin）。校验失败返回 400 invalid_request
// 并回显日语提示；成功返回应用后的完整设置视图。
func (h *handlers) putSettings(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	var body settingsUpdateDTO
	if err := decodeJSON(r, &body); err != nil {
		h.writeStatusError(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	view, err := h.settings.Update(ctx, SettingsUpdate{
		DownloadMaxRetry: body.DownloadMaxRetry,
		ScrapeFullURL:    body.ScrapeFullURL,
		TownSkipRegex:    body.TownSkipRegex,
		ResetToDefault:   body.ResetToDefault,
	})
	if err != nil {
		if msg, ok := settingsValidationMessage(err); ok {
			h.writeStatusError(w, r, http.StatusBadRequest, "invalid_request", msg)
			return
		}
		h.logger.ErrorContext(r.Context(), "update settings failed",
			"path", r.URL.Path, "trace_id", requestIDFrom(r.Context()), "err", err)
		h.writeStatusError(w, r, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toSettingsDTO(view))
}

func toSettingsDTO(v SettingsView) settingsDTO {
	return settingsDTO{
		DownloadMaxRetry: settingItemDTO[int]{
			Value:      v.DownloadMaxRetry,
			Default:    v.DownloadMaxRetryDefault,
			Overridden: v.DownloadMaxRetryOver,
		},
		ScrapeFullURL: settingItemDTO[string]{
			Value:      v.ScrapeFullURL,
			Default:    v.ScrapeFullURLDefault,
			Overridden: v.ScrapeFullURLOver,
		},
		TownSkipRegex: settingItemDTO[string]{
			Value:      v.TownSkipRegex,
			Default:    v.TownSkipRegexDefault,
			Overridden: v.TownSkipRegexOver,
		},
	}
}
