-- +goose Up
-- Store exact reader positions independently from the lightweight per-book summary.
CREATE TABLE reading_position (
    id                   INTEGER PRIMARY KEY,
    book_id              INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    file_id              INTEGER NOT NULL REFERENCES book_file(id) ON DELETE CASCADE,
    owner_user_id        INTEGER NOT NULL DEFAULT 1,
    channel              TEXT NOT NULL CHECK(channel IN ('standard', 'speed')),
    percent              REAL NOT NULL DEFAULT 0 CHECK(percent >= 0 AND percent <= 100),
    active_reader_mode   TEXT NOT NULL,
    locators_json        TEXT NOT NULL DEFAULT '{}',
    source_hash          TEXT NOT NULL DEFAULT '',
    revision             INTEGER NOT NULL DEFAULT 0,
    updated_at_ms        INTEGER NOT NULL DEFAULT 0,
    UNIQUE(owner_user_id, file_id, channel)
);

CREATE INDEX idx_reading_position_book_channel
    ON reading_position(owner_user_id, book_id, channel, updated_at_ms DESC);

ALTER TABLE reading_progress ADD COLUMN standard_updated_at_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE reading_progress ADD COLUMN speed_updated_at_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE reading_progress ADD COLUMN speed_file_id INTEGER REFERENCES book_file(id);
ALTER TABLE reading_progress ADD COLUMN legacy_standard_adopted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE reading_progress ADD COLUMN legacy_speed_adopted INTEGER NOT NULL DEFAULT 0;

ALTER TABLE reading_session ADD COLUMN file_id INTEGER REFERENCES book_file(id);
ALTER TABLE reading_session ADD COLUMN channel TEXT NOT NULL DEFAULT 'standard';
ALTER TABLE reading_session ADD COLUMN reader_mode TEXT NOT NULL DEFAULT '';
ALTER TABLE reading_session ADD COLUMN superseded_at INTEGER;
ALTER TABLE reading_session ADD COLUMN last_client_sequence INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_reading_session_authority
    ON reading_session(owner_user_id, book_id, channel, superseded_at, started_at DESC);

-- Preserve positions when an exact legacy file is already known. Locators are
-- adopted lazily by the API so incompatible CFI/page values are never guessed.
INSERT INTO reading_position (
    book_id, file_id, owner_user_id, channel, percent, active_reader_mode,
    locators_json, source_hash, revision, updated_at_ms
)
SELECT
    rp.book_id, rp.file_id, rp.owner_user_id, 'standard',
    MIN(100, MAX(0, COALESCE(rp.percent, 0))),
    CASE
        WHEN LOWER(bf.format) = 'epub' THEN 'epub_paginated'
        WHEN LOWER(bf.format) = 'pdf' THEN 'pdf'
        WHEN LOWER(bf.format) IN ('cbz', 'cbr', 'cb7', 'cbt') THEN 'comic'
        WHEN LOWER(bf.format) IN ('mp3', 'm4a', 'm4b', 'flac', 'ogg', 'opus', 'wav', 'aac') THEN 'audio'
        ELSE 'continuous_text'
    END,
    CASE
        WHEN LOWER(bf.format) = 'epub' AND COALESCE(rp.cfi, '') <> '' THEN
            json_object('epub_paginated', json_object('revision', 1, 'locator', json_object('type', 'epub_cfi', 'cfi', rp.cfi)))
        WHEN LOWER(bf.format) = 'pdf' AND COALESCE(rp.page, 0) > 0 THEN
            json_object('pdf', json_object('revision', 1, 'locator', json_object('type', 'pdf_page', 'page', rp.page)))
        WHEN LOWER(bf.format) IN ('cbz', 'cbr', 'cb7', 'cbt') AND COALESCE(rp.page, 0) > 0 THEN
            json_object('comic', json_object('revision', 1, 'locator', json_object('type', 'comic_page', 'page', rp.page)))
        ELSE '{}'
    END,
    COALESCE(bf.hash, ''), 1, COALESCE(rp.updated_at, 0) * 1000
FROM reading_progress rp
JOIN book_file bf ON bf.id = rp.file_id AND bf.book_id = rp.book_id AND bf.missing_at IS NULL
WHERE COALESCE(rp.percent, 0) > 0;

UPDATE reading_progress
SET legacy_standard_adopted = 1
WHERE EXISTS (
    SELECT 1 FROM reading_position pos
    WHERE pos.owner_user_id = reading_progress.owner_user_id
      AND pos.book_id = reading_progress.book_id
      AND pos.channel = 'standard'
);

INSERT INTO reading_position (
    book_id, file_id, owner_user_id, channel, percent, active_reader_mode,
    locators_json, source_hash, revision, updated_at_ms
)
SELECT
    rp.book_id, rp.file_id, rp.owner_user_id, 'speed',
    MIN(100, MAX(0, COALESCE(rp.speed_reader_percent, 0))),
    'speed',
    CASE WHEN COALESCE(rp.speed_reader_word_index, 0) > 0 THEN
        json_object('speed', json_object('revision', 1, 'locator', json_object('type', 'word_index', 'word_index', rp.speed_reader_word_index)))
        ELSE '{}'
    END,
    COALESCE(bf.hash, ''), 1, COALESCE(rp.updated_at, 0) * 1000
FROM reading_progress rp
JOIN book_file bf ON bf.id = rp.file_id AND bf.book_id = rp.book_id AND bf.missing_at IS NULL
WHERE COALESCE(rp.speed_reader_percent, 0) > 0;

UPDATE reading_progress
SET legacy_speed_adopted = 1,
    speed_file_id = file_id
WHERE EXISTS (
    SELECT 1 FROM reading_position pos
    WHERE pos.owner_user_id = reading_progress.owner_user_id
      AND pos.book_id = reading_progress.book_id
      AND pos.channel = 'speed'
);

-- Mark Unread is a reset operation regardless of which status API initiated it.
-- +goose StatementBegin
CREATE TRIGGER reading_progress_unread_reset
AFTER UPDATE OF status ON reading_progress
WHEN NEW.status = 'unread' AND OLD.status <> 'unread'
BEGIN
    DELETE FROM reading_position
    WHERE owner_user_id = NEW.owner_user_id AND book_id = NEW.book_id;

    UPDATE reading_progress
    SET file_id = NULL,
        percent = 0,
        cfi = NULL,
        page = NULL,
        speed_reader_word_index = NULL,
        speed_reader_percent = 0,
        speed_file_id = NULL,
        standard_updated_at_ms = 0,
        speed_updated_at_ms = 0,
        legacy_standard_adopted = 1,
        legacy_speed_adopted = 1
    WHERE id = NEW.id;

    UPDATE reading_session
    SET superseded_at = COALESCE(superseded_at, CAST(strftime('%s', 'now') AS INTEGER)),
        ended_at = COALESCE(ended_at, CAST(strftime('%s', 'now') AS INTEGER))
    WHERE owner_user_id = NEW.owner_user_id
      AND book_id = NEW.book_id
      AND superseded_at IS NULL;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS reading_progress_unread_reset;
DROP INDEX IF EXISTS idx_reading_session_authority;
DROP INDEX IF EXISTS idx_reading_position_book_channel;
DROP TABLE IF EXISTS reading_position;
-- Added columns are intentionally retained because older SQLite versions cannot
-- safely remove them in place.
