package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// ctxKey 是上下文中存放已认证 token 的私有键类型。
type ctxKey struct{}

// TokenFromContext 取出 RequireScope 中间件放入的已认证 token。
func TokenFromContext(ctx context.Context) (*domain.Token, bool) {
	t, ok := ctx.Value(ctxKey{}).(*domain.Token)
	return t, ok
}

// RequireScope 返回一个中间件：校验 Bearer token 并要求至少 need scope。
// 失败时写出统一错误（401/403），绝不泄露原因细节。成功则把 token 注入
// 上下文并调用 next。
func (s *Service) RequireScope(need domain.Scope, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or malformed Authorization header")
			return
		}
		tok, err := s.Authenticate(r.Context(), raw)
		if err != nil {
			if errors.Is(err, ErrUnauthorized) {
				writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
				return
			}
			// 仓储/内部错误：不向客户端暴露细节，仅记录服务端日志。
			slog.Error("authenticate failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		if !tok.Scope.Satisfies(need) {
			writeError(w, http.StatusForbidden, "forbidden", "insufficient scope")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, tok)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bearerToken 从 Authorization 头解析 "Bearer <token>"（scheme 大小写不敏感）。
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	tok := strings.TrimSpace(h[len(prefix):])
	if tok == "" {
		return "", false
	}
	return tok, true
}

// Handlers 暴露 token 管理端点。它们都假设已被 RequireScope(admin) 包裹。
type Handlers struct {
	svc *Service
}

// NewHandlers 构造管理端点处理器。
func NewHandlers(svc *Service) *Handlers {
	return &Handlers{svc: svc}
}

// tokenInfo 是脱敏后的对外表示（绝不含 Hash 或明文）。
type tokenInfo struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scope      string     `json:"scope"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

func toInfo(t *domain.Token) tokenInfo {
	return tokenInfo{
		ID:         t.ID,
		Name:       t.Name,
		Prefix:     t.Prefix,
		Scope:      string(t.Scope),
		CreatedAt:  t.CreatedAt,
		ExpiresAt:  t.ExpiresAt,
		LastUsedAt: t.LastUsedAt,
		RevokedAt:  t.RevokedAt,
	}
}

type createTokenRequest struct {
	Name    string `json:"name"`
	Scope   string `json:"scope"`
	TTLSecs *int64 `json:"ttl_seconds,omitempty"` // 可选过期时长；缺省=永不过期
}

type createTokenResponse struct {
	tokenInfo
	Token string `json:"token"` // 明文，仅此一次返回
}

// CreateToken 处理 POST /v1/tokens（admin）。
func (h *Handlers) CreateToken(w http.ResponseWriter, r *http.Request) {
	var req createTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	params := IssueParams{Name: req.Name, Scope: domain.Scope(req.Scope)}
	if req.TTLSecs != nil {
		if *req.TTLSecs <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "ttl_seconds must be positive")
			return
		}
		d := time.Duration(*req.TTLSecs) * time.Second
		params.TTL = &d
	}

	issued, err := h.svc.Issue(r.Context(), params)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmptyName):
			writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		case errors.Is(err, ErrInvalidScope):
			writeError(w, http.StatusBadRequest, "invalid_request", "scope must be read or admin")
		default:
			slog.Error("issue token failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, createTokenResponse{
		tokenInfo: toInfo(issued.Token),
		Token:     issued.Plaintext,
	})
}

// ListTokens 处理 GET /v1/tokens（admin），返回脱敏列表。
func (h *Handlers) ListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.svc.List(r.Context())
	if err != nil {
		slog.Error("list tokens failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	out := make([]tokenInfo, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, toInfo(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// RevokeToken 处理 DELETE /v1/tokens/{id}（admin）。吊销立即生效。
func (h *Handlers) RevokeToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing token id")
		return
	}
	if err := h.svc.Revoke(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrTokenNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "token not found")
			return
		}
		slog.Error("revoke token failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeJSON 解码请求体（限制大小，拒绝未知字段）。
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// writeJSON 写出 JSON 响应。
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode failed", "err", err)
	}
}

// errorResponse 是 spec §7 的统一错误格式。message 必须是安全文案，
// 绝不包含 token、hash、内部栈或配置。
type errorResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// writeError 写出统一错误响应。
func writeError(w http.ResponseWriter, code int, status, message string) {
	writeJSON(w, code, errorResponse{Status: status, Message: message})
}
