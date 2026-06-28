-- 0003_sync_skipped_rows (MySQL 8) — import-time town regex skip audit log.

ALTER TABLE sync_runs ADD COLUMN rows_skipped BIGINT NOT NULL DEFAULT 0 AFTER rows_deleted;

CREATE TABLE IF NOT EXISTS sync_skipped_rows (
    id              BIGINT       NOT NULL AUTO_INCREMENT,
    run_id          VARCHAR(36)  NOT NULL,
    source_type     VARCHAR(16)  NOT NULL,
    line_number     INT          NOT NULL DEFAULT 0,
    zipcode         VARCHAR(7)   NOT NULL DEFAULT '',
    jis_code        VARCHAR(5)   NOT NULL DEFAULT '',
    prefecture      VARCHAR(64)  NOT NULL DEFAULT '',
    city            VARCHAR(128) NOT NULL DEFAULT '',
    town            VARCHAR(256) NOT NULL DEFAULT '',
    town_kana       VARCHAR(256) NOT NULL DEFAULT '',
    reason          VARCHAR(64)  NOT NULL DEFAULT '',
    pattern         VARCHAR(1024) NOT NULL DEFAULT '',
    raw_record_json TEXT         NOT NULL,
    created_at      DATETIME(3)  NULL,
    PRIMARY KEY (id),
    KEY idx_sync_skipped_rows_run_id (run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
