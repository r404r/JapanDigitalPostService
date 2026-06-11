// Package e2e 跨包端到端验证后端装配：同步引擎（fixture，不触网）落库 →
// 只读查询 API → 同步状态/历史/触发 API → token 管理 API，全部经真实 HTTP 路由
// 与真实 Bearer 鉴权驱动。这是发布验收要求的「可复用端到端测试」：把 sync /
// query / sync-status / auth 串成一条链路，并覆盖无 token / 无效 token 的 401。
package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/auth"
	"github.com/r404r/JapanDigitalPostService/internal/crypto"
	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"github.com/r404r/JapanDigitalPostService/internal/query"
	"github.com/r404r/JapanDigitalPostService/internal/server"
	"github.com/r404r/JapanDigitalPostService/internal/store"
	syncpkg "github.com/r404r/JapanDigitalPostService/internal/sync"
)

// 小 fixture：千代田（1 条）+ あま市 4980000（2 条，同邮编多町域）。
const fixtureCSV = `13101,"100  ","1000001","トウキョウト","チヨダク","チヨダ","東京都","千代田区","千代田",0,0,0,0,0,0
23100,"498  ","4980000","アイチケン","アマシ","ニシキオリ","愛知県","あま市","錦織",0,0,0,1,0,0
23100,"498  ","4980000","アイチケン","アマシ","ジマンダ","愛知県","あま市","蜂須賀",0,0,0,1,0,0
`

const fullURL = "https://example.test/utf_ken_all.zip"

// fakeFetcher 按 URL 返回内存 CSV，未注册返回 ErrSourceNotFound（不触网）。
type fakeFetcher struct{ files map[string]string }

func (f *fakeFetcher) Fetch(_ context.Context, url string) (*syncpkg.SourceFile, error) {
	c, ok := f.files[url]
	if !ok {
		return nil, syncpkg.ErrSourceNotFound
	}
	return &syncpkg.SourceFile{
		URL:      url,
		CSV:      io.NopCloser(strings.NewReader(c)),
		Checksum: "fake",
		Size:     int64(len(c)),
	}, nil
}

// testServer 是装配后的端到端测试夹具。
type testServer struct {
	ts     *httptest.Server
	engine *syncpkg.Engine
	admin  string // bootstrap admin 明文 token
	read   string // read scope 明文 token
}

// buildServer 复刻 cmd/server 的装配（含真实 Bearer 鉴权、同步状态/触发端点；
// 不含进程内调度），返回夹具。
func buildServer(t *testing.T) *testServer {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "e2e.db")
	st, err := store.Open(context.Background(), store.Options{Driver: "sqlite", DSN: dsn, ConnectTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	engine := syncpkg.NewEngine(st.Addresses(), st.SyncRuns(), st.Locker(),
		&fakeFetcher{files: map[string]string{fullURL: fixtureCSV}},
		syncpkg.Options{FullURL: fullURL, BatchSize: 2, FullPrune: true, FullMinRows: 1}, nil)

	sqlDB, err := st.DB().DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	reader := store.NewAddressReadRepo(sqlDB)
	svc := query.NewService(reader, 20, 1000)

	const adminToken = "jdps_e2e_admin_bootstrap_token_value_0001"
	authSvc := auth.NewService(auth.NewMemoryStore(), time.Now)
	if err := authSvc.EnsureBootstrap(context.Background(), adminToken); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	readIssued, err := authSvc.Issue(context.Background(), auth.IssueParams{Name: "e2e-read", Scope: domain.ScopeRead})
	if err != nil {
		t.Fatalf("issue read token: %v", err)
	}

	router := server.NewRouter(server.Options{
		QueryService: svc,
		QueryTimeout: 2 * time.Second,
		AuthMiddleware: func(next http.Handler) http.Handler {
			return authSvc.RequireScope(domain.ScopeRead, next)
		},
	})
	th := auth.NewHandlers(authSvc)
	syncH := server.NewSyncHandlers(server.SyncOptions{
		Runs:         st.SyncRuns(),
		Reader:       reader,
		Runner:       engine,
		QueryTimeout: 2 * time.Second,
	})

	mux := http.NewServeMux()
	mux.Handle("GET /v1/health", router)
	mux.Handle("GET /v1/addresses", router)
	mux.Handle("GET /v1/addresses/{zipcode}", router)
	mux.Handle("GET /v1/sync/status", authSvc.RequireScope(domain.ScopeRead, http.HandlerFunc(syncH.GetStatus)))
	mux.Handle("GET /v1/sync/runs", authSvc.RequireScope(domain.ScopeRead, http.HandlerFunc(syncH.ListRuns)))
	mux.Handle("POST /v1/sync/trigger", authSvc.RequireScope(domain.ScopeAdmin, http.HandlerFunc(syncH.Trigger)))
	mux.Handle("POST /v1/tokens", authSvc.RequireScope(domain.ScopeAdmin, http.HandlerFunc(th.CreateToken)))
	mux.Handle("GET /v1/tokens", authSvc.RequireScope(domain.ScopeAdmin, http.HandlerFunc(th.ListTokens)))
	mux.Handle("DELETE /v1/tokens/{id}", authSvc.RequireScope(domain.ScopeAdmin, http.HandlerFunc(th.RevokeToken)))

	cipher, err := crypto.New(crypto.ModeNone, "", "")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	ts := httptest.NewServer(cipher.Middleware(mux))
	t.Cleanup(ts.Close)
	return &testServer{ts: ts, engine: engine, admin: adminToken, read: readIssued.Plaintext}
}

func req(t *testing.T, ts *httptest.Server, method, path, bearer string) (*http.Response, map[string]any) {
	t.Helper()
	r, _ := http.NewRequest(method, ts.URL+path, nil)
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := ts.Client().Do(r)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	var body map[string]any
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(b) > 0 {
		_ = json.Unmarshal(b, &body)
	}
	return resp, body
}

// reqArray 与 req 类似，但解析 JSON 数组响应（/v1/sync/runs 返回数组）。
func reqArray(t *testing.T, ts *httptest.Server, method, path, bearer string) (*http.Response, []map[string]any) {
	t.Helper()
	r, _ := http.NewRequest(method, ts.URL+path, nil)
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := ts.Client().Do(r)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	var body []map[string]any
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(b) > 0 {
		_ = json.Unmarshal(b, &body)
	}
	return resp, body
}

// TestE2E_SyncQueryToken 跑通：health → 同步前查询鉴权 → 全量同步（fixture）→
// 按邮编查询命中 → token 鉴权（无 token 401、admin 发行/列表/吊销）。
func TestE2E_SyncQueryToken(t *testing.T) {
	srv := buildServer(t)
	ts, admin, read := srv.ts, srv.admin, srv.read

	// 1. health（免认证）。
	if resp, body := req(t, ts, "GET", "/v1/health", ""); resp.StatusCode != 200 || body["status"] != "ok" {
		t.Fatalf("health: code=%d body=%v", resp.StatusCode, body)
	}

	// 2. 查询端点真实鉴权：无 token / 无效 token → 401。
	if resp, _ := req(t, ts, "GET", "/v1/addresses?zipcode=4980000", ""); resp.StatusCode != 401 {
		t.Fatalf("addresses no-auth: code=%d, want 401", resp.StatusCode)
	}
	if resp, _ := req(t, ts, "GET", "/v1/addresses?zipcode=4980000", "jdps_bogus_token"); resp.StatusCode != 401 {
		t.Fatalf("addresses bad-token: code=%d, want 401", resp.StatusCode)
	}

	// 3. 同步前查询（带 read token）：库为空，命中 0。
	if _, body := req(t, ts, "GET", "/v1/addresses?zipcode=4980000", read); body["total_count"].(float64) != 0 {
		t.Fatalf("pre-sync total_count=%v, want 0", body["total_count"])
	}

	// 4. 触发全量同步（fixture，3 行）。
	run, err := srv.engine.Run(context.Background(), domain.SyncFull, domain.TriggerManual)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if run.Status != domain.StatusSuccess || run.RowsAdded != 3 {
		t.Fatalf("sync run: status=%s added=%d, want success/3", run.Status, run.RowsAdded)
	}

	// 5. 同步后查询（带 read token）：4980000 同邮编多町域 → 2 条；千代田精确 → address_count=1。
	if resp, body := req(t, ts, "GET", "/v1/addresses?zipcode=4980000", read); resp.StatusCode != 200 || body["total_count"].(float64) != 2 {
		t.Fatalf("post-sync 4980000: code=%d total=%v, want 200/2", resp.StatusCode, body["total_count"])
	}
	if resp, body := req(t, ts, "GET", "/v1/addresses/1000001", read); resp.StatusCode != 200 || body["address_count"].(float64) != 1 {
		t.Fatalf("zip 1000001: code=%d address_count=%v, want 200/1", resp.StatusCode, body["address_count"])
	}

	// 6. token 端点鉴权：无 token → 401。
	if resp, _ := req(t, ts, "GET", "/v1/tokens", ""); resp.StatusCode != 401 {
		t.Fatalf("tokens no-auth: code=%d, want 401", resp.StatusCode)
	}

	// 7. admin 发行一个 read token → 201，返回一次性明文。
	resp, body := postJSON(t, ts, "/v1/tokens", admin, `{"name":"e2e-read2","scope":"read"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("create token: code=%d body=%v", resp.StatusCode, body)
	}
	id, _ := body["id"].(string)
	plaintext, _ := body["token"].(string)
	if id == "" || plaintext == "" {
		t.Fatalf("create token missing id/token: %v", body)
	}

	// 8. admin 列表能看到该 token（脱敏，不含明文/hash）。
	if resp, _ := req(t, ts, "GET", "/v1/tokens", admin); resp.StatusCode != 200 {
		t.Fatalf("list tokens: code=%d", resp.StatusCode)
	}

	// 9. 吊销该 token → 204。
	if resp, _ := req(t, ts, "DELETE", "/v1/tokens/"+id, admin); resp.StatusCode != 204 {
		t.Fatalf("revoke token: code=%d", resp.StatusCode)
	}
}

// TestE2E_SyncStatusRunsTrigger 覆盖 /v1/sync/* 三端点的可达性、鉴权与语义：
// 触发同步 → status/runs 反映最新结果。
func TestE2E_SyncStatusRunsTrigger(t *testing.T) {
	srv := buildServer(t)
	ts, admin, read := srv.ts, srv.admin, srv.read

	// 1. 鉴权边界：status/runs 无 token → 401；trigger 用 read token → 403（需 admin）。
	if resp, _ := req(t, ts, "GET", "/v1/sync/status", ""); resp.StatusCode != 401 {
		t.Fatalf("status no-auth: code=%d, want 401", resp.StatusCode)
	}
	if resp, _ := req(t, ts, "GET", "/v1/sync/runs", "jdps_bogus"); resp.StatusCode != 401 {
		t.Fatalf("runs bad-token: code=%d, want 401", resp.StatusCode)
	}
	if resp, _ := postJSON(t, ts, "/v1/sync/trigger", read, `{"type":"full"}`); resp.StatusCode != 403 {
		t.Fatalf("trigger read-scope: code=%d, want 403", resp.StatusCode)
	}

	// 2. 初始 status（read token）：库为空，无成功记录。
	resp, body := req(t, ts, "GET", "/v1/sync/status", read)
	if resp.StatusCode != 200 {
		t.Fatalf("status: code=%d", resp.StatusCode)
	}
	if body["total_addresses"].(float64) != 0 || body["running"].(bool) {
		t.Fatalf("initial status=%v, want 0 addresses / not running", body)
	}
	if body["last_success_at"] != nil || body["last_type"] != nil {
		t.Fatalf("initial status should have nil last_success_at/last_type: %v", body)
	}

	// 3. admin 手动触发全量同步 → 202，返回完成的 run（fixture，3 行）。
	resp, body = postJSON(t, ts, "/v1/sync/trigger", admin, `{"type":"full"}`)
	if resp.StatusCode != 202 {
		t.Fatalf("trigger: code=%d body=%v, want 202", resp.StatusCode, body)
	}
	if body["status"] != "success" || body["type"] != "full" || body["trigger"] != "manual" || body["rows_added"].(float64) != 3 {
		t.Fatalf("trigger run=%v, want success/full/manual/3", body)
	}

	// 4. 非法 type → 400。
	if resp, _ := postJSON(t, ts, "/v1/sync/trigger", admin, `{"type":"auto"}`); resp.StatusCode != 400 {
		t.Fatalf("trigger bad type: code=%d, want 400", resp.StatusCode)
	}

	// 5. status 反映最新结果：3 条地址、最近成功类型 full。
	resp, body = req(t, ts, "GET", "/v1/sync/status", read)
	if resp.StatusCode != 200 || body["total_addresses"].(float64) != 3 {
		t.Fatalf("post-trigger status: code=%d total=%v, want 200/3", resp.StatusCode, body["total_addresses"])
	}
	if body["last_type"] != "full" || body["last_success_at"] == nil {
		t.Fatalf("post-trigger status last_type=%v last_success_at=%v, want full/non-nil", body["last_type"], body["last_success_at"])
	}

	// 6. runs 历史（read token）：至少一条，最新为成功的 full/manual。
	rresp, runs := reqArray(t, ts, "GET", "/v1/sync/runs?limit=10", read)
	if rresp.StatusCode != 200 || len(runs) == 0 {
		t.Fatalf("runs: code=%d len=%d, want 200/>=1", rresp.StatusCode, len(runs))
	}
	if runs[0]["status"] != "success" || runs[0]["type"] != "full" || runs[0]["trigger"] != "manual" {
		t.Fatalf("latest run=%v, want success/full/manual", runs[0])
	}
}

func postJSON(t *testing.T, ts *httptest.Server, path, bearer, payload string) (*http.Response, map[string]any) {
	t.Helper()
	r, _ := http.NewRequest("POST", ts.URL+path, strings.NewReader(payload))
	r.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := ts.Client().Do(r)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	var body map[string]any
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(b) > 0 {
		_ = json.Unmarshal(b, &body)
	}
	return resp, body
}
