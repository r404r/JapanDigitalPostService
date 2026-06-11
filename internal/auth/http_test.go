package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// newWiredMux 复刻 cmd/server 的路由装配，便于端到端测试鉴权矩阵。
func newWiredMux(t *testing.T) (*http.ServeMux, *Service) {
	t.Helper()
	svc := NewService(NewMemoryStore(), time.Now)
	h := NewHandlers(svc)
	mux := http.NewServeMux()
	mux.Handle("POST /v1/tokens", svc.RequireScope(domain.ScopeAdmin, http.HandlerFunc(h.CreateToken)))
	mux.Handle("GET /v1/tokens", svc.RequireScope(domain.ScopeAdmin, http.HandlerFunc(h.ListTokens)))
	mux.Handle("DELETE /v1/tokens/{id}", svc.RequireScope(domain.ScopeAdmin, http.HandlerFunc(h.RevokeToken)))
	// 一个仅需 read scope 的受保护端点，用于验证 scope 边界。
	mux.Handle("GET /v1/protected", svc.RequireScope(domain.ScopeRead, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	return mux, svc
}

func do(mux *http.ServeMux, method, path, bearer, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestEndpoints_AuthMatrix(t *testing.T) {
	mux, svc := newWiredMux(t)
	ctx := context.Background()
	admin, _ := svc.Issue(ctx, IssueParams{Name: "admin", Scope: domain.ScopeAdmin})
	read, _ := svc.Issue(ctx, IssueParams{Name: "read", Scope: domain.ScopeRead})

	// 无 token → 401
	if got := do(mux, "GET", "/v1/tokens", "", "").Code; got != http.StatusUnauthorized {
		t.Errorf("no token: got %d, want 401", got)
	}
	// 无效 token → 401
	if got := do(mux, "GET", "/v1/tokens", "jdps_bogus", "").Code; got != http.StatusUnauthorized {
		t.Errorf("bad token: got %d, want 401", got)
	}
	// read token 访问 admin 端点 → 403
	if got := do(mux, "GET", "/v1/tokens", read.Plaintext, "").Code; got != http.StatusForbidden {
		t.Errorf("read on admin endpoint: got %d, want 403", got)
	}
	// read token 访问 read 端点 → 200
	if got := do(mux, "GET", "/v1/protected", read.Plaintext, "").Code; got != http.StatusOK {
		t.Errorf("read on read endpoint: got %d, want 200", got)
	}
	// admin token 访问 admin 端点 → 200
	if got := do(mux, "GET", "/v1/tokens", admin.Plaintext, "").Code; got != http.StatusOK {
		t.Errorf("admin on admin endpoint: got %d, want 200", got)
	}
}

func TestCreateToken_ReturnsPlaintextOnce_AndListIsRedacted(t *testing.T) {
	mux, svc := newWiredMux(t)
	admin, _ := svc.Issue(context.Background(), IssueParams{Name: "admin", Scope: domain.ScopeAdmin})

	rec := do(mux, "POST", "/v1/tokens", admin.Plaintext, `{"name":"app","scope":"read"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	var created createTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" {
		t.Fatal("create response must include plaintext token once")
	}
	newPlain := created.Token

	// 列表脱敏：绝不含明文或 hash。
	listRec := do(mux, "GET", "/v1/tokens", admin.Plaintext, "")
	bodyStr := listRec.Body.String()
	if strings.Contains(bodyStr, newPlain) {
		t.Error("list leaked plaintext token")
	}
	if strings.Contains(bodyStr, "hash") || strings.Contains(strings.ToLower(bodyStr), hashToken(newPlain)) {
		t.Error("list leaked token hash")
	}
	// 但新发的 token 必须能用。
	if got := do(mux, "GET", "/v1/protected", newPlain, "").Code; got != http.StatusOK {
		t.Errorf("newly issued token should work: got %d", got)
	}
}

func TestCreateToken_BadInput(t *testing.T) {
	mux, svc := newWiredMux(t)
	admin, _ := svc.Issue(context.Background(), IssueParams{Name: "admin", Scope: domain.ScopeAdmin})

	cases := []string{
		`{"name":"","scope":"read"}`,
		`{"name":"x","scope":"superuser"}`,
		`{"name":"x","scope":"read","ttl_seconds":-1}`,
		`not json`,
		`{"name":"x","scope":"read","unexpected":true}`,
	}
	for _, body := range cases {
		rec := do(mux, "POST", "/v1/tokens", admin.Plaintext, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: got %d, want 400", body, rec.Code)
		}
	}
}

func TestRevokeToken_Lifecycle(t *testing.T) {
	mux, svc := newWiredMux(t)
	admin, _ := svc.Issue(context.Background(), IssueParams{Name: "admin", Scope: domain.ScopeAdmin})

	// 发行一个 read token
	rec := do(mux, "POST", "/v1/tokens", admin.Plaintext, `{"name":"tmp","scope":"read"}`)
	var created createTokenResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	// 吊销立即生效
	if got := do(mux, "DELETE", "/v1/tokens/"+created.ID, admin.Plaintext, "").Code; got != http.StatusNoContent {
		t.Fatalf("revoke: got %d, want 204", got)
	}
	if got := do(mux, "GET", "/v1/protected", created.Token, "").Code; got != http.StatusUnauthorized {
		t.Errorf("revoked token should be 401: got %d", got)
	}
	// 吊销不存在的 id → 404
	if got := do(mux, "DELETE", "/v1/tokens/no-such-id", admin.Plaintext, "").Code; got != http.StatusNotFound {
		t.Errorf("revoke unknown: got %d, want 404", got)
	}
}

func TestErrorResponse_NoSensitiveLeak(t *testing.T) {
	mux, _ := newWiredMux(t)
	rec := do(mux, "GET", "/v1/tokens", "jdps_some-secret-value", "")
	body := rec.Body.String()
	// 错误体不得回显客户端提供的 token，也不得含 hash/栈。
	if strings.Contains(body, "some-secret-value") {
		t.Error("error response echoed the presented token")
	}
	var er errorResponse
	if err := json.Unmarshal([]byte(body), &er); err != nil {
		t.Fatalf("error body not valid JSON: %v", err)
	}
	if er.Status != "unauthorized" {
		t.Errorf("status = %q, want unauthorized", er.Status)
	}
}
