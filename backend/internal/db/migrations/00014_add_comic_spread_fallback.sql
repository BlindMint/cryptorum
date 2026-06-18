-- +goose Up
ALTER TABLE library ADD COLUMN comic_spread_fallback TEXT NOT NULL DEFAULT 'inherit';
ALTER TABLE book_metadata ADD COLUMN comic_spread_fallback TEXT NOT NULL DEFAULT 'inherit';

-- +goose Down
-- SQLite column rollback would require rebuilding these tables; keep this migration forward-only.
