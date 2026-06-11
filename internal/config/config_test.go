package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// 清掉可能影响默认值的环境变量。
	for _, k := range []string{"HTTP_ADDR", "STATIC_DIR", "QUERY_TIMEOUT", "FUZZY_LIMIT", "MAX_TOTAL"} {
		t.Setenv(k, "")
	}
	cfg := Load()
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr default = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.StaticDir != "" {
		t.Errorf("StaticDir default = %q, want empty", cfg.StaticDir)
	}
	if cfg.QueryTimeout != 2*time.Second {
		t.Errorf("QueryTimeout default = %v, want 2s", cfg.QueryTimeout)
	}
	if cfg.FuzzyLimit != 20 {
		t.Errorf("FuzzyLimit default = %d, want 20", cfg.FuzzyLimit)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("STATIC_DIR", "/app/web")
	t.Setenv("QUERY_TIMEOUT", "500ms")
	t.Setenv("FUZZY_LIMIT", "10")
	cfg := Load()
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
	}
	if cfg.StaticDir != "/app/web" {
		t.Errorf("StaticDir = %q, want /app/web", cfg.StaticDir)
	}
	if cfg.QueryTimeout != 500*time.Millisecond {
		t.Errorf("QueryTimeout = %v, want 500ms", cfg.QueryTimeout)
	}
	if cfg.FuzzyLimit != 10 {
		t.Errorf("FuzzyLimit = %d, want 10", cfg.FuzzyLimit)
	}
}
