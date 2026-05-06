-- +goose Up
CREATE TABLE app_user (
    id                INTEGER PRIMARY KEY,
    username          TEXT NOT NULL UNIQUE,
    password_hash     TEXT NOT NULL DEFAULT '',
    is_admin          INTEGER NOT NULL DEFAULT 0,
    is_bootstrap_admin INTEGER NOT NULL DEFAULT 0,
    permissions_json  TEXT NOT NULL DEFAULT '[]',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_app_user_username ON app_user(username);

ALTER TABLE library ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE shelf ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE book ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE book_file ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE book_metadata ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE reading_progress ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE reading_session ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE annotation ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE bookmark ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notebook ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE notebook_entry ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_library_owner_user_id ON library(owner_user_id, id);
CREATE INDEX IF NOT EXISTS idx_shelf_owner_user_id ON shelf(owner_user_id, id);
CREATE INDEX IF NOT EXISTS idx_book_owner_user_id ON book(owner_user_id, id);
CREATE INDEX IF NOT EXISTS idx_book_metadata_owner_user_id ON book_metadata(owner_user_id, book_id);
CREATE INDEX IF NOT EXISTS idx_reading_progress_owner_user_id ON reading_progress(owner_user_id, book_id);
CREATE INDEX IF NOT EXISTS idx_reading_session_owner_user_id ON reading_session(owner_user_id, book_id, started_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_reading_session_owner_user_id;
DROP INDEX IF EXISTS idx_reading_progress_owner_user_id;
DROP INDEX IF EXISTS idx_book_metadata_owner_user_id;
DROP INDEX IF EXISTS idx_book_owner_user_id;
DROP INDEX IF EXISTS idx_shelf_owner_user_id;
DROP INDEX IF EXISTS idx_library_owner_user_id;
DROP INDEX IF EXISTS idx_app_user_username;
DROP TABLE IF EXISTS app_user;
