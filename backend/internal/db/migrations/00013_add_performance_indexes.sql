-- +goose Up

CREATE INDEX IF NOT EXISTS idx_book_file_active_book_format
    ON book_file(book_id, format)
    WHERE missing_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_reading_progress_status_updated
    ON reading_progress(status, updated_at DESC, book_id);

CREATE INDEX IF NOT EXISTS idx_reading_progress_updated_book
    ON reading_progress(updated_at DESC, book_id);

CREATE INDEX IF NOT EXISTS idx_book_metadata_series_title
    ON book_metadata(series, title);

CREATE INDEX IF NOT EXISTS idx_book_metadata_language
    ON book_metadata(language);

CREATE INDEX IF NOT EXISTS idx_book_metadata_publisher
    ON book_metadata(publisher);

-- +goose Down

DROP INDEX IF EXISTS idx_book_metadata_publisher;
DROP INDEX IF EXISTS idx_book_metadata_language;
DROP INDEX IF EXISTS idx_book_metadata_series_title;
DROP INDEX IF EXISTS idx_reading_progress_updated_book;
DROP INDEX IF EXISTS idx_reading_progress_status_updated;
DROP INDEX IF EXISTS idx_book_file_active_book_format;
