-- 0001_init (SQLite) — addresses / tokens / sync_runs / sync_locks 初始 schema。
-- 与 internal/store 的 GORM 模型一一对应（开发用 AutoMigrate，生产可执行本文件）。
-- 三方言分文件维护（自增/时间/布尔类型差异），列名与索引名保持一致。
-- 逻辑唯一键为 4 列 (zipcode, jis_code, town, town_kana)：同一町域两种合法读音需各自独立
-- （GHO-33 / spec §2 §5.3）。

CREATE TABLE IF NOT EXISTS addresses (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    zipcode         VARCHAR(7)   NOT NULL,
    jis_code        VARCHAR(5)   NOT NULL,
    prefecture      VARCHAR(64)  NOT NULL,
    prefecture_kana VARCHAR(128) NOT NULL,
    city            VARCHAR(128) NOT NULL,
    city_kana       VARCHAR(256) NOT NULL,
    town            VARCHAR(256) NOT NULL,
    town_kana       VARCHAR(256) NOT NULL,
    flag_multi_zip  INTEGER      NOT NULL DEFAULT 0,
    flag_koaza      INTEGER      NOT NULL DEFAULT 0,
    flag_chome      INTEGER      NOT NULL DEFAULT 0,
    flag_multi_town INTEGER      NOT NULL DEFAULT 0,
    source_hash     VARCHAR(64)  NOT NULL,
    updated_at      DATETIME
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_addr ON addresses (zipcode, jis_code, town, town_kana);
CREATE INDEX IF NOT EXISTS idx_addresses_zipcode    ON addresses (zipcode);
CREATE INDEX IF NOT EXISTS idx_addresses_prefecture ON addresses (prefecture);
CREATE INDEX IF NOT EXISTS idx_addresses_city       ON addresses (city);
CREATE INDEX IF NOT EXISTS idx_addresses_town       ON addresses (town);

CREATE TABLE IF NOT EXISTS tokens (
    id           VARCHAR(36)  PRIMARY KEY,
    name         VARCHAR(128) NOT NULL,
    prefix       VARCHAR(16)  NOT NULL,
    token_hash   VARCHAR(64)  NOT NULL,
    scope        VARCHAR(16)  NOT NULL,
    created_at   DATETIME,
    expires_at   DATETIME,
    last_used_at DATETIME,
    revoked_at   DATETIME
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_tokens_token_hash ON tokens (token_hash);

CREATE TABLE IF NOT EXISTS sync_runs (
    id            VARCHAR(36)  PRIMARY KEY,
    type          VARCHAR(8)   NOT NULL,
    status        VARCHAR(8)   NOT NULL,
    trigger       VARCHAR(8)   NOT NULL,
    source_url    VARCHAR(512) NOT NULL,
    file_checksum VARCHAR(128) NOT NULL,
    file_size     BIGINT       NOT NULL DEFAULT 0,
    diff_period   VARCHAR(8)   NOT NULL DEFAULT '',
    rows_added    BIGINT       NOT NULL DEFAULT 0,
    rows_updated  BIGINT       NOT NULL DEFAULT 0,
    rows_deleted  BIGINT       NOT NULL DEFAULT 0,
    rows_skipped  BIGINT       NOT NULL DEFAULT 0,
    rows_total    BIGINT       NOT NULL DEFAULT 0,
    started_at    DATETIME     NOT NULL,
    finished_at   DATETIME,
    duration_ms   BIGINT       NOT NULL DEFAULT 0,
    error_message TEXT         NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sync_runs_type   ON sync_runs (type);
CREATE INDEX IF NOT EXISTS idx_sync_runs_status ON sync_runs (status);

CREATE TABLE IF NOT EXISTS sync_skipped_rows (
    id              INTEGER      PRIMARY KEY AUTOINCREMENT,
    run_id          VARCHAR(36)  NOT NULL,
    source_type     VARCHAR(16)  NOT NULL,
    line_number     INTEGER      NOT NULL DEFAULT 0,
    zipcode         VARCHAR(7)   NOT NULL DEFAULT '',
    jis_code        VARCHAR(5)   NOT NULL DEFAULT '',
    prefecture      VARCHAR(64)  NOT NULL DEFAULT '',
    city            VARCHAR(128) NOT NULL DEFAULT '',
    town            VARCHAR(256) NOT NULL DEFAULT '',
    town_kana       VARCHAR(256) NOT NULL DEFAULT '',
    reason          VARCHAR(64)  NOT NULL DEFAULT '',
    pattern         VARCHAR(1024) NOT NULL DEFAULT '',
    raw_record_json TEXT         NOT NULL DEFAULT '',
    created_at      DATETIME
);
CREATE INDEX IF NOT EXISTS idx_sync_skipped_rows_run_id ON sync_skipped_rows (run_id);

-- 单行同步互斥锁（id 恒为 1）。acquired_at 不用零值，避免严格模式方言拒绝。
CREATE TABLE IF NOT EXISTS sync_locks (
    id          INTEGER      PRIMARY KEY,
    locked      BOOLEAN      NOT NULL DEFAULT 0,
    holder      VARCHAR(128) NOT NULL DEFAULT '',
    acquired_at DATETIME
);
