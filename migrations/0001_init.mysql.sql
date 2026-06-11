-- 0001_init (MySQL 8, InnoDB / utf8mb4) — addresses / tokens / sync_runs / sync_locks 初始 schema。
-- 与 internal/store 的 GORM 模型一一对应；生产环境显式执行本文件。
-- 注意：
--   - `trigger` 为 MySQL 保留字，需反引号。
--   - 逻辑唯一键为 4 列 (zipcode, jis_code, town, town_kana)。InnoDB 单索引前缀上限
--     3072 字节，utf8mb4 下需控制列长：town/town_kana 各 256 字符（×4=1024B），
--     7+5+256+256 列 ×4 = 2096B < 3072B（见 internal/domain/address.go 注释）。
--   - acquired_at 由应用写入有效纪元值，不依赖零值（严格模式拒绝 '0000-00-00'）。

CREATE TABLE IF NOT EXISTS addresses (
    id              BIGINT       NOT NULL AUTO_INCREMENT,
    zipcode         VARCHAR(7)   NOT NULL,
    jis_code        VARCHAR(5)   NOT NULL,
    prefecture      VARCHAR(64)  NOT NULL,
    prefecture_kana VARCHAR(128) NOT NULL,
    city            VARCHAR(128) NOT NULL,
    city_kana       VARCHAR(256) NOT NULL,
    town            VARCHAR(256) NOT NULL,
    town_kana       VARCHAR(256) NOT NULL,
    flag_multi_zip  INT          NOT NULL DEFAULT 0,
    flag_koaza      INT          NOT NULL DEFAULT 0,
    flag_chome      INT          NOT NULL DEFAULT 0,
    flag_multi_town INT          NOT NULL DEFAULT 0,
    source_hash     VARCHAR(64)  NOT NULL,
    updated_at      DATETIME(3)  NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_addr (zipcode, jis_code, town, town_kana),
    KEY idx_addresses_zipcode    (zipcode),
    KEY idx_addresses_prefecture (prefecture),
    KEY idx_addresses_city       (city),
    KEY idx_addresses_town       (town)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS tokens (
    id           VARCHAR(36)  NOT NULL,
    name         VARCHAR(128) NOT NULL,
    prefix       VARCHAR(16)  NOT NULL,
    token_hash   VARCHAR(64)  NOT NULL,
    scope        VARCHAR(16)  NOT NULL,
    created_at   DATETIME(3)  NULL,
    expires_at   DATETIME(3)  NULL,
    last_used_at DATETIME(3)  NULL,
    revoked_at   DATETIME(3)  NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_tokens_token_hash (token_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sync_runs (
    id            VARCHAR(36)  NOT NULL,
    type          VARCHAR(8)   NOT NULL,
    status        VARCHAR(8)   NOT NULL,
    `trigger`     VARCHAR(8)   NOT NULL,
    source_url    VARCHAR(512) NOT NULL,
    file_checksum VARCHAR(128) NOT NULL,
    file_size     BIGINT       NOT NULL DEFAULT 0,
    diff_period   VARCHAR(8)   NOT NULL DEFAULT '',
    rows_added    BIGINT       NOT NULL DEFAULT 0,
    rows_updated  BIGINT       NOT NULL DEFAULT 0,
    rows_deleted  BIGINT       NOT NULL DEFAULT 0,
    rows_total    BIGINT       NOT NULL DEFAULT 0,
    started_at    DATETIME(3)  NOT NULL,
    finished_at   DATETIME(3)  NULL,
    duration_ms   BIGINT       NOT NULL DEFAULT 0,
    error_message TEXT         NOT NULL,
    PRIMARY KEY (id),
    KEY idx_sync_runs_type   (type),
    KEY idx_sync_runs_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sync_locks (
    id          INT          NOT NULL,
    locked      TINYINT(1)   NOT NULL DEFAULT 0,
    holder      VARCHAR(128) NOT NULL DEFAULT '',
    acquired_at DATETIME(3)  NULL,
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
