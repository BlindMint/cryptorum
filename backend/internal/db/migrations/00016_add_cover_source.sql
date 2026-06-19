-- +goose Up
ALTER TABLE book_metadata ADD COLUMN cover_source TEXT NOT NULL DEFAULT '';

-- +goose Down
-- SQLite cannot drop columns on older versions; leave cover_source in place.
