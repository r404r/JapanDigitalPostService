package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// 清掉可能影响默认值的环境变量。
	for _, k := range []string{
		"HTTP_ADDR", "STATIC_DIR", "HTTP_READ_HEADER_TIMEOUT", "HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT", "HTTP_IDLE_TIMEOUT",
		"QUERY_TIMEOUT", "FUZZY_LIMIT", "MAX_TOTAL", "DB_DRIVER", "DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "DB_CONN_MAX_LIFETIME",
		"SYNC_LOCK_RELEASE_TIMEOUT",
	} {
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
	if cfg.HTTPReadHeaderTimeout != 5*time.Second || cfg.HTTPReadTimeout != 15*time.Second ||
		cfg.HTTPWriteTimeout != 30*time.Second || cfg.HTTPIdleTimeout != 120*time.Second {
		t.Errorf("http timeouts = %v/%v/%v/%v, want 5s/15s/30s/120s",
			cfg.HTTPReadHeaderTimeout, cfg.HTTPReadTimeout, cfg.HTTPWriteTimeout, cfg.HTTPIdleTimeout)
	}
	if cfg.FuzzyLimit != 20 {
		t.Errorf("FuzzyLimit default = %d, want 20", cfg.FuzzyLimit)
	}
	if cfg.DBMaxOpenConns != 1 || cfg.DBMaxIdleConns != 1 || cfg.DBConnMaxLifetime != 0 {
		t.Errorf("sqlite pool defaults = %d/%d/%v, want 1/1/0", cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
	}
	if cfg.SyncLockReleaseTimeout != 5*time.Second {
		t.Errorf("SyncLockReleaseTimeout default = %v, want 5s", cfg.SyncLockReleaseTimeout)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("STATIC_DIR", "/app/web")
	t.Setenv("HTTP_READ_HEADER_TIMEOUT", "2s")
	t.Setenv("HTTP_READ_TIMEOUT", "3s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "4s")
	t.Setenv("HTTP_IDLE_TIMEOUT", "5s")
	t.Setenv("QUERY_TIMEOUT", "500ms")
	t.Setenv("FUZZY_LIMIT", "10")
	t.Setenv("DB_MAX_OPEN_CONNS", "12")
	t.Setenv("DB_MAX_IDLE_CONNS", "4")
	t.Setenv("DB_CONN_MAX_LIFETIME", "30m")
	t.Setenv("SYNC_LOCK_RELEASE_TIMEOUT", "750ms")
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
	if cfg.HTTPReadHeaderTimeout != 2*time.Second || cfg.HTTPReadTimeout != 3*time.Second ||
		cfg.HTTPWriteTimeout != 4*time.Second || cfg.HTTPIdleTimeout != 5*time.Second {
		t.Errorf("http timeouts = %v/%v/%v/%v, want 2s/3s/4s/5s",
			cfg.HTTPReadHeaderTimeout, cfg.HTTPReadTimeout, cfg.HTTPWriteTimeout, cfg.HTTPIdleTimeout)
	}
	if cfg.FuzzyLimit != 10 {
		t.Errorf("FuzzyLimit = %d, want 10", cfg.FuzzyLimit)
	}
	if cfg.DBMaxOpenConns != 12 || cfg.DBMaxIdleConns != 4 || cfg.DBConnMaxLifetime != 30*time.Minute {
		t.Errorf("db pool = %d/%d/%v, want 12/4/30m", cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
	}
	if cfg.SyncLockReleaseTimeout != 750*time.Millisecond {
		t.Errorf("SyncLockReleaseTimeout = %v, want 750ms", cfg.SyncLockReleaseTimeout)
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
