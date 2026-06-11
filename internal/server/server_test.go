package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"github.com/r404r/JapanDigitalPostService/internal/query"
	"github.com/r404r/JapanDigitalPostService/internal/store"
)

func newTestServer(t *testing.T, maxTotal int) http.Handler {
	t.Helper()
	db, err := store.OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := store.SeedSampleIfEmpty(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := query.NewService(store.NewAddressRepo(db), 20, maxTotal)
	return NewRouter(Options{QueryService: svc, QueryTimeout: 2 * time.Second})
}

func do(t *testing.T, h http.Handler, path string) (*http.Response, searchResponseDTO) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	var body searchResponseDTO
	_ = json.NewDecoder(res.Body).Decode(&body)
	return res, body
}

func TestHealth(t *testing.T) {
	h := newTestServer(t, 1000)
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health code = %d", rec.Code)
	}
	if rec.Header().Get(requestIDHeader) == "" {
		t.Error("missing X-Request-Id header")
	}
}

func TestSearchByZipcode_Query(t *testing.T) {
	h := newTestServer(t, 1000)
	res, body := do(t, h, "/v1/addresses?zipcode=4980000")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("code = %d", res.StatusCode)
	}
	if body.Status != "ok" || body.TotalCount != 3 || body.ReturnedCount != 3 {
		t.Errorf("got %+v", body)
	}
	if res.Header.Get(requestIDHeader) == "" {
		t.Error("missing request id header")
	}
}

func TestSearch_InvalidRequest(t *testing.T) {
	h := newTestServer(t, 1000)
	res, body := do(t, h, "/v1/addresses")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", res.StatusCode)
	}
	if body.Status != "invalid_request" {
		t.Errorf("status = %s", body.Status)
	}
}

func TestSearch_BadLimit(t *testing.T) {
	h := newTestServer(t, 1000)
	res, body := do(t, h, "/v1/addresses?q=x&limit=abc")
	if res.StatusCode != http.StatusBadRequest || body.Status != "invalid_request" {
		t.Fatalf("code=%d status=%s", res.StatusCode, body.Status)
	}
}

func TestSearch_TooManyResults(t *testing.T) {
	h := newTestServer(t, 2) // seed 有 5 条東京都，maxTotal=2 触发 too_many
	res, body := do(t, h, "/v1/addresses?prefecture=東京都")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("code = %d", res.StatusCode)
	}
	if body.Status != "too_many_results" {
		t.Errorf("status = %s, want too_many_results", body.Status)
	}
	if body.TotalCount != 5 {
		t.Errorf("total = %d, want 5", body.TotalCount)
	}
}

func TestGetByZipcodePath_MultiAddress(t *testing.T) {
	h := newTestServer(t, 1000)
	res, body := do(t, h, "/v1/addresses/4980000")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("code = %d", res.StatusCode)
	}
	if body.AddressCount == nil || *body.AddressCount != 3 {
		t.Errorf("address_count = %v, want 3", body.AddressCount)
	}
	if len(body.Items) != 3 {
		t.Errorf("items = %d, want 3", len(body.Items))
	}
}

func TestGetByZipcodePath_NotFound(t *testing.T) {
	h := newTestServer(t, 1000)
	res, body := do(t, h, "/v1/addresses/9999999")
	if res.StatusCode != http.StatusNotFound || body.Status != "not_found" {
		t.Fatalf("code=%d status=%s, want 404/not_found", res.StatusCode, body.Status)
	}
}

func TestGetByZipcodePath_Invalid(t *testing.T) {
	h := newTestServer(t, 1000)
	res, body := do(t, h, "/v1/addresses/100")
	if res.StatusCode != http.StatusBadRequest || body.Status != "invalid_request" {
		t.Fatalf("code=%d status=%s, want 400/invalid_request", res.StatusCode, body.Status)
	}
}

// blockingRepo 在 ctx 结束前一直阻塞，用于在 HTTP 层验证查询超时映射。
type blockingRepo struct{}

func (blockingRepo) Search(ctx context.Context, q domain.AddressQuery) ([]domain.Address, int, error) {
	<-ctx.Done()
	return nil, 0, ctx.Err()
}
func (blockingRepo) CountAll(ctx context.Context) (int, error) { return 0, nil }

func TestSearch_TimeoutAt504(t *testing.T) {
	svc := query.NewService(blockingRepo{}, 20, 1000)
	h := NewRouter(Options{QueryService: svc, QueryTimeout: 10 * time.Millisecond})
	res, body := do(t, h, "/v1/addresses?q=x")
	if res.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("code = %d, want 504", res.StatusCode)
	}
	if body.Status != "timeout" {
		t.Errorf("status = %s, want timeout", body.Status)
	}
}
