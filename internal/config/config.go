// Package config 从环境变量加载服务配置。
//
// 当前为骨架版本，仅覆盖启动所需字段；DB / 同步 / 加密等字段
// 在对应 task（0002/0004/0007）扩展。所有键与默认值见 docs/architecture.md §9。
package config

import (
	"os"
	"strconv"
	"time"
)

// Config 是服务运行配置。
type Config struct {
	HTTPAddr     string        // HTTP_ADDR
	QueryTimeout time.Duration // QUERY_TIMEOUT
	FuzzyLimit   int           // FUZZY_LIMIT
	MaxTotal     int           // MAX_TOTAL

	// 以下字段在后续 task 启用（先占位，便于统一加载）：
	DBDriver string // DB_DRIVER: postgres|mysql|sqlite
	DBDSN    string // DB_DSN
	SyncCron string // SYNC_CRON

	// 认证（task-0006）。
	AdminBootstrapToken string // ADMIN_BOOTSTRAP_TOKEN: 首个 admin token，引导用

	// 传输安全 / 可选载荷加密（task-0006 设计，task-0007 深化）。
	PayloadEncryption string // PAYLOAD_ENCRYPTION: none|aes-gcm
	PayloadEncKey     string // PAYLOAD_ENC_KEY: base64(32B) 密钥，仅 aes-gcm 模式需要
	PayloadEncKeyID   string // PAYLOAD_ENC_KEY_ID: 可选密钥标识，便于轮换
}

// Load 从环境变量读取配置，缺省时使用 architecture §9 的默认值。
func Load() Config {
	return Config{
		HTTPAddr:     getEnv("HTTP_ADDR", ":8080"),
		QueryTimeout: getDuration("QUERY_TIMEOUT", 2*time.Second),
		FuzzyLimit:   getInt("FUZZY_LIMIT", 20),
		MaxTotal:     getInt("MAX_TOTAL", 1000),
		DBDriver:     getEnv("DB_DRIVER", "sqlite"),
		DBDSN:        getEnv("DB_DSN", "file:dev.db?cache=shared&_fk=1"),
		SyncCron:     getEnv("SYNC_CRON", "0 3 * * *"),

		AdminBootstrapToken: getEnv("ADMIN_BOOTSTRAP_TOKEN", ""),

		PayloadEncryption: getEnv("PAYLOAD_ENCRYPTION", "none"),
		PayloadEncKey:     getEnv("PAYLOAD_ENC_KEY", ""),
		PayloadEncKeyID:   getEnv("PAYLOAD_ENC_KEY_ID", ""),
	}
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
