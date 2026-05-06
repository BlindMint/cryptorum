-- +goose Up
ALTER TABLE library ADD COLUMN exclude_from_suggestions INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite cannot drop columns on older deployments without rebuilding the table.
