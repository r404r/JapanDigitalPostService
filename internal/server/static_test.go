package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWithStaticFilesServesIndexAndAssets(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "index.html"), "<!doctype html><title>app</title>")
	writeTestFile(t, filepath.Join(dir, "assets", "app.js"), "console.log('ok')")

	handler := WithStaticFiles(http.NotFoundHandler(), dir)

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/", want: "<!doctype html><title>app</title>"},
		{path: "/admin/sync", want: "<!doctype html><title>app</title>"},
		{path: "/assets/app.js", want: "console.log('ok')"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", tc.path, rr.Code)
		}
		if got := rr.Body.String(); got != tc.want {
			t.Fatalf("%s body = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestWithStaticFilesPassesAPIToHandler(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "index.html"), "index")
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("api"))
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	rr := httptest.NewRecorder()
	WithStaticFiles(api, dir).ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rr.Code)
	}
	if got := rr.Body.String(); got != "api" {
		t.Fatalf("body = %q, want api", got)
	}
}

func TestWithStaticFilesMissingAssetReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "index.html"), "index")

	req := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	rr := httptest.NewRecorder()
	WithStaticFiles(http.NotFoundHandler(), dir).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func writeTestFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
