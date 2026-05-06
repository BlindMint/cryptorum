-- +goose Up

CREATE INDEX IF NOT EXISTS idx_book_added_at ON book(added_at DESC);
CREATE INDEX IF NOT EXISTS idx_book_library_added_at ON book(library_id, added_at DESC);
CREATE INDEX IF NOT EXISTS idx_book_file_book_id ON book_file(book_id);
CREATE INDEX IF NOT EXISTS idx_book_file_book_id_format ON book_file(book_id, format);
CREATE INDEX IF NOT EXISTS idx_book_metadata_title ON book_metadata(title);
CREATE INDEX IF NOT EXISTS idx_book_metadata_series ON book_metadata(series);
CREATE INDEX IF NOT EXISTS idx_reading_session_started_at ON reading_session(started_at);

-- +goose Down

DROP INDEX IF EXISTS idx_reading_session_started_at;
DROP INDEX IF EXISTS idx_book_metadata_series;
DROP INDEX IF EXISTS idx_book_metadata_title;
DROP INDEX IF EXISTS idx_book_file_book_id_format;
DROP INDEX IF EXISTS idx_book_file_book_id;
DROP INDEX IF EXISTS idx_book_library_added_at;
DROP INDEX IF EXISTS idx_book_added_at;
