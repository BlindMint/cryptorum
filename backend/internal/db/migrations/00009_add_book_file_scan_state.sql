-- +goose Up
ALTER TABLE book_file ADD COLUMN missing_at INTEGER;
ALTER TABLE book_file ADD COLUMN scan_seen_at INTEGER;

CREATE INDEX IF NOT EXISTS idx_book_file_path ON book_file(path);
CREATE INDEX IF NOT EXISTS idx_book_file_missing_at ON book_file(missing_at);
CREATE INDEX IF NOT EXISTS idx_book_file_hash_size ON book_file(hash, size);

-- +goose Down
DROP INDEX IF EXISTS idx_book_file_hash_size;
DROP INDEX IF EXISTS idx_book_file_missing_at;
DROP INDEX IF EXISTS idx_book_file_path;
