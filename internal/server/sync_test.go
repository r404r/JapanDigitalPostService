package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/auth"
	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// ---- 测试替身 ----

// fakeReader 实现 domain.AddressReader：只关心 CountAll。
type fakeReader struct {
	count int
	err   error
}

func (f fakeReader) Search(context.Context, domain.AddressQuery) ([]domain.Address, int, error) {
	return nil, 0, nil
}
func (f fakeReader) CountAll(context.Context) (int, error) { return f.count, f.err }

// fakeRuns 实现 domain.SyncRunRepository（状态/历史读侧）。
type fakeRuns struct {
	list          []domain.SyncRun
	running       int64
	latestSuccess *domain.SyncRun
	lastLimit     int
	lastOffset    int
	err           error
}

func (f *fakeRuns) Create(context.Context, *domain.SyncRun) error   { return nil }
func (f *fakeRuns) Update(context.Context, *domain.SyncRun) error   { return nil }
func (f *fakeRuns) Latest(context.Context) (*domain.SyncRun, error) { return nil, nil }
func (f *fakeRuns) LatestSuccess(context.Context) (*domain.SyncRun, error) {
	return f.latestSuccess, f.err
}
func (f *fakeRuns) List(_ context.Context, limit, offset int) ([]domain.SyncRun, error) {
	f.lastLimit, f.lastOffset = limit, offset
	return f.list, f.err
}
func (f *fakeRuns) CountRunning(context.Context) (int64, error) { return f.running, f.err }
func (f *fakeRuns) MarkRunningFailed(context.Context, string, time.Time) (int64, error) {
	return 0, nil
}

// fakeTrigger 实现 SyncTrigger。
type fakeTrigger struct {
	run     *domain.SyncRun
	err     error
	called  bool
	gotType domain.SyncType
}

func (f *fakeTrigger) TriggerAsync(reqType domain.SyncType, _ domain.SyncTrigger) (*domain.SyncRun, error) {
	f.called = true
	f.gotType = reqType
	return f.run, f.err
}

// newSyncRouter 装配一个带真实 auth 鉴权的 router（admin bootstrap token 注入），
// 返回 handler、admin 与 read 明文 token。
func newSyncRouter(t *testing.T, opts Options) (http.Handler, string, string) {
	t.Helper()
	const adminTok = "jdps_test_admin_bootstrap_value_0001"
	authSvc := auth.NewService(auth.NewMemoryStore(), time.Now)
	if err := authSvc.EnsureBootstrap(context.Background(), adminTok); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	issued, err := authSvc.Issue(context.Background(), auth.IssueParams{Name: "read", Scope: domain.ScopeRead})
	if err != nil {
		t.Fatalf("issue read: %v", err)
	}
	opts.Auth = authSvc
	return NewRouter(opts), adminTok, issued.Plaintext
}

func doAuth(t *testing.T, h http.Handler, method, path, bearer, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestSyncStatus(t *testing.T) {
	finished := time.Date(2026, 6, 11, 3, 0, 0, 0, time.UTC)
	runs := &fakeRuns{
		running:       0,
		latestSuccess: &domain.SyncRun{Type: domain.SyncFull, Status: domain.StatusSuccess, FinishedAt: &finished},
	}
	h, admin, read := newSyncRouter(t, Options{AddressReader: fakeReader{count: 42}, SyncRuns: runs})

	rec := doAuth(t, h, "GET", "/v1/sync/status", read, "")
	if rec.Code != 200 {
		t.Fatalf("code=%d, want 200 (read scope)", rec.Code)
	}
	var body syncStatusDTO
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.TotalAddresses != 42 || body.Running {
		t.Errorf("got total=%d running=%v, want 42/false", body.TotalAddresses, body.Running)
	}
	if body.LastType == nil || *body.LastType != "full" {
		t.Errorf("last_type=%v, want full", body.LastType)
	}
	if body.LastSuccessAt == nil || !body.LastSuccessAt.Equal(finished) {
		t.Errorf("last_success_at=%v, want %v", body.LastSuccessAt, finished)
	}

	// admin 也能读（admin 隐含 read）。
	if rec := doAuth(t, h, "GET", "/v1/sync/status", admin, ""); rec.Code != 200 {
		t.Errorf("admin status code=%d, want 200", rec.Code)
	}
	// 无 token → 401。
	if rec := doAuth(t, h, "GET", "/v1/sync/status", "", ""); rec.Code != 401 {
		t.Errorf("no-auth status code=%d, want 401", rec.Code)
	}
}

func TestSyncStatus_NoSuccessYet(t *testing.T) {
	runs := &fakeRuns{running: 1, latestSuccess: nil}
	h, _, read := newSyncRouter(t, Options{AddressReader: fakeReader{count: 0}, SyncRuns: runs})
	rec := doAuth(t, h, "GET", "/v1/sync/status", read, "")
	var body syncStatusDTO
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if !body.Running {
		t.Errorf("running=%v, want true", body.Running)
	}
	if body.LastSuccessAt != nil || body.LastType != nil {
		t.Errorf("last_success_at/last_type should be null when no success yet: %v / %v", body.LastSuccessAt, body.LastType)
	}
}

func TestSyncRuns_Pagination(t *testing.T) {
	runs := &fakeRuns{list: []domain.SyncRun{
		{ID: "r1", Type: domain.SyncDiff, Status: domain.StatusSuccess, DiffPeriod: "2606"},
		{ID: "r2", Type: domain.SyncFull, Status: domain.StatusFailed, ErrorMessage: "boom"},
	}}
	h, _, read := newSyncRouter(t, Options{AddressReader: fakeReader{}, SyncRuns: runs})

	rec := doAuth(t, h, "GET", "/v1/sync/runs?limit=5&offset=10", read, "")
	if rec.Code != 200 {
		t.Fatalf("code=%d, want 200", rec.Code)
	}
	if runs.lastLimit != 5 || runs.lastOffset != 10 {
		t.Errorf("repo got limit=%d offset=%d, want 5/10", runs.lastLimit, runs.lastOffset)
	}
	var out []syncRunDTO
	_ = json.NewDecoder(rec.Body).Decode(&out)
	if len(out) != 2 {
		t.Fatalf("len=%d, want 2", len(out))
	}
	// diff_period 非空映射为字符串；error_message 在 success 行为 null、failed 行非 null。
	if out[0].DiffPeriod == nil || *out[0].DiffPeriod != "2606" {
		t.Errorf("r1 diff_period=%v, want 2606", out[0].DiffPeriod)
	}
	if out[0].ErrorMessage != nil {
		t.Errorf("r1 error_message=%v, want null", out[0].ErrorMessage)
	}
	if out[1].ErrorMessage == nil || *out[1].ErrorMessage != "boom" {
		t.Errorf("r2 error_message=%v, want boom", out[1].ErrorMessage)
	}
}

func TestSyncRuns_BadLimit(t *testing.T) {
	h, _, read := newSyncRouter(t, Options{AddressReader: fakeReader{}, SyncRuns: &fakeRuns{}})
	rec := doAuth(t, h, "GET", "/v1/sync/runs?limit=abc", read, "")
	if rec.Code != 400 {
		t.Fatalf("code=%d, want 400", rec.Code)
	}
	var body errorDTO
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.Status != "invalid_request" {
		t.Errorf("status=%s, want invalid_request", body.Status)
	}
}

func TestSyncTrigger_AdminAccepted(t *testing.T) {
	tr := &fakeTrigger{run: &domain.SyncRun{ID: "run-1", Type: domain.SyncFull, Status: domain.StatusRunning}}
	h, admin, _ := newSyncRouter(t, Options{SyncTrigger: tr})

	rec := doAuth(t, h, "POST", "/v1/sync/trigger", admin, `{"type":"full"}`)
	if rec.Code != 202 {
		t.Fatalf("code=%d, want 202", rec.Code)
	}
	if !tr.called || tr.gotType != domain.SyncFull {
		t.Errorf("trigger called=%v type=%v, want true/full", tr.called, tr.gotType)
	}
	var body syncRunDTO
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.ID != "run-1" || body.Status != "running" {
		t.Errorf("response id=%s status=%s, want run-1/running", body.ID, body.Status)
	}
}

func TestSyncTrigger_AutoAccepted(t *testing.T) {
	// auto 是合法入参：handler 原样透传给引擎，引擎按库空与否解析为 full/diff，
	// 202 返回解析后的真实 run（type 始终是 full 或 diff，不会是 auto）。
	tr := &fakeTrigger{run: &domain.SyncRun{ID: "run-auto", Type: domain.SyncDiff, Status: domain.StatusRunning}}
	h, admin, _ := newSyncRouter(t, Options{SyncTrigger: tr})

	rec := doAuth(t, h, "POST", "/v1/sync/trigger", admin, `{"type":"auto"}`)
	if rec.Code != 202 {
		t.Fatalf("code=%d, want 202", rec.Code)
	}
	if !tr.called || tr.gotType != domain.SyncAuto {
		t.Errorf("trigger called=%v type=%v, want true/auto", tr.called, tr.gotType)
	}
	var body syncRunDTO
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.ID != "run-auto" || body.Type != "diff" {
		t.Errorf("response id=%s type=%s, want run-auto/diff (resolved)", body.ID, body.Type)
	}
}

func TestSyncTrigger_ReadForbidden(t *testing.T) {
	tr := &fakeTrigger{run: &domain.SyncRun{ID: "x"}}
	h, _, read := newSyncRouter(t, Options{SyncTrigger: tr})
	rec := doAuth(t, h, "POST", "/v1/sync/trigger", read, `{"type":"full"}`)
	if rec.Code != 403 {
		t.Fatalf("code=%d, want 403 (read cannot trigger)", rec.Code)
	}
	if tr.called {
		t.Error("trigger should not be called when scope insufficient")
	}
}

func TestSyncTrigger_NoAuth(t *testing.T) {
	h, _, _ := newSyncRouter(t, Options{SyncTrigger: &fakeTrigger{run: &domain.SyncRun{}}})
	if rec := doAuth(t, h, "POST", "/v1/sync/trigger", "", `{"type":"full"}`); rec.Code != 401 {
		t.Fatalf("code=%d, want 401", rec.Code)
	}
}

func TestSyncTrigger_Conflict(t *testing.T) {
	tr := &fakeTrigger{err: domain.ErrSyncRunning}
	h, admin, _ := newSyncRouter(t, Options{SyncTrigger: tr})
	rec := doAuth(t, h, "POST", "/v1/sync/trigger", admin, `{"type":"diff"}`)
	if rec.Code != 409 {
		t.Fatalf("code=%d, want 409", rec.Code)
	}
	var body errorDTO
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.Status != "sync_running" {
		t.Errorf("status=%s, want sync_running", body.Status)
	}
}

func TestSyncTrigger_BadType(t *testing.T) {
	tr := &fakeTrigger{run: &domain.SyncRun{}}
	h, admin, _ := newSyncRouter(t, Options{SyncTrigger: tr})
	for _, payload := range []string{`{"type":"weekly"}`, `{"type":""}`, `{}`, `{"type":"full","x":1}`, `not json`} {
		rec := doAuth(t, h, "POST", "/v1/sync/trigger", admin, payload)
		if rec.Code != 400 {
			t.Errorf("payload %q: code=%d, want 400", payload, rec.Code)
		}
	}
	if tr.called {
		t.Error("trigger should not be called for invalid input")
	}
}
