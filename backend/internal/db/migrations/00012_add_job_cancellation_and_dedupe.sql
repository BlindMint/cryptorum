-- +goose Up
ALTER TABLE metadata_job ADD COLUMN cancel_requested_at INTEGER;
ALTER TABLE metadata_job ADD COLUMN dedupe_key TEXT;

CREATE INDEX IF NOT EXISTS idx_metadata_job_dedupe_key ON metadata_job(dedupe_key);
CREATE UNIQUE INDEX IF NOT EXISTS idx_metadata_job_active_dedupe
    ON metadata_job(dedupe_key)
    WHERE dedupe_key IS NOT NULL AND status IN ('queued', 'running', 'cancelling');

-- +goose Down
DROP INDEX IF EXISTS idx_metadata_job_active_dedupe;
DROP INDEX IF EXISTS idx_metadata_job_dedupe_key;
-- SQLite does not support dropping columns cleanly in-place; keep the schema change.
