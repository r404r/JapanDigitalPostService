// Package config 从环境变量加载服务配置。
//
// 覆盖启动、数据库连接健壮性、同步引擎（task-0004）与认证 / 可选载荷加密
// （task-0006）所需字段。所有键与默认值见 docs/architecture.md §9。
package config

import (
	"os"
	"strconv"
	"time"
)

// 同步数据源默认地址（已对 Japan Post 真实端点核验，见 docs/spec.md §2）。
// 注意：全量与差分共用 service/search 路径；utf-zip.html 上的相对链接 base 会 404。
const (
	defaultFullURL = "https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_ken_all.zip"
	defaultAddTmpl = "https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_add_%s.zip"
	defaultDelTmpl = "https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_del_%s.zip"
)

// Config 是服务运行配置。
type Config struct {
	HTTPAddr              string        // HTTP_ADDR
	StaticDir             string        // STATIC_DIR: optional React production build directory
	HTTPReadHeaderTimeout time.Duration // HTTP_READ_HEADER_TIMEOUT
	HTTPReadTimeout       time.Duration // HTTP_READ_TIMEOUT
	HTTPWriteTimeout      time.Duration // HTTP_WRITE_TIMEOUT
	HTTPIdleTimeout       time.Duration // HTTP_IDLE_TIMEOUT
	QueryTimeout          time.Duration // QUERY_TIMEOUT
	FuzzyLimit            int           // FUZZY_LIMIT
	MaxTotal              int           // MAX_TOTAL

	// 数据库连接与健壮性。
	DBDriver          string        // DB_DRIVER: postgres|mysql|sqlite
	DBDSN             string        // DB_DSN
	DBConnectTimeout  time.Duration // DB_CONNECT_TIMEOUT
	DBMaxRetry        int           // DB_MAX_RETRY
	DBRetryBackoff    time.Duration // DB_RETRY_BACKOFF
	DBMaxOpenConns    int           // DB_MAX_OPEN_CONNS
	DBMaxIdleConns    int           // DB_MAX_IDLE_CONNS
	DBConnMaxLifetime time.Duration // DB_CONN_MAX_LIFETIME

	// 同步调度与引擎。
	SyncCron               string        // SYNC_CRON
	SyncSchedulerOn        bool          // SYNC_SCHEDULER_ENABLED（server 进程内调度开关）
	SyncFullURL            string        // SYNC_FULL_URL
	SyncAddURLTemplate     string        // SYNC_ADD_URL_TEMPLATE（含 %s = YYMM）
	SyncDelURLTemplate     string        // SYNC_DEL_URL_TEMPLATE（含 %s = YYMM）
	SyncBatchSize          int           // SYNC_BATCH_SIZE
	SyncFullPrune          bool          // SYNC_FULL_PRUNE（全量后剪除消失地址）
	SyncFullMinRows        int           // SYNC_FULL_MIN_ROWS（剪枝安全下限）
	SyncDiffFallback       bool          // SYNC_DIFF_FALLBACK_FULL
	SyncDiffLookback       int           // SYNC_DIFF_LOOKBACK_MONTHS
	SyncLockReleaseTimeout time.Duration // SYNC_LOCK_RELEASE_TIMEOUT
	DownloadTimeout        time.Duration // DOWNLOAD_TIMEOUT（单次尝试）
	DownloadMaxRetry       int           // DOWNLOAD_MAX_RETRY
	DownloadBackoff        time.Duration // DOWNLOAD_RETRY_BACKOFF
	TownSkipRegex          string        // SYNC_TOWN_SKIP_REGEX（按町域名跳过导入；空=关闭）

	// 认证（task-0006）。
	AdminBootstrapToken string // ADMIN_BOOTSTRAP_TOKEN: 首个 admin token，引导用

	// 传输安全 / 可选载荷加密（task-0006 设计，task-0007 深化）。
	PayloadEncryption string // PAYLOAD_ENCRYPTION: none|aes-gcm
	PayloadEncKey     string // PAYLOAD_ENC_KEY: base64(32B) 密钥，仅 aes-gcm 模式需要
	PayloadEncKeyID   string // PAYLOAD_ENC_KEY_ID: 可选密钥标识，便于轮换

	// SeedSample：addresses 表为空时是否写入内置示例数据，便于本地启动即可查询
	// （task-0005）。同步引擎（task-0004）已落地，故默认关闭；本地开发可显式开启。
	SeedSample bool // SEED_SAMPLE_DATA
}

// Load 从环境变量读取配置，缺省时使用 architecture §9 的默认值。
func Load() Config {
	dbDriver := getEnv("DB_DRIVER", "sqlite")
	maxOpenDefault, maxIdleDefault, lifetimeDefault := dbPoolDefaults(dbDriver)
	return Config{
		HTTPAddr:              getEnv("HTTP_ADDR", ":8080"),
		StaticDir:             getEnv("STATIC_DIR", ""),
		HTTPReadHeaderTimeout: getDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		HTTPReadTimeout:       getDuration("HTTP_READ_TIMEOUT", 15*time.Second),
		HTTPWriteTimeout:      getDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
		HTTPIdleTimeout:       getDuration("HTTP_IDLE_TIMEOUT", 120*time.Second),
		QueryTimeout:          getDuration("QUERY_TIMEOUT", 2*time.Second),
		FuzzyLimit:            getInt("FUZZY_LIMIT", 20),
		MaxTotal:              getInt("MAX_TOTAL", 1000),

		DBDriver:          dbDriver,
		DBDSN:             getEnv("DB_DSN", "file:dev.db?cache=shared&_fk=1"),
		DBConnectTimeout:  getDuration("DB_CONNECT_TIMEOUT", 5*time.Second),
		DBMaxRetry:        getInt("DB_MAX_RETRY", 5),
		DBRetryBackoff:    getDuration("DB_RETRY_BACKOFF", 500*time.Millisecond),
		DBMaxOpenConns:    getInt("DB_MAX_OPEN_CONNS", maxOpenDefault),
		DBMaxIdleConns:    getInt("DB_MAX_IDLE_CONNS", maxIdleDefault),
		DBConnMaxLifetime: getDuration("DB_CONN_MAX_LIFETIME", lifetimeDefault),

		SyncCron:               getEnv("SYNC_CRON", "0 3 * * *"),
		SyncSchedulerOn:        getBool("SYNC_SCHEDULER_ENABLED", true),
		SyncFullURL:            getEnv("SYNC_FULL_URL", defaultFullURL),
		SyncAddURLTemplate:     getEnv("SYNC_ADD_URL_TEMPLATE", defaultAddTmpl),
		SyncDelURLTemplate:     getEnv("SYNC_DEL_URL_TEMPLATE", defaultDelTmpl),
		SyncBatchSize:          getInt("SYNC_BATCH_SIZE", 1000),
		SyncFullPrune:          getBool("SYNC_FULL_PRUNE", true),
		SyncFullMinRows:        getInt("SYNC_FULL_MIN_ROWS", 1000),
		SyncDiffFallback:       getBool("SYNC_DIFF_FALLBACK_FULL", true),
		SyncDiffLookback:       getInt("SYNC_DIFF_LOOKBACK_MONTHS", 3),
		SyncLockReleaseTimeout: getDuration("SYNC_LOCK_RELEASE_TIMEOUT", 5*time.Second),
		DownloadTimeout:        getDuration("DOWNLOAD_TIMEOUT", 60*time.Second),
		DownloadMaxRetry:       getInt("DOWNLOAD_MAX_RETRY", 3),
		DownloadBackoff:        getDuration("DOWNLOAD_RETRY_BACKOFF", time.Second),
		TownSkipRegex:          getEnv("SYNC_TOWN_SKIP_REGEX", ""),

		AdminBootstrapToken: getEnv("ADMIN_BOOTSTRAP_TOKEN", ""),

		PayloadEncryption: getEnv("PAYLOAD_ENCRYPTION", "none"),
		PayloadEncKey:     getEnv("PAYLOAD_ENC_KEY", ""),
		PayloadEncKeyID:   getEnv("PAYLOAD_ENC_KEY_ID", ""),

		SeedSample: getBool("SEED_SAMPLE_DATA", false),
	}
}

func dbPoolDefaults(driver string) (maxOpen int, maxIdle int, maxLifetime time.Duration) {
	if driver == "" || driver == "sqlite" {
		return 1, 1, 0
	}
	return 25, 10, time.Hour
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

func getBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
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
