-- +goose Up
ALTER TABLE book_metadata ADD COLUMN extracted_from_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE book_metadata ADD COLUMN metadata_updated_at INTEGER NOT NULL DEFAULT 0;

CREATE TABLE book_metadata_revision (
    id                     INTEGER PRIMARY KEY,
    book_id                INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    changed_at             INTEGER NOT NULL,
    changed_by_user_id     INTEGER,
    change_source          TEXT NOT NULL,
    changed_fields         TEXT NOT NULL DEFAULT '[]',
    previous_metadata_json TEXT NOT NULL
);

CREATE INDEX idx_book_metadata_revision_book
    ON book_metadata_revision(book_id, changed_at DESC, id DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_book_metadata_revision_book;
DROP TABLE IF EXISTS book_metadata_revision;
-- Added book_metadata columns are intentionally retained because older SQLite
-- versions cannot safely remove them in place.
