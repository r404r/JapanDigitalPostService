// Package e2e 跨包端到端验证后端装配：经真实 HTTP 路由（与 cmd/server 同一
// internal/server.NewRouter 装配）串起 Bearer 鉴权 → 手动同步触发（fixture，不
// 触网）→ 同步落库 → 只读查询 → 同步状态/历史 → token 管理。这是发布验收要求的
// 「可复用端到端测试」：覆盖 spec §5.1 鉴权边界（无 token 401 → read 查询成功 →
// read 触发 403 → admin 触发 → status/runs 可见结果）。
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

// buildServer 复刻 cmd/server 的装配（经 server.NewRouter，含真实 Bearer 鉴权 +
// 同步触发端点；不含进程内调度），返回 httptest server 与 bootstrap 明文 admin token。
func buildServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "e2e.db")
	st, err := store.Open(context.Background(), store.Options{Driver: "sqlite", DSN: dsn, ConnectTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// 同步引擎（fixture fetcher，与触发端点共享同一 Store）。
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

	router := server.NewRouter(server.Options{
		QueryService:  svc,
		QueryTimeout:  2 * time.Second,
		AddressReader: reader,
		SyncRuns:      st.SyncRuns(),
		SyncTrigger:   engine,
		Auth:          authSvc,
		TokenHandlers: auth.NewHandlers(authSvc),
	})

	cipher, err := crypto.New(crypto.ModeNone, "", "")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	ts := httptest.NewServer(cipher.Middleware(router))
	t.Cleanup(ts.Close)
	return ts, adminToken
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
	return resp, decodeObj(resp)
}

func decodeObj(resp *http.Response) map[string]any {
	var body map[string]any
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(b) > 0 {
		_ = json.Unmarshal(b, &body)
	}
	return body
}

// TestE2E_AuthSyncQuery 跑通 spec §5.1 鉴权边界与同步装配：
// health 公开 → addresses 无 token 401 → admin 发行 read token → read 查询成功 →
// read 触发 403 → admin 触发（异步 full，fixture 3 行）→ 轮询 runs 至 success →
// read 查询命中 → read 看 status（total_addresses=3 / last_type=full）→ admin 吊销。
func TestE2E_AuthSyncQuery(t *testing.T) {
	ts, admin := buildServer(t)

	// 1. health（公开）。
	if resp, body := req(t, ts, "GET", "/v1/health", ""); resp.StatusCode != 200 || body["status"] != "ok" {
		t.Fatalf("health: code=%d body=%v", resp.StatusCode, body)
	}

	// 2. 查询无 token → 401（占位中间件已替换为真实鉴权）。
	if resp, _ := req(t, ts, "GET", "/v1/addresses?zipcode=4980000", ""); resp.StatusCode != 401 {
		t.Fatalf("addresses no-auth: code=%d, want 401", resp.StatusCode)
	}

	// 3. admin 发行一个 read token。
	resp, body := postJSON(t, ts, "/v1/tokens", admin, `{"name":"e2e-read","scope":"read"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("create read token: code=%d body=%v", resp.StatusCode, body)
	}
	readID, _ := body["id"].(string)
	readTok, _ := body["token"].(string)
	if readID == "" || readTok == "" {
		t.Fatalf("create token missing id/token: %v", body)
	}

	// 4. read token 查询成功（同步前命中 0）。
	if resp, body := req(t, ts, "GET", "/v1/addresses?zipcode=4980000", readTok); resp.StatusCode != 200 || body["total_count"].(float64) != 0 {
		t.Fatalf("pre-sync query: code=%d total=%v, want 200/0", resp.StatusCode, body["total_count"])
	}

	// 5. read token 触发同步 → 403（仅 admin 可触发）。
	if resp, _ := postJSON(t, ts, "/v1/sync/trigger", readTok, `{"type":"full"}`); resp.StatusCode != 403 {
		t.Fatalf("read trigger: code=%d, want 403", resp.StatusCode)
	}

	// 6. admin 触发全量同步 → 202，异步执行，返回 running 的 run id。
	resp, body = postJSON(t, ts, "/v1/sync/trigger", admin, `{"type":"full"}`)
	if resp.StatusCode != 202 {
		t.Fatalf("admin trigger: code=%d body=%v, want 202", resp.StatusCode, body)
	}
	runID, _ := body["id"].(string)
	if runID == "" || body["status"] != "running" {
		t.Fatalf("trigger response: id=%q status=%v, want non-empty/running", runID, body["status"])
	}

	// 7. 轮询 /sync/runs（read 可见）直到该 run success。
	waitForSuccess(t, ts, readTok, runID)

	// 8. 同步后查询：4980000 同邮编多町域 → 2；千代田精确 → address_count=1。
	if resp, body := req(t, ts, "GET", "/v1/addresses?zipcode=4980000", readTok); resp.StatusCode != 200 || body["total_count"].(float64) != 2 {
		t.Fatalf("post-sync 4980000: code=%d total=%v, want 200/2", resp.StatusCode, body["total_count"])
	}
	if resp, body := req(t, ts, "GET", "/v1/addresses/1000001", readTok); resp.StatusCode != 200 || body["address_count"].(float64) != 1 {
		t.Fatalf("zip 1000001: code=%d address_count=%v, want 200/1", resp.StatusCode, body["address_count"])
	}

	// 9. /sync/status（read 可见）：total_addresses=3，running=false，last_type=full。
	resp, body = req(t, ts, "GET", "/v1/sync/status", readTok)
	if resp.StatusCode != 200 {
		t.Fatalf("sync status: code=%d", resp.StatusCode)
	}
	if body["total_addresses"].(float64) != 3 {
		t.Errorf("total_addresses=%v, want 3", body["total_addresses"])
	}
	if body["running"] != false {
		t.Errorf("running=%v, want false", body["running"])
	}
	if body["last_type"] != "full" {
		t.Errorf("last_type=%v, want full", body["last_type"])
	}
	if body["last_success_at"] == nil {
		t.Errorf("last_success_at is null, want a timestamp")
	}

	// 10. token 管理仍仅 admin：read token 列表 → 403；admin 吊销 read token → 204。
	if resp, _ := req(t, ts, "GET", "/v1/tokens", readTok); resp.StatusCode != 403 {
		t.Fatalf("read list tokens: code=%d, want 403", resp.StatusCode)
	}
	if resp, _ := req(t, ts, "DELETE", "/v1/tokens/"+readID, admin); resp.StatusCode != 204 {
		t.Fatalf("revoke token: code=%d, want 204", resp.StatusCode)
	}
}

// waitForSuccess 轮询 /sync/runs 直到 runID 状态为 success（异步触发的同步在后台执行）。
func waitForSuccess(t *testing.T, ts *httptest.Server, bearer, runID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, _ := http.NewRequest("GET", ts.URL+"/v1/sync/runs", nil)
		r.Header.Set("Authorization", "Bearer "+bearer)
		resp, err := ts.Client().Do(r)
		if err != nil {
			t.Fatalf("list runs: %v", err)
		}
		var runs []map[string]any
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		_ = json.Unmarshal(b, &runs)
		for _, run := range runs {
			if run["id"] == runID {
				switch run["status"] {
				case "success":
					return
				case "failed":
					t.Fatalf("sync run %s failed: %v", runID, run["error_message"])
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("sync run %s did not reach success within deadline", runID)
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
	return resp, decodeObj(resp)
}
