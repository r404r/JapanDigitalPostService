package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/r404r/JapanDigitalPostService/internal/domain"
)

// fakeRepo 是 domain.SettingsRepository 的内存替身，不触网/不触 DB。
type fakeRepo struct {
	data   map[domain.RuntimeSettingKey]string
	getErr error
	setErr error
	delErr error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{data: map[domain.RuntimeSettingKey]string{}} }

func (f *fakeRepo) GetAll(context.Context) (map[domain.RuntimeSettingKey]string, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	out := make(map[domain.RuntimeSettingKey]string, len(f.data))
	for k, v := range f.data {
		out[k] = v
	}
	return out, nil
}

func (f *fakeRepo) Set(_ context.Context, k domain.RuntimeSettingKey, v string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.data[k] = v
	return nil
}

func (f *fakeRepo) Delete(_ context.Context, k domain.RuntimeSettingKey) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.data, k)
	return nil
}

const defURL = "https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_ken_all.zip"

func newService(repo domain.SettingsRepository) *Service {
	return NewService(repo, Defaults{ScrapeFullURL: defURL, DownloadMaxRetry: 3}, nil)
}

// Given 无任何覆盖值 When 解析 Then 返回基线默认值且 Overridden=false。
func TestResolve_DefaultsWhenNoOverride(t *testing.T) {
	svc := newService(newFakeRepo())
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DownloadMaxRetry != 3 || got.DownloadMaxRetryOver {
		t.Errorf("retry=%d over=%v, want 3/false", got.DownloadMaxRetry, got.DownloadMaxRetryOver)
	}
	if got.ScrapeFullURL != defURL || got.ScrapeFullURLOver {
		t.Errorf("url=%q over=%v, want default/false", got.ScrapeFullURL, got.ScrapeFullURLOver)
	}
}

// Given DB 存在覆盖值 When 解析 Then DB 覆盖优先于默认（DB > env > 默认）。
func TestResolve_DBOverrideWins(t *testing.T) {
	repo := newFakeRepo()
	repo.data[domain.SettingDownloadMaxRetry] = "7"
	repo.data[domain.SettingScrapeFullURL] = "https://post.japanpost.jp/custom.zip"
	svc := newService(repo)

	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DownloadMaxRetry != 7 || !got.DownloadMaxRetryOver {
		t.Errorf("retry=%d over=%v, want 7/true", got.DownloadMaxRetry, got.DownloadMaxRetryOver)
	}
	if got.ScrapeFullURL != "https://post.japanpost.jp/custom.zip" || !got.ScrapeFullURLOver {
		t.Errorf("url=%q over=%v, want custom/true", got.ScrapeFullURL, got.ScrapeFullURLOver)
	}
}

// Given DB 存了一个脏（越界/非法）值 When 解析 Then 防御式回退到默认，不报错。
func TestResolve_DefensiveFallbackOnDirtyValue(t *testing.T) {
	repo := newFakeRepo()
	repo.data[domain.SettingDownloadMaxRetry] = "999"                        // 越界
	repo.data[domain.SettingScrapeFullURL] = "http://evil.example.com/x.zip" // 非白名单
	svc := newService(repo)

	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DownloadMaxRetry != 3 || got.DownloadMaxRetryOver {
		t.Errorf("retry=%d over=%v, want fallback 3/false", got.DownloadMaxRetry, got.DownloadMaxRetryOver)
	}
	if got.ScrapeFullURL != defURL || got.ScrapeFullURLOver {
		t.Errorf("url=%q over=%v, want fallback default/false", got.ScrapeFullURL, got.ScrapeFullURLOver)
	}
}

// Given 合法入参 When Update Then 写入覆盖并返回更新后视图。
func TestUpdate_SetsOverride(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)
	n := 5
	url := "https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_ken_all.zip"
	got, err := svc.Update(context.Background(), UpdateInput{DownloadMaxRetry: &n, ScrapeFullURL: &url})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.DownloadMaxRetry != 5 || !got.DownloadMaxRetryOver {
		t.Errorf("retry=%d over=%v, want 5/true", got.DownloadMaxRetry, got.DownloadMaxRetryOver)
	}
	if repo.data[domain.SettingDownloadMaxRetry] != "5" {
		t.Errorf("stored retry=%q, want 5", repo.data[domain.SettingDownloadMaxRetry])
	}
}

// Given 已有覆盖 When Update reset_to_default Then 删除覆盖、有效值回到默认。
func TestUpdate_ResetRestoresDefault(t *testing.T) {
	repo := newFakeRepo()
	repo.data[domain.SettingScrapeFullURL] = "https://post.japanpost.jp/custom.zip"
	svc := newService(repo)

	got, err := svc.Update(context.Background(), UpdateInput{
		ResetToDefault: []domain.RuntimeSettingKey{domain.SettingScrapeFullURL},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.ScrapeFullURL != defURL || got.ScrapeFullURLOver {
		t.Errorf("url=%q over=%v, want default/false after reset", got.ScrapeFullURL, got.ScrapeFullURLOver)
	}
	if _, ok := repo.data[domain.SettingScrapeFullURL]; ok {
		t.Error("override row should be deleted after reset")
	}
}

// Given 同键既设值又 reset When Update Then 返回校验错误，且不写入。
func TestUpdate_SetAndResetSameKeyConflicts(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)
	n := 4
	_, err := svc.Update(context.Background(), UpdateInput{
		DownloadMaxRetry: &n,
		ResetToDefault:   []domain.RuntimeSettingKey{domain.SettingDownloadMaxRetry},
	})
	if !IsValidation(err) {
		t.Fatalf("err=%v, want validation error", err)
	}
	if _, ok := repo.data[domain.SettingDownloadMaxRetry]; ok {
		t.Error("nothing should be written on conflict")
	}
}

// Given 各类非法 URL When Update Then 拒绝并给日语提示，且不写入（SSRF 防护边界）。
func TestUpdate_RejectsInvalidURLs(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"empty", "   "},
		{"http_not_https", "http://www.post.japanpost.jp/x.zip"},
		{"foreign_host", "https://evil.example.com/x.zip"},
		{"userinfo_smuggle", "https://www.post.japanpost.jp@evil.example.com/x.zip"},
		{"non_jp_subdomain", "https://post.japanpost.jp.evil.com/x.zip"},
		{"garbage", "://nonsense"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newService(repo)
			u := c.url
			_, err := svc.Update(context.Background(), UpdateInput{ScrapeFullURL: &u})
			if !IsValidation(err) {
				t.Fatalf("url=%q err=%v, want validation error", c.url, err)
			}
			if len(repo.data) != 0 {
				t.Errorf("url=%q should not be stored, repo=%v", c.url, repo.data)
			}
		})
	}
}

// Given 越界重试次数 When Update Then 拒绝（0..10 边界）。
func TestUpdate_RejectsRetryOutOfRange(t *testing.T) {
	for _, n := range []int{-1, 11, 100} {
		repo := newFakeRepo()
		svc := newService(repo)
		v := n
		_, err := svc.Update(context.Background(), UpdateInput{DownloadMaxRetry: &v})
		if !IsValidation(err) {
			t.Fatalf("n=%d err=%v, want validation error", n, err)
		}
	}
	// 边界内合法：0 与 10。
	for _, n := range []int{0, 10} {
		repo := newFakeRepo()
		svc := newService(repo)
		v := n
		if _, err := svc.Update(context.Background(), UpdateInput{DownloadMaxRetry: &v}); err != nil {
			t.Fatalf("n=%d should be valid, got %v", n, err)
		}
	}
}

// Given repo 读失败 When 解析 Then 透传错误（非校验错误）。
func TestResolve_PropagatesRepoError(t *testing.T) {
	repo := newFakeRepo()
	repo.getErr = errors.New("db down")
	svc := newService(repo)
	if _, err := svc.Get(context.Background()); err == nil {
		t.Fatal("want error when repo fails")
	}
	if _, err := svc.ResolveSyncSettings(context.Background()); err == nil {
		t.Fatal("ResolveSyncSettings should propagate repo error")
	}
}

// Given DB 覆盖存在 When ResolveSyncSettings Then 返回引擎所需的有效配置（DB 优先）。
func TestResolveSyncSettings_ReturnsEffective(t *testing.T) {
	repo := newFakeRepo()
	repo.data[domain.SettingDownloadMaxRetry] = "6"
	repo.data[domain.SettingScrapeFullURL] = "https://www.post.japanpost.jp/custom.zip"
	svc := newService(repo)

	eff, err := svc.ResolveSyncSettings(context.Background())
	if err != nil {
		t.Fatalf("ResolveSyncSettings: %v", err)
	}
	if eff.DownloadMaxRetry != 6 {
		t.Errorf("retry=%d, want 6", eff.DownloadMaxRetry)
	}
	if eff.ScrapeFullURL != "https://www.post.japanpost.jp/custom.zip" {
		t.Errorf("url=%q, want override", eff.ScrapeFullURL)
	}

	// 无覆盖时回退默认。
	eff2, err := newService(newFakeRepo()).ResolveSyncSettings(context.Background())
	if err != nil {
		t.Fatalf("ResolveSyncSettings default: %v", err)
	}
	if eff2.DownloadMaxRetry != 3 || eff2.ScrapeFullURL != defURL {
		t.Errorf("default effective = %+v, want 3/%s", eff2, defURL)
	}
}

// Given ValidationError When .Error() Then 返回 Message（用于日志/回显）。
func TestValidationError_Message(t *testing.T) {
	e := &ValidationError{Field: "scrape_full_url", Message: "URL は https で指定してください。"}
	if e.Error() != "URL は https で指定してください。" {
		t.Errorf("Error()=%q", e.Error())
	}
}

// Given 未知 reset 键 When Update Then 校验错误。
func TestUpdate_RejectsUnknownResetKey(t *testing.T) {
	svc := newService(newFakeRepo())
	_, err := svc.Update(context.Background(), UpdateInput{
		ResetToDefault: []domain.RuntimeSettingKey{"bogus_key"},
	})
	if !IsValidation(err) {
		t.Fatalf("err=%v, want validation error for unknown key", err)
	}
}
