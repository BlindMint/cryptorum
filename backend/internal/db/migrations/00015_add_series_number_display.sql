-- +goose Up
ALTER TABLE book_metadata ADD COLUMN series_number_display TEXT;

CREATE INDEX IF NOT EXISTS idx_book_metadata_series_number
    ON book_metadata(series, series_number, title);

-- +goose Down
DROP INDEX IF EXISTS idx_book_metadata_series_number;

ALTER TABLE book_metadata DROP COLUMN series_number_display;
