package server

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

// fakeRuns 是 domain.SyncRunRepository 的内存实现，便于 handler 单测。
type fakeRuns struct {
	list       []domain.SyncRun
	running    int64
	latestOK   *domain.SyncRun
	listErr    error
	runningErr error
}

func (f *fakeRuns) Create(context.Context, *domain.SyncRun) error { return nil }
func (f *fakeRuns) Update(context.Context, *domain.SyncRun) error { return nil }
func (f *fakeRuns) Latest(context.Context) (*domain.SyncRun, error) {
	if len(f.list) == 0 {
		return nil, nil
	}
	return &f.list[0], nil
}
func (f *fakeRuns) LatestSuccess(context.Context) (*domain.SyncRun, error) {
	return f.latestOK, nil
}
func (f *fakeRuns) List(_ context.Context, limit, offset int) ([]domain.SyncRun, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.list, nil
}
func (f *fakeRuns) CountRunning(context.Context) (int64, error) {
	return f.running, f.runningErr
}

// fakeReader 是 domain.AddressReader 的最小实现（只需 CountAll）。
type fakeReader struct {
	count int
	err   error
}

func (f *fakeReader) Search(context.Context, domain.AddressQuery) ([]domain.Address, int, error) {
	return nil, 0, nil
}
func (f *fakeReader) CountAll(context.Context) (int, error) { return f.count, f.err }

// fakeRunner 是 SyncRunner 的可控实现。
type fakeRunner struct {
	run *domain.SyncRun
	err error
}

func (f *fakeRunner) Run(context.Context, domain.SyncType, domain.SyncTrigger) (*domain.SyncRun, error) {
	return f.run, f.err
}

func newSyncTestServer(opts SyncOptions) http.Handler {
	h := NewSyncHandlers(opts)
	mux := http.NewServeMux()
	mux.Handle("GET /v1/sync/status", http.HandlerFunc(h.GetStatus))
	mux.Handle("GET /v1/sync/runs", http.HandlerFunc(h.ListRuns))
	mux.Handle("POST /v1/sync/trigger", http.HandlerFunc(h.Trigger))
	return mux
}

func TestSyncStatus(t *testing.T) {
	finished := time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC)
	runs := &fakeRuns{
		running:  0,
		latestOK: &domain.SyncRun{Type: domain.SyncFull, Status: domain.StatusSuccess, FinishedAt: &finished},
	}
	h := newSyncTestServer(SyncOptions{Runs: runs, Reader: &fakeReader{count: 42}})

	req := httptest.NewRequest(http.MethodGet, "/v1/sync/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d, want 200", rec.Code)
	}
	var body syncStatusDTO
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TotalAddresses != 42 || body.Running {
		t.Errorf("status=%+v, want 42/not-running", body)
	}
	if body.LastType == nil || *body.LastType != "full" || body.LastSuccessAt == nil || !body.LastSuccessAt.Equal(finished) {
		t.Errorf("last success not reflected: %+v", body)
	}
}

func TestSyncStatus_NoSuccessYet(t *testing.T) {
	h := newSyncTestServer(SyncOptions{Runs: &fakeRuns{running: 1}, Reader: &fakeReader{count: 0}})
	req := httptest.NewRequest(http.MethodGet, "/v1/sync/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body syncStatusDTO
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if !body.Running || body.LastSuccessAt != nil || body.LastType != nil {
		t.Errorf("status=%+v, want running/nil last", body)
	}
}

func TestSyncRuns(t *testing.T) {
	runs := &fakeRuns{list: []domain.SyncRun{
		{ID: "r2", Type: domain.SyncDiff, Status: domain.StatusSuccess, Trigger: domain.TriggerManual},
		{ID: "r1", Type: domain.SyncFull, Status: domain.StatusFailed, Trigger: domain.TriggerSchedule},
	}}
	h := newSyncTestServer(SyncOptions{Runs: runs, Reader: &fakeReader{}})

	req := httptest.NewRequest(http.MethodGet, "/v1/sync/runs?limit=10", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	var out []syncRunDTO
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 || out[0].ID != "r2" || out[0].Type != "diff" {
		t.Errorf("runs=%+v", out)
	}
}

func TestSyncRuns_BadLimit(t *testing.T) {
	h := newSyncTestServer(SyncOptions{Runs: &fakeRuns{}, Reader: &fakeReader{}})
	req := httptest.NewRequest(http.MethodGet, "/v1/sync/runs?limit=abc", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d, want 400", rec.Code)
	}
}

func TestSyncTrigger_Success(t *testing.T) {
	finished := time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC)
	runner := &fakeRunner{run: &domain.SyncRun{ID: "rx", Type: domain.SyncFull, Status: domain.StatusSuccess, Trigger: domain.TriggerManual, RowsAdded: 3, FinishedAt: &finished}}
	h := newSyncTestServer(SyncOptions{Runs: &fakeRuns{}, Reader: &fakeReader{}, Runner: runner})

	req := httptest.NewRequest(http.MethodPost, "/v1/sync/trigger", strings.NewReader(`{"type":"full"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("code=%d, want 202", rec.Code)
	}
	var body syncRunDTO
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.ID != "rx" || body.Status != "success" || body.RowsAdded != 3 {
		t.Errorf("run=%+v", body)
	}
}

func TestSyncTrigger_Conflict(t *testing.T) {
	h := newSyncTestServer(SyncOptions{Runs: &fakeRuns{}, Reader: &fakeReader{}, Runner: &fakeRunner{err: domain.ErrSyncRunning}})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync/trigger", strings.NewReader(`{"type":"diff"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("code=%d, want 409", rec.Code)
	}
	var body errorDTO
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.Status != "sync_running" {
		t.Errorf("status=%s, want sync_running", body.Status)
	}
}

func TestSyncTrigger_BadType(t *testing.T) {
	h := newSyncTestServer(SyncOptions{Runs: &fakeRuns{}, Reader: &fakeReader{}, Runner: &fakeRunner{}})
	for _, payload := range []string{`{"type":"auto"}`, `{"type":""}`, `{}`, `not-json`} {
		req := httptest.NewRequest(http.MethodPost, "/v1/sync/trigger", strings.NewReader(payload))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("payload %q: code=%d, want 400", payload, rec.Code)
		}
	}
}
