-- +goose Up
ALTER TABLE book_metadata ADD COLUMN cover_updated_on INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite column rollback would require a table rebuild; keep this migration forward-only.
