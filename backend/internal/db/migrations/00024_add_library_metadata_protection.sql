-- +goose Up
ALTER TABLE library ADD COLUMN metadata_protection_enabled INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE library DROP COLUMN metadata_protection_enabled;
