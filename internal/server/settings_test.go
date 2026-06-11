package server

import (
	"context"
	"encoding/json"
	"testing"
)

// fakeSettings 实现 SettingsService，记录入参并返回预置视图/错误。
type fakeSettings struct {
	view      SettingsView
	updateErr error
	gotUpdate SettingsUpdate
	updated   bool
}

func (f *fakeSettings) Get(context.Context) (SettingsView, error) { return f.view, nil }
func (f *fakeSettings) Update(_ context.Context, in SettingsUpdate) (SettingsView, error) {
	f.updated = true
	f.gotUpdate = in
	if f.updateErr != nil {
		return SettingsView{}, f.updateErr
	}
	return f.view, nil
}

func newSettingsView() SettingsView {
	return SettingsView{
		DownloadMaxRetry: 3, DownloadMaxRetryDefault: 3, DownloadMaxRetryOver: false,
		ScrapeFullURL: "https://www.post.japanpost.jp/x.zip", ScrapeFullURLDefault: "https://www.post.japanpost.jp/x.zip",
	}
}

// Given admin token When GET /v1/admin/settings Then 200 且返回 value/default/overridden。
func TestGetSettings_AdminOK(t *testing.T) {
	fs := &fakeSettings{view: newSettingsView()}
	h, admin, _ := newSyncRouter(t, Options{Settings: fs})

	rec := doAuth(t, h, "GET", "/v1/admin/settings", admin, "")
	if rec.Code != 200 {
		t.Fatalf("code=%d, want 200", rec.Code)
	}
	var body settingsDTO
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.DownloadMaxRetry.Value != 3 || body.DownloadMaxRetry.Default != 3 || body.DownloadMaxRetry.Overridden {
		t.Errorf("download_max_retry dto = %+v", body.DownloadMaxRetry)
	}
}

// Given read token（非 admin）When GET /v1/admin/settings Then 403。
func TestGetSettings_RequiresAdmin(t *testing.T) {
	fs := &fakeSettings{view: newSettingsView()}
	h, _, read := newSyncRouter(t, Options{Settings: fs})

	if rec := doAuth(t, h, "GET", "/v1/admin/settings", read, ""); rec.Code != 403 {
		t.Errorf("read scope code=%d, want 403", rec.Code)
	}
	if rec := doAuth(t, h, "GET", "/v1/admin/settings", "", ""); rec.Code != 401 {
		t.Errorf("no-auth code=%d, want 401", rec.Code)
	}
}

// Given 合法 body When PUT Then 200 且把入参透传给 service。
func TestPutSettings_OK(t *testing.T) {
	fs := &fakeSettings{view: newSettingsView()}
	h, admin, _ := newSyncRouter(t, Options{Settings: fs})

	rec := doAuth(t, h, "PUT", "/v1/admin/settings", admin, `{"download_max_retry":5}`)
	if rec.Code != 200 {
		t.Fatalf("code=%d, want 200", rec.Code)
	}
	if !fs.updated || fs.gotUpdate.DownloadMaxRetry == nil || *fs.gotUpdate.DownloadMaxRetry != 5 {
		t.Errorf("update not forwarded: %+v", fs.gotUpdate)
	}
}

// Given reset_to_default body When PUT Then 透传 ResetToDefault。
func TestPutSettings_ResetForwarded(t *testing.T) {
	fs := &fakeSettings{view: newSettingsView()}
	h, admin, _ := newSyncRouter(t, Options{Settings: fs})

	rec := doAuth(t, h, "PUT", "/v1/admin/settings", admin, `{"reset_to_default":["scrape_full_url"]}`)
	if rec.Code != 200 {
		t.Fatalf("code=%d, want 200", rec.Code)
	}
	if len(fs.gotUpdate.ResetToDefault) != 1 || fs.gotUpdate.ResetToDefault[0] != "scrape_full_url" {
		t.Errorf("reset not forwarded: %+v", fs.gotUpdate.ResetToDefault)
	}
}

// Given service 返回校验错误 When PUT Then 400 invalid_request 且回显日语 message。
func TestPutSettings_ValidationError(t *testing.T) {
	fs := &fakeSettings{view: newSettingsView(), updateErr: &SettingsError{Validation: true, Message: "URL は https で指定してください。"}}
	h, admin, _ := newSyncRouter(t, Options{Settings: fs})

	rec := doAuth(t, h, "PUT", "/v1/admin/settings", admin, `{"scrape_full_url":"http://x"}`)
	if rec.Code != 400 {
		t.Fatalf("code=%d, want 400", rec.Code)
	}
	var body errorDTO
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "invalid_request" || body.Message != "URL は https で指定してください。" {
		t.Errorf("error dto = %+v", body)
	}
}

// Given 非 admin When PUT Then 403（写操作需 admin）。
func TestPutSettings_RequiresAdmin(t *testing.T) {
	fs := &fakeSettings{view: newSettingsView()}
	h, _, read := newSyncRouter(t, Options{Settings: fs})
	if rec := doAuth(t, h, "PUT", "/v1/admin/settings", read, `{"download_max_retry":1}`); rec.Code != 403 {
		t.Errorf("read scope code=%d, want 403", rec.Code)
	}
	if fs.updated {
		t.Error("service should not be called when authz fails")
	}
}

// Given 非法 JSON When PUT Then 400。
func TestPutSettings_BadJSON(t *testing.T) {
	fs := &fakeSettings{view: newSettingsView()}
	h, admin, _ := newSyncRouter(t, Options{Settings: fs})
	if rec := doAuth(t, h, "PUT", "/v1/admin/settings", admin, `{not json}`); rec.Code != 400 {
		t.Errorf("bad json code=%d, want 400", rec.Code)
	}
}
