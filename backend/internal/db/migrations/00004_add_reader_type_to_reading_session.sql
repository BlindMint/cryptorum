-- +goose Up
ALTER TABLE reading_session ADD COLUMN reader_type TEXT NOT NULL DEFAULT 'normal';

-- +goose Down
-- SQLite does not support dropping columns cleanly in-place; keep the schema change.
