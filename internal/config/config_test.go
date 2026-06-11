package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// 清掉可能影响默认值的环境变量。
	for _, k := range []string{"HTTP_ADDR", "STATIC_DIR", "QUERY_TIMEOUT", "FUZZY_LIMIT", "MAX_TOTAL", "DB_DRIVER", "DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "DB_CONN_MAX_LIFETIME"} {
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
	if cfg.DBMaxOpenConns != 1 || cfg.DBMaxIdleConns != 1 || cfg.DBConnMaxLifetime != 0 {
		t.Errorf("sqlite pool defaults = %d/%d/%v, want 1/1/0", cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("STATIC_DIR", "/app/web")
	t.Setenv("QUERY_TIMEOUT", "500ms")
	t.Setenv("FUZZY_LIMIT", "10")
	t.Setenv("DB_MAX_OPEN_CONNS", "12")
	t.Setenv("DB_MAX_IDLE_CONNS", "4")
	t.Setenv("DB_CONN_MAX_LIFETIME", "30m")
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
	if cfg.DBMaxOpenConns != 12 || cfg.DBMaxIdleConns != 4 || cfg.DBConnMaxLifetime != 30*time.Minute {
		t.Errorf("db pool = %d/%d/%v, want 12/4/30m", cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
	}
}

func TestLoadPostgresPoolDefaults(t *testing.T) {
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_MAX_OPEN_CONNS", "")
	t.Setenv("DB_MAX_IDLE_CONNS", "")
	t.Setenv("DB_CONN_MAX_LIFETIME", "")

	cfg := Load()
	if cfg.DBMaxOpenConns != 25 || cfg.DBMaxIdleConns != 10 || cfg.DBConnMaxLifetime != time.Hour {
		t.Errorf("postgres pool defaults = %d/%d/%v, want 25/10/1h", cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
	}
}
