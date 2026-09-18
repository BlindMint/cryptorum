-- +goose Up
ALTER TABLE library ADD COLUMN media_scope TEXT NOT NULL DEFAULT 'mixed'
    CHECK(media_scope IN ('mixed', 'books', 'audio'));

-- +goose Down
ALTER TABLE library DROP COLUMN media_scope;
