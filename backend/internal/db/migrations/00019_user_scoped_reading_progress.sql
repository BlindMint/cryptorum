-- +goose Up
-- Store reading state per user/book instead of globally per book.
CREATE TABLE reading_progress_new (
    id                      INTEGER PRIMARY KEY,
    book_id                 INTEGER NOT NULL REFERENCES book(id),
    file_id                 INTEGER REFERENCES book_file(id),
    percent                 REAL,
    cfi                     TEXT,
    page                    INTEGER,
    status                  TEXT NOT NULL DEFAULT 'unread',
    speed_reader_word_index INTEGER,
    speed_reader_percent    REAL,
    updated_at              INTEGER NOT NULL DEFAULT 0,
    owner_user_id           INTEGER NOT NULL DEFAULT 1,
    UNIQUE(book_id, owner_user_id)
);

INSERT INTO reading_progress_new (
    id, book_id, file_id, percent, cfi, page, status,
    speed_reader_word_index, speed_reader_percent, updated_at, owner_user_id
)
SELECT
    id, book_id, file_id, percent, cfi, page, status,
    speed_reader_word_index, speed_reader_percent, updated_at,
    COALESCE(NULLIF(owner_user_id, 0), 1)
FROM reading_progress;

DROP TABLE reading_progress;
ALTER TABLE reading_progress_new RENAME TO reading_progress;

CREATE INDEX IF NOT EXISTS idx_reading_progress_owner_user_id
    ON reading_progress(owner_user_id, book_id);
CREATE INDEX IF NOT EXISTS idx_reading_progress_status_owner
    ON reading_progress(owner_user_id, status, updated_at DESC);

-- +goose Down
-- Collapse back to one reading-progress row per book, keeping the newest row.
CREATE TABLE reading_progress_old (
    id                      INTEGER PRIMARY KEY,
    book_id                 INTEGER NOT NULL UNIQUE REFERENCES book(id),
    file_id                 INTEGER REFERENCES book_file(id),
    percent                 REAL,
    cfi                     TEXT,
    page                    INTEGER,
    status                  TEXT NOT NULL DEFAULT 'unread',
    speed_reader_word_index INTEGER,
    speed_reader_percent    REAL,
    updated_at              INTEGER NOT NULL DEFAULT 0,
    owner_user_id           INTEGER NOT NULL DEFAULT 1
);

INSERT INTO reading_progress_old (
    id, book_id, file_id, percent, cfi, page, status,
    speed_reader_word_index, speed_reader_percent, updated_at, owner_user_id
)
SELECT
    rp.id, rp.book_id, rp.file_id, rp.percent, rp.cfi, rp.page, rp.status,
    rp.speed_reader_word_index, rp.speed_reader_percent, rp.updated_at, rp.owner_user_id
FROM reading_progress rp
WHERE rp.id = (
    SELECT rp2.id
    FROM reading_progress rp2
    WHERE rp2.book_id = rp.book_id
    ORDER BY rp2.updated_at DESC, rp2.id DESC
    LIMIT 1
);

DROP TABLE reading_progress;
ALTER TABLE reading_progress_old RENAME TO reading_progress;

CREATE INDEX IF NOT EXISTS idx_reading_progress_owner_user_id
    ON reading_progress(owner_user_id, book_id);
