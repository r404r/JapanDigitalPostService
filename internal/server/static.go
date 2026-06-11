package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// WithStaticFiles serves a production frontend build for non-API GET/HEAD
// requests. Unknown frontend routes fall back to index.html for SPA routing.
func WithStaticFiles(api http.Handler, dir string) http.Handler {
	static := http.FileServer(http.Dir(dir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1") || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			api.ServeHTTP(w, r)
			return
		}

		clean := strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), "/")
		if clean == "." {
			clean = "index.html"
		}
		fullPath := filepath.Join(dir, clean)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			static.ServeHTTP(w, r)
			return
		}
		if filepath.Ext(clean) != "" {
			http.NotFound(w, r)
			return
		}

		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}
