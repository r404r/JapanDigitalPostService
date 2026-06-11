package domain

import (
	"context"
	"time"
)

// RuntimeSettingKey 标识一个可在管理画面配置、持久化到 DB 的运行时覆盖项。
// 只有被显式设置过的键才会作为一行存在；不存在该行即“无覆盖”，有效值回退到
// env / 代码默认值（优先级见 docs/architecture.md §9.1）。
type RuntimeSettingKey string

const (
	// SettingDownloadMaxRetry 抓取下载的额外重试次数（管理画面可配，默认 3）。
	SettingDownloadMaxRetry RuntimeSettingKey = "download_max_retry"
	// SettingScrapeFullURL 全量抓取数据源 URL（管理画面可配，默认 = 当前配置的全量 URL）。
	SettingScrapeFullURL RuntimeSettingKey = "scrape_full_url"
)

// RuntimeSetting 是一条持久化的覆盖值。Value 以字符串存储（数值/URL 均如此），
// 由上层 settings 服务按键解析与校验。“恢复默认”语义即删除对应行。
type RuntimeSetting struct {
	Key       string    `gorm:"column:key;primaryKey;type:varchar(64)"`
	Value     string    `gorm:"column:value;type:varchar(1024)"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName 固定表名。
func (RuntimeSetting) TableName() string { return "runtime_settings" }

// SettingsRepository 持久化管理画面可配置的运行时覆盖值。它只承载“覆盖”——
// 未被设置的键不存在行，有效值由上层服务结合 env/默认值解析。
type SettingsRepository interface {
	// GetAll 返回当前所有覆盖值（key→raw value）。空映射是正常情况。
	GetAll(ctx context.Context) (map[RuntimeSettingKey]string, error)
	// Set 写入或覆盖一个键的值（upsert）。
	Set(ctx context.Context, key RuntimeSettingKey, value string) error
	// Delete 删除一个键的覆盖值（恢复默认）。键不存在时不报错。
	Delete(ctx context.Context, key RuntimeSettingKey) error
}

// EffectiveSyncSettings 是同步引擎在每次运行前解析得到的有效配置快照
// （DB 覆盖 > env > 代码默认值）。引擎据此决定全量数据源与下载重试次数，
// 从而让管理画面的改动无需重启即可生效。
type EffectiveSyncSettings struct {
	ScrapeFullURL    string
	DownloadMaxRetry int
}
