-- +goose Up
ALTER TABLE book_metadata ADD COLUMN asin TEXT;

-- +goose Down
-- SQLite column rollback would require a table rebuild; keep this migration forward-only.
