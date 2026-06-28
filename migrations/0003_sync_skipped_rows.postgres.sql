-- 0003_sync_skipped_rows (PostgreSQL) — import-time town regex skip audit log.

ALTER TABLE sync_runs ADD COLUMN IF NOT EXISTS rows_skipped BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS sync_skipped_rows (
    id              BIGSERIAL    PRIMARY KEY,
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
    created_at      TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_sync_skipped_rows_run_id ON sync_skipped_rows (run_id);
