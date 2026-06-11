package sync

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
	"github.com/r404r/JapanDigitalPostService/internal/store"
)

// fakeFetcher 按 URL 返回内存 CSV，未注册的 URL 返回 ErrSourceNotFound，
// 用于在不触网的情况下端到端验证引擎。
type fakeFetcher struct{ files map[string]string }

func (f *fakeFetcher) Fetch(_ context.Context, url string) (*SourceFile, error) {
	c, ok := f.files[url]
	if !ok {
		return nil, ErrSourceNotFound
	}
	return &SourceFile{
		URL:      url,
		CSV:      io.NopCloser(strings.NewReader(c)),
		Checksum: "fake-" + url,
		Size:     int64(len(c)),
	}, nil
}

type blockingFetcher struct {
	started chan struct{}
}

func (f *blockingFetcher) Fetch(ctx context.Context, url string) (*SourceFile, error) {
	close(f.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

const (
	rowA = `01101,"060  ","0600000","ホッカイドウ","サッポロシチュウオウク","イカニケイサイガナイバアイ","北海道","札幌市中央区","以下に掲載がない場合",0,0,0,0,0,0`
	rowB = `01101,"064  ","0640941","ホッカイドウ","サッポロシチュウオウク","アサヒガオカ","北海道","札幌市中央区","旭ケ丘",0,0,1,0,0,0`
	// rowBmod 与 rowB 同键（zip/jis/town/town_kana），仅 flag_multi_zip 变化 → updated。
	rowBmod = `01101,"064  ","0640941","ホッカイドウ","サッポロシチュウオウク","アサヒガオカ","北海道","札幌市中央区","旭ケ丘",1,0,1,0,1,3`
	rowC    = `15210,"948  ","9480013","ニイガタケン","トオカマチシ","カワハラチョウ","新潟県","十日町市","川原町",0,0,0,0,0,0`
	rowD    = `23202,"444  ","4440819","アイチケン","オカザキシ","オカザキエキマエ","愛知県","岡崎市","岡崎駅前",0,0,1,0,1,5`
	// rowKani / rowWasa：同一 (zip,jis,town)=6730012/28203/和坂 的两种合法读音，
	// 真实全量数据存在此对（Finding 2），并入 town_kana 键后应各自独立落库。
	rowKani = `28203,"673  ","6730012","ヒョウゴケン","アカシシ","カニガサカ","兵庫県","明石市","和坂",0,0,0,0,0,0`
	rowWasa = `28203,"673  ","6730012","ヒョウゴケン","アカシシ","ワサカ","兵庫県","明石市","和坂",0,0,0,0,0,0`
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(context.Background(), store.Options{Driver: "sqlite", DSN: dsn, ConnectTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func newTestEngine(t *testing.T, st *store.Store, files map[string]string) *Engine {
	t.Helper()
	e := NewEngine(st.Addresses(), st.SyncRuns(), st.Locker(), &fakeFetcher{files: files}, Options{
		FullURL:            "full",
		AddURLTemplate:     "add_%s",
		DelURLTemplate:     "del_%s",
		BatchSize:          2,
		FullPrune:          true,
		FullMinRows:        1,
		DiffFallbackFull:   true,
		DiffLookbackMonths: 3,
	}, nil)
	// 固定时钟到 2026-06-11，使差分窗口确定为 [2604,2605,2606]。
	e.now = func() time.Time { return time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC) }
	var seq int
	e.newID = func() string { seq++; return "run-" + string(rune('0'+seq)) }
	return e
}

func count(t *testing.T, st *store.Store) int64 {
	t.Helper()
	n, err := st.Addresses().Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEngineFullThenIdempotentRerun(t *testing.T) {
	st := openTestStore(t)
	full := strings.Join([]string{rowA, rowB, rowC}, "\n") + "\n"
	e := newTestEngine(t, st, map[string]string{"full": full})

	// 空库 → full。
	run, err := e.Run(context.Background(), domain.SyncAuto, domain.TriggerManual)
	if err != nil {
		t.Fatalf("full run: %v", err)
	}
	if run.Type != domain.SyncFull || run.Status != domain.StatusSuccess {
		t.Fatalf("run = %+v", run)
	}
	if run.RowsAdded != 3 || run.RowsTotal != 3 || run.RowsDeleted != 0 {
		t.Errorf("full counts a=%d t=%d d=%d, want 3/3/0", run.RowsAdded, run.RowsTotal, run.RowsDeleted)
	}
	if got := count(t, st); got != 3 {
		t.Fatalf("addresses = %d, want 3", got)
	}

	// 重跑 full → 全部 unchanged，计数稳定。
	run2, err := e.Run(context.Background(), domain.SyncFull, domain.TriggerManual)
	if err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if run2.RowsAdded != 0 || run2.RowsUpdated != 0 || run2.RowsDeleted != 0 {
		t.Errorf("rerun counts a=%d u=%d d=%d, want 0/0/0", run2.RowsAdded, run2.RowsUpdated, run2.RowsDeleted)
	}
	if got := count(t, st); got != 3 {
		t.Errorf("addresses after rerun = %d, want 3", got)
	}
}

func TestEngineFullPrunesRemovedRows(t *testing.T) {
	st := openTestStore(t)
	files := map[string]string{"full": strings.Join([]string{rowA, rowB, rowC}, "\n") + "\n"}
	e := newTestEngine(t, st, files)
	if _, err := e.Run(context.Background(), domain.SyncFull, domain.TriggerManual); err != nil {
		t.Fatal(err)
	}
	// 新全量文件移除 rowC → 应被剪除。
	files["full"] = strings.Join([]string{rowA, rowB}, "\n") + "\n"
	run, err := e.Run(context.Background(), domain.SyncFull, domain.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if run.RowsDeleted != 1 {
		t.Errorf("pruned = %d, want 1", run.RowsDeleted)
	}
	if got := count(t, st); got != 2 {
		t.Errorf("addresses = %d, want 2", got)
	}
}

func TestEngineDiffAddDelUpdate(t *testing.T) {
	st := openTestStore(t)
	files := map[string]string{
		"full": strings.Join([]string{rowA, rowB, rowC}, "\n") + "\n",
		// 2605 差分：删除 rowC，新增 rowD，修改 rowB。
		"del_2605": rowC + "\n",
		"add_2605": strings.Join([]string{rowBmod, rowD}, "\n") + "\n",
	}
	e := newTestEngine(t, st, files)
	if _, err := e.Run(context.Background(), domain.SyncFull, domain.TriggerManual); err != nil {
		t.Fatal(err)
	}

	// 非空库 → auto 走 diff，应用 2605。
	run, err := e.Run(context.Background(), domain.SyncAuto, domain.TriggerSchedule)
	if err != nil {
		t.Fatalf("diff run: %v", err)
	}
	if run.Type != domain.SyncDiff {
		t.Fatalf("type = %s, want diff", run.Type)
	}
	if run.DiffPeriod != "2605" {
		t.Errorf("DiffPeriod = %q, want 2605", run.DiffPeriod)
	}
	if run.RowsDeleted != 1 || run.RowsAdded != 1 || run.RowsUpdated != 1 {
		t.Errorf("diff counts d=%d a=%d u=%d, want 1/1/1", run.RowsDeleted, run.RowsAdded, run.RowsUpdated)
	}
	if got := count(t, st); got != 3 { // A, B(mod), D；C 被删
		t.Errorf("addresses = %d, want 3", got)
	}

	// 重放同一差分 → 幂等（删 0、增 0、改 0）。
	run2, err := e.Run(context.Background(), domain.SyncDiff, domain.TriggerSchedule)
	if err != nil {
		t.Fatal(err)
	}
	if run2.RowsDeleted != 0 || run2.RowsAdded != 0 || run2.RowsUpdated != 0 {
		t.Errorf("diff replay counts d=%d a=%d u=%d, want 0/0/0", run2.RowsDeleted, run2.RowsAdded, run2.RowsUpdated)
	}
}

func TestEngineDiffFallbackToFull(t *testing.T) {
	st := openTestStore(t)
	// 先 full 建库，再请求 diff，但窗口内无差分文件 → 回退 full。
	files := map[string]string{"full": strings.Join([]string{rowA, rowB}, "\n") + "\n"}
	e := newTestEngine(t, st, files)
	if _, err := e.Run(context.Background(), domain.SyncFull, domain.TriggerManual); err != nil {
		t.Fatal(err)
	}
	run, err := e.Run(context.Background(), domain.SyncDiff, domain.TriggerSchedule)
	if err != nil {
		t.Fatalf("diff fallback: %v", err)
	}
	if run.Type != domain.SyncFull {
		t.Errorf("type = %s, want full (fallback)", run.Type)
	}
	if run.Status != domain.StatusSuccess {
		t.Errorf("status = %s", run.Status)
	}
}

// TestEngineFullKeepsDistinctReadings 覆盖 Finding 2 的端到端路径：全量文件中同一
// (zip,jis,town) 的两种读音（且落入同一 upsert 分块）应各自独立落库，不被折叠、
// 不触发 SQLite "ON CONFLICT cannot affect row a second time"。
func TestEngineFullKeepsDistinctReadings(t *testing.T) {
	st := openTestStore(t)
	// rowKani 与 rowWasa 相邻，BatchSize=2 使二者落入同一分块。
	full := strings.Join([]string{rowKani, rowWasa, rowA}, "\n") + "\n"
	e := newTestEngine(t, st, map[string]string{"full": full})

	run, err := e.Run(context.Background(), domain.SyncFull, domain.TriggerManual)
	if err != nil {
		t.Fatalf("full run: %v", err)
	}
	if run.Status != domain.StatusSuccess {
		t.Fatalf("status = %s, err = %s", run.Status, run.ErrorMessage)
	}
	if run.RowsAdded != 3 || run.RowsTotal != 3 {
		t.Errorf("counts a=%d t=%d, want 3/3 (两读音各计一条)", run.RowsAdded, run.RowsTotal)
	}
	if got := count(t, st); got != 3 {
		t.Fatalf("addresses = %d, want 3 (和坂两读音 + rowA)", got)
	}

	// 重跑全量 → 全部 unchanged，计数稳定（不再像旧键那样稳定记 updated≥1）。
	run2, err := e.Run(context.Background(), domain.SyncFull, domain.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if run2.RowsUpdated != 0 || run2.RowsAdded != 0 || run2.RowsDeleted != 0 {
		t.Errorf("rerun counts a=%d u=%d d=%d, want 0/0/0", run2.RowsAdded, run2.RowsUpdated, run2.RowsDeleted)
	}
}

// TestMonthsWindow 覆盖 Finding 1：月末日期回退不得因短月归一化跳月/重复。
func TestMonthsWindow(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		n    int
		want []string
	}{
		{"march31_back3", time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC), 3, []string{"2601", "2602", "2603"}},
		{"may31_back3", time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC), 3, []string{"2603", "2604", "2605"}},
		{"jan31_crossyear", time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), 3, []string{"2511", "2512", "2601"}},
		{"jan30_crossyear", time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC), 2, []string{"2512", "2601"}},
		{"mar29_leapfeb", time.Date(2024, 3, 29, 0, 0, 0, 0, time.UTC), 2, []string{"2402", "2403"}},
		{"mid_month", time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC), 3, []string{"2604", "2605", "2606"}},
		{"single", time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC), 1, []string{"2607"}},
		{"dec31", time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), 4, []string{"2509", "2510", "2511", "2512"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := monthsWindow(c.now, c.n)
			if len(got) != len(c.want) {
				t.Fatalf("monthsWindow(%v,%d) = %v, want %v", c.now, c.n, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("monthsWindow(%v,%d) = %v, want %v", c.now, c.n, got, c.want)
				}
			}
		})
	}
}

func TestEngineConcurrentTriggerRejected(t *testing.T) {
	st := openTestStore(t)
	e := newTestEngine(t, st, map[string]string{"full": rowA + "\n"})

	// 手动占用锁，模拟已有同步在跑。
	release, ok, err := st.Locker().Acquire(context.Background(), "holder-x")
	if err != nil || !ok {
		t.Fatalf("acquire: ok=%v err=%v", ok, err)
	}
	defer release()

	_, err = e.Run(context.Background(), domain.SyncFull, domain.TriggerManual)
	if !errors.Is(err, domain.ErrSyncRunning) {
		t.Fatalf("want ErrSyncRunning, got %v", err)
	}
}

func TestEngineTriggerAsync(t *testing.T) {
	st := openTestStore(t)
	full := strings.Join([]string{rowA, rowB, rowC}, "\n") + "\n"
	e := newTestEngine(t, st, map[string]string{"full": full})

	// 立即返回 running 的运行记录（后台异步执行）。
	run, err := e.TriggerAsync(domain.SyncFull, domain.TriggerManual)
	if err != nil {
		t.Fatalf("TriggerAsync: %v", err)
	}
	if run.ID == "" || run.Status != domain.StatusRunning || run.Type != domain.SyncFull {
		t.Fatalf("returned run = %+v, want running/full with id", run)
	}

	// 轮询直到后台执行完成并落库为 success。
	deadline := time.Now().Add(5 * time.Second)
	for {
		latest, lerr := st.SyncRuns().Latest(context.Background())
		if lerr != nil {
			t.Fatalf("latest: %v", lerr)
		}
		if latest != nil && latest.Status == domain.StatusSuccess {
			if latest.ID != run.ID || latest.RowsAdded != 3 {
				t.Fatalf("final run id=%s added=%d, want %s/3", latest.ID, latest.RowsAdded, run.ID)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("async sync did not complete; latest=%+v", latest)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := count(t, st); got != 3 {
		t.Fatalf("addresses = %d, want 3", got)
	}

	// 完成后锁已释放：可再次触发（同步 Run 不应被 ErrSyncRunning 拒绝）。
	if _, err := e.Run(context.Background(), domain.SyncFull, domain.TriggerManual); err != nil {
		t.Fatalf("post-async run should succeed (lock released): %v", err)
	}
}

func TestEngineTriggerAsyncConcurrentRejected(t *testing.T) {
	st := openTestStore(t)
	e := newTestEngine(t, st, map[string]string{"full": rowA + "\n"})

	release, ok, err := st.Locker().Acquire(context.Background(), "holder-x")
	if err != nil || !ok {
		t.Fatalf("acquire: ok=%v err=%v", ok, err)
	}
	defer release()

	if _, err := e.TriggerAsync(domain.SyncFull, domain.TriggerManual); !errors.Is(err, domain.ErrSyncRunning) {
		t.Fatalf("want ErrSyncRunning, got %v", err)
	}
}

func TestEngineUploadFullCSVRecordsRunAndApplies(t *testing.T) {
	st := openTestStore(t)
	e := newTestEngine(t, st, nil)
	data := []byte(strings.Join([]string{rowA, rowB}, "\n") + "\n")

	run, err := e.UploadFull(context.Background(), "utf_ken_all.csv", data)
	if err != nil {
		t.Fatalf("UploadFull: %v", err)
	}
	if run.Type != domain.SyncFull || run.Trigger != domain.TriggerUpload || run.Status != domain.StatusSuccess {
		t.Fatalf("run = %+v, want full/upload/success", run)
	}
	if run.SourceURL != "upload:utf_ken_all.csv" || run.FileSize != int64(len(data)) || run.FileChecksum == "" {
		t.Fatalf("source/size/checksum = %q/%d/%q", run.SourceURL, run.FileSize, run.FileChecksum)
	}
	if run.RowsAdded != 2 || run.RowsTotal != 2 {
		t.Fatalf("counts added=%d total=%d, want 2/2", run.RowsAdded, run.RowsTotal)
	}
	if got := count(t, st); got != 2 {
		t.Fatalf("addresses = %d, want 2", got)
	}
}

func TestEngineUploadFullZip(t *testing.T) {
	st := openTestStore(t)
	e := newTestEngine(t, st, nil)
	data := zipCSV(t, "utf_ken_all.csv", []byte(rowA+"\n"))

	run, err := e.UploadFull(context.Background(), "ken_all.zip", data)
	if err != nil {
		t.Fatalf("UploadFull zip: %v", err)
	}
	if run.Status != domain.StatusSuccess || run.RowsAdded != 1 || run.SourceURL != "upload:ken_all.zip" {
		t.Fatalf("run = %+v, want successful zip upload", run)
	}
}

func TestEngineUploadFullRejectsBadEncodingAndRecordsFailure(t *testing.T) {
	st := openTestStore(t)
	e := newTestEngine(t, st, nil)
	data := []byte{0x82, 0xa0, '\n'} // invalid UTF-8 bytes representative of Shift-JIS input.

	run, err := e.UploadFull(context.Background(), "utf_ken_all.csv", data)
	if !errors.Is(err, ErrUploadEncoding) {
		t.Fatalf("err=%v, want ErrUploadEncoding", err)
	}
	if run == nil || run.Status != domain.StatusFailed || run.Trigger != domain.TriggerUpload {
		t.Fatalf("run=%+v, want failed upload run", run)
	}
	latest, lerr := st.SyncRuns().Latest(context.Background())
	if lerr != nil {
		t.Fatalf("Latest: %v", lerr)
	}
	if latest == nil || latest.Status != domain.StatusFailed || !strings.Contains(latest.ErrorMessage, "UTF-8") {
		t.Fatalf("latest=%+v, want recorded UTF-8 failure", latest)
	}
}

func TestEngineUploadFullConcurrentRejected(t *testing.T) {
	st := openTestStore(t)
	e := newTestEngine(t, st, nil)

	release, ok, err := st.Locker().Acquire(context.Background(), "holder-x")
	if err != nil || !ok {
		t.Fatalf("acquire: ok=%v err=%v", ok, err)
	}
	defer release()

	_, err = e.UploadFull(context.Background(), "utf_ken_all.csv", []byte(rowA+"\n"))
	if !errors.Is(err, domain.ErrSyncRunning) {
		t.Fatalf("want ErrSyncRunning, got %v", err)
	}
}

func zipCSV(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func TestEngineShutdownCancelsTriggerAsyncAndMarksRunFailed(t *testing.T) {
	st := openTestStore(t)
	fetcher := &blockingFetcher{started: make(chan struct{})}
	e := NewEngine(st.Addresses(), st.SyncRuns(), st.Locker(), fetcher, Options{
		FullURL:   "full",
		BatchSize: 2,
	}, nil)

	run, err := e.TriggerAsync(domain.SyncFull, domain.TriggerManual)
	if err != nil {
		t.Fatalf("TriggerAsync: %v", err)
	}
	select {
	case <-fetcher.started:
	case <-time.After(time.Second):
		t.Fatal("fetcher did not start")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	latest, err := st.SyncRuns().Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if latest == nil || latest.ID != run.ID {
		t.Fatalf("latest = %+v, want run %s", latest, run.ID)
	}
	if latest.Status != domain.StatusFailed {
		t.Fatalf("status = %s, want failed", latest.Status)
	}
	if !strings.Contains(latest.ErrorMessage, "context canceled") {
		t.Fatalf("error = %q, want context canceled", latest.ErrorMessage)
	}
	running, err := st.SyncRuns().CountRunning(context.Background())
	if err != nil {
		t.Fatalf("CountRunning: %v", err)
	}
	if running != 0 {
		t.Fatalf("running = %d, want 0", running)
	}
}

func TestSchedulerStopCancelsRunningSync(t *testing.T) {
	st := openTestStore(t)
	fetcher := &blockingFetcher{started: make(chan struct{})}
	e := NewEngine(st.Addresses(), st.SyncRuns(), st.Locker(), fetcher, Options{
		FullURL:   "full",
		BatchSize: 2,
	}, nil)

	sch, err := NewScheduler(e, "@every 1s", nil)
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	sch.Start()
	select {
	case <-fetcher.started:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("scheduled sync did not start")
	}

	stopCtx := sch.Stop()
	select {
	case <-stopCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("scheduler stop did not finish")
	}

	latest, err := st.SyncRuns().Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if latest == nil || latest.Status != domain.StatusFailed {
		t.Fatalf("latest = %+v, want failed run", latest)
	}
	if !strings.Contains(latest.ErrorMessage, "context canceled") {
		t.Fatalf("error = %q, want context canceled", latest.ErrorMessage)
	}
}
