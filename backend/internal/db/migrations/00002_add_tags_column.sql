-- +goose Up
-- Add tags column to book_metadata for separate tag support

ALTER TABLE book_metadata ADD COLUMN tags TEXT;

-- +goose Down
-- SQLite column rollback would require a table rebuild; keep this migration forward-only.
