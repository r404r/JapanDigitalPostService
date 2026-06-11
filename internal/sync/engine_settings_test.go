package sync

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// recordingFetcher 记录最近一次 Fetch 的 URL 与被注入的重试次数；实现
// retryConfigurable，用于断言引擎按有效配置驱动下载。
type recordingFetcher struct {
	csv         string
	lastURL     string
	maxRetrySet int
	retryCalled bool
}

func (f *recordingFetcher) Fetch(_ context.Context, url string) (*SourceFile, error) {
	f.lastURL = url
	return &SourceFile{
		URL:      url,
		CSV:      io.NopCloser(strings.NewReader(f.csv)),
		Checksum: "rec-" + url,
		Size:     int64(len(f.csv)),
	}, nil
}

func (f *recordingFetcher) SetMaxRetry(n int) {
	f.retryCalled = true
	f.maxRetrySet = n
}

// fakeResolver 返回预置的有效配置或错误。
type fakeResolver struct {
	settings domain.EffectiveSyncSettings
	err      error
}

func (r fakeResolver) ResolveSyncSettings(context.Context) (domain.EffectiveSyncSettings, error) {
	return r.settings, r.err
}

// Given 注入 resolver（自定义 URL+重试）When 执行全量同步 Then 用解析后的 URL 下载、
// 并把解析后的重试次数注入 fetcher（管理画面配置每次运行前生效）。
func TestEngine_ResolverOverridesURLAndRetry(t *testing.T) {
	st := openTestStore(t)
	rec := &recordingFetcher{csv: rowA + "\n"}
	e := NewEngine(st.Addresses(), st.SyncRuns(), st.Locker(), rec,
		Options{FullURL: "static-url", BatchSize: 2, FullMinRows: 1}, nil)
	e.UseSettingsResolver(fakeResolver{settings: domain.EffectiveSyncSettings{
		ScrapeFullURL: "https://www.post.japanpost.jp/resolved.zip", DownloadMaxRetry: 4,
	}})

	run, err := e.Run(context.Background(), domain.SyncFull, domain.TriggerManual)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.Status != domain.StatusSuccess {
		t.Fatalf("status=%s, want success", run.Status)
	}
	if rec.lastURL != "https://www.post.japanpost.jp/resolved.zip" {
		t.Errorf("fetched URL=%q, want resolved URL", rec.lastURL)
	}
	if !rec.retryCalled || rec.maxRetrySet != 4 {
		t.Errorf("retry injected=%v value=%d, want true/4", rec.retryCalled, rec.maxRetrySet)
	}
}

// Given resolver 解析失败 When 执行同步 Then 回退到静态 opt.FullURL，不打断同步。
func TestEngine_ResolverErrorFallsBackToStatic(t *testing.T) {
	st := openTestStore(t)
	rec := &recordingFetcher{csv: rowA + "\n"}
	e := NewEngine(st.Addresses(), st.SyncRuns(), st.Locker(), rec,
		Options{FullURL: "static-url", BatchSize: 2, FullMinRows: 1}, nil)
	e.UseSettingsResolver(fakeResolver{err: errors.New("db down")})

	run, err := e.Run(context.Background(), domain.SyncFull, domain.TriggerManual)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if run.Status != domain.StatusSuccess {
		t.Fatalf("status=%s, want success (graceful fallback)", run.Status)
	}
	if rec.lastURL != "static-url" {
		t.Errorf("fetched URL=%q, want static fallback", rec.lastURL)
	}
}

// Given resolver 返回空 URL When 执行同步 Then 保留静态 URL（不被空值清掉）。
func TestEngine_ResolverEmptyURLKeepsStatic(t *testing.T) {
	st := openTestStore(t)
	rec := &recordingFetcher{csv: rowA + "\n"}
	e := NewEngine(st.Addresses(), st.SyncRuns(), st.Locker(), rec,
		Options{FullURL: "static-url", BatchSize: 2, FullMinRows: 1}, nil)
	e.UseSettingsResolver(fakeResolver{settings: domain.EffectiveSyncSettings{ScrapeFullURL: "", DownloadMaxRetry: 2}})

	if _, err := e.Run(context.Background(), domain.SyncFull, domain.TriggerManual); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rec.lastURL != "static-url" {
		t.Errorf("fetched URL=%q, want static-url when resolver URL empty", rec.lastURL)
	}
}
