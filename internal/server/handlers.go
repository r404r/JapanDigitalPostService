package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"github.com/r404r/JapanDigitalPostService/internal/query"
	"github.com/r404r/JapanDigitalPostService/internal/version"
)

// ---- 对外 JSON 形态（snake_case，契约见 api/openapi.yaml）----

type addressDTO struct {
	Zipcode        string `json:"zipcode"`
	JISCode        string `json:"jis_code"`
	Prefecture     string `json:"prefecture"`
	PrefectureKana string `json:"prefecture_kana"`
	City           string `json:"city"`
	CityKana       string `json:"city_kana"`
	Town           string `json:"town"`
	TownKana       string `json:"town_kana"`
}

type searchResponseDTO struct {
	Status        string       `json:"status"`
	TotalCount    int          `json:"total_count"`
	ReturnedCount int          `json:"returned_count"`
	Truncated     bool         `json:"truncated"`
	AddressCount  *int         `json:"address_count,omitempty"`
	Items         []addressDTO `json:"items"`
}

type errorDTO struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	TraceID string `json:"trace_id,omitempty"`
}

func toAddressDTO(a domain.Address) addressDTO {
	return addressDTO{
		Zipcode:        a.Zipcode,
		JISCode:        a.JISCode,
		Prefecture:     a.Prefecture,
		PrefectureKana: a.PrefectureKana,
		City:           a.City,
		CityKana:       a.CityKana,
		Town:           a.Town,
		TownKana:       a.TownKana,
	}
}

func toSearchDTO(res *query.Result) searchResponseDTO {
	items := make([]addressDTO, 0, len(res.Items))
	for _, a := range res.Items {
		items = append(items, toAddressDTO(a))
	}
	return searchResponseDTO{
		Status:        string(res.Status),
		TotalCount:    res.TotalCount,
		ReturnedCount: res.Returned,
		Truncated:     res.Truncated,
		AddressCount:  res.AddressCount,
		Items:         items,
	}
}

// ---- handlers ----

type handlers struct {
	svc          *query.Service
	reader       domain.AddressReader     // 同步状态的地址计数（CountAll）
	runs         domain.SyncRunRepository // 同步状态/历史
	trigger      SyncTrigger              // 手动触发（异步）
	queryTimeout time.Duration
	logger       *slog.Logger
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": version.Version,
	})
}

// searchAddresses 处理 GET /v1/addresses。
func (h *handlers) searchAddresses(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	limit, ok := parseIntParam(r, "limit")
	if !ok {
		h.writeError(w, r, &query.Error{Status: query.StatusInvalidRequest, HTTPStatus: 400, Message: "limit must be an integer"})
		return
	}
	offset, ok := parseIntParam(r, "offset")
	if !ok {
		h.writeError(w, r, &query.Error{Status: query.StatusInvalidRequest, HTTPStatus: 400, Message: "offset must be an integer"})
		return
	}

	params := query.SearchParams{
		Zipcode:    r.URL.Query().Get("zipcode"),
		Prefecture: r.URL.Query().Get("prefecture"),
		City:       r.URL.Query().Get("city"),
		Q:          r.URL.Query().Get("q"),
		Limit:      limit,
		Offset:     offset,
	}

	res, qerr := h.svc.Search(ctx, params)
	if qerr != nil {
		h.writeError(w, r, qerr)
		return
	}
	writeJSON(w, http.StatusOK, toSearchDTO(res))
}

// getByZipcode 处理 GET /v1/addresses/{zipcode}。
func (h *handlers) getByZipcode(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	res, qerr := h.svc.GetByZipcode(ctx, r.PathValue("zipcode"))
	if qerr != nil {
		h.writeError(w, r, qerr)
		return
	}
	writeJSON(w, http.StatusOK, toSearchDTO(res))
}

// writeError 落地统一错误体（spec §7），并把 trace id 同时写入响应与日志。
func (h *handlers) writeError(w http.ResponseWriter, r *http.Request, qerr *query.Error) {
	traceID := requestIDFrom(r.Context())
	if qerr.HTTPStatus >= 500 || qerr.Status == query.StatusTimeout {
		h.logger.WarnContext(r.Context(), "query failed",
			"status", string(qerr.Status), "http", qerr.HTTPStatus,
			"path", r.URL.Path, "trace_id", traceID, "msg", qerr.Message)
	}
	writeJSON(w, qerr.HTTPStatus, errorDTO{
		Status:  string(qerr.Status),
		Message: qerr.Message,
		TraceID: traceID,
	})
}

// parseIntParam 解析可选的整型 query 参数；缺省返回 (0, true)，非法返回 (0, false)。
func parseIntParam(r *http.Request, name string) (int, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return 0, true
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
