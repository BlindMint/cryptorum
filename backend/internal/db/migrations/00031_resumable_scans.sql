-- +goose Up
ALTER TABLE metadata_job ADD COLUMN recovered_at INTEGER;
ALTER TABLE metadata_job ADD COLUMN rescan_requested INTEGER NOT NULL DEFAULT 0;
CREATE TABLE scan_file_checkpoint (
 job_id INTEGER NOT NULL REFERENCES metadata_job(id) ON DELETE CASCADE,
 path TEXT NOT NULL,
 size INTEGER NOT NULL,
 modified_ns INTEGER NOT NULL,
 status TEXT NOT NULL,
 error TEXT NOT NULL DEFAULT '',
 PRIMARY KEY (job_id, path)
);
ALTER TABLE book_file ADD COLUMN indexing_complete INTEGER NOT NULL DEFAULT 1;
DROP INDEX idx_metadata_job_active_dedupe;
CREATE UNIQUE INDEX idx_metadata_job_active_dedupe ON metadata_job(dedupe_key)
 WHERE dedupe_key IS NOT NULL AND status IN ('queued','running','cancelling','pausing','paused');

-- +goose Down
DROP TABLE scan_file_checkpoint;
DROP INDEX idx_metadata_job_active_dedupe;
CREATE UNIQUE INDEX idx_metadata_job_active_dedupe ON metadata_job(dedupe_key)
 WHERE dedupe_key IS NOT NULL AND status IN ('queued','running','cancelling');
