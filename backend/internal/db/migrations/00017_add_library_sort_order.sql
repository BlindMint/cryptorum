-- +goose Up
ALTER TABLE library ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite cannot drop columns on older versions; leave sort_order in place.
