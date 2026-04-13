-- +goose Up
CREATE TABLE metadata_job (
    id               INTEGER PRIMARY KEY,
    job_type         TEXT NOT NULL,
    title            TEXT NOT NULL,
    status           TEXT NOT NULL,
    payload_json     TEXT,
    result_json      TEXT,
    total_items      INTEGER NOT NULL DEFAULT 0,
    completed_items  INTEGER NOT NULL DEFAULT 0,
    failed_items     INTEGER NOT NULL DEFAULT 0,
    error            TEXT,
    created_at       INTEGER NOT NULL,
    started_at       INTEGER,
    completed_at     INTEGER
);

CREATE TABLE app_notification (
    id          INTEGER PRIMARY KEY,
    kind        TEXT NOT NULL,
    title       TEXT NOT NULL,
    message     TEXT,
    url         TEXT,
    read_at     INTEGER,
    created_at  INTEGER NOT NULL
);

CREATE TABLE app_log (
    id          INTEGER PRIMARY KEY,
    level       TEXT NOT NULL,
    category    TEXT NOT NULL,
    message     TEXT NOT NULL,
    data_json   TEXT,
    created_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_metadata_job_status_created_at ON metadata_job(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_app_notification_created_at ON app_notification(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_app_log_created_at ON app_log(created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_app_log_created_at;
DROP INDEX IF EXISTS idx_app_notification_created_at;
DROP INDEX IF EXISTS idx_metadata_job_status_created_at;
DROP TABLE IF EXISTS app_log;
DROP TABLE IF EXISTS app_notification;
DROP TABLE IF EXISTS metadata_job;
