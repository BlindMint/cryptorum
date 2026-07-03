-- +goose Up
ALTER TABLE book_file ADD COLUMN hash_algorithm TEXT NOT NULL DEFAULT 'sha256-full-v1';

-- Existing hashes were full-file SHA-256 for small files and sampled SHA-256
-- fingerprints for files over 10 MiB.
UPDATE book_file
SET hash_algorithm = 'sha256-sampled-v1'
WHERE size > 10485760;

CREATE INDEX IF NOT EXISTS idx_book_file_hash_algorithm
    ON book_file(hash_algorithm);

-- +goose Down
DROP INDEX IF EXISTS idx_book_file_hash_algorithm;
-- SQLite column rollback would require rebuilding book_file; keep this migration forward-only.
