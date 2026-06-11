-- 0002_runtime_settings (PostgreSQL) — 管理画面可配置、重启后保留的运行时覆盖值。
-- 与 internal/domain.RuntimeSetting / GORM AutoMigrate 一一对应。
-- 只存“覆盖”：未被设置过的键无行，有效值回退到 env / 代码默认值
-- （优先级 DB > env > 默认，见 docs/architecture.md §9.1）。“恢复默认”即删除对应行。
CREATE TABLE IF NOT EXISTS runtime_settings (
    key        VARCHAR(64)   PRIMARY KEY,
    value      VARCHAR(1024) NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ
);
