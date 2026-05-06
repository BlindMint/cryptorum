-- +goose Up
-- Libraries and paths
CREATE TABLE library (
    id      INTEGER PRIMARY KEY,
    name    TEXT NOT NULL,
    icon    TEXT
);

CREATE TABLE library_path (
    id          INTEGER PRIMARY KEY,
    library_id  INTEGER NOT NULL REFERENCES library(id),
    path        TEXT NOT NULL
);

-- Books and files
CREATE TABLE book (
    id          INTEGER PRIMARY KEY,
    library_id  INTEGER NOT NULL REFERENCES library(id),
    added_at    INTEGER NOT NULL,  -- unix timestamp
    last_scanned INTEGER NOT NULL
);

CREATE TABLE book_file (
    id          INTEGER PRIMARY KEY,
    book_id     INTEGER NOT NULL REFERENCES book(id),
    path        TEXT NOT NULL,
    format      TEXT NOT NULL,     -- epub, pdf, cbz, cbr, cb7, mp3, m4b, etc.
    size        INTEGER NOT NULL,
    hash        TEXT NOT NULL,     -- SHA-256 for change/duplicate detection
    last_modified INTEGER NOT NULL
);

CREATE TABLE book_metadata (
    id              INTEGER PRIMARY KEY,
    book_id         INTEGER NOT NULL UNIQUE REFERENCES book(id),
    title           TEXT,
    authors         TEXT,          -- JSON array
    series          TEXT,
    series_number   REAL,
    publisher       TEXT,
    pub_date        TEXT,
    description     TEXT,
    rating          REAL,
    genres          TEXT,          -- JSON array
    isbn            TEXT,
    asin            TEXT,
    cover_path      TEXT,
    cover_updated_on INTEGER NOT NULL DEFAULT 0,
    page_count      INTEGER,
    language        TEXT,
    locked_fields   TEXT           -- JSON array of field names
);

-- Full-text search (kept in sync via triggers)
CREATE VIRTUAL TABLE book_fts USING fts5(
    title,
    authors,
    description,
    series,
    content='book_metadata',
    content_rowid='id'
);

-- Shelves (standard and magic)
CREATE TABLE shelf (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    icon        TEXT,
    is_magic    INTEGER NOT NULL DEFAULT 0,
    rules_json  TEXT,              -- JSON filter rules for magic shelves
    sort_by     TEXT,
    sort_dir    TEXT
);

CREATE TABLE book_shelf (
    book_id     INTEGER NOT NULL REFERENCES book(id),
    shelf_id    INTEGER NOT NULL REFERENCES shelf(id),
    PRIMARY KEY (book_id, shelf_id)
);

-- Reading
CREATE TABLE reading_progress (
    id                      INTEGER PRIMARY KEY,
    book_id                 INTEGER NOT NULL UNIQUE REFERENCES book(id),
    file_id                 INTEGER REFERENCES book_file(id),
    percent                 REAL,
    cfi                     TEXT,              -- EPUB Canonical Fragment Identifier
    page                    INTEGER,           -- PDF page number
    status                  TEXT NOT NULL DEFAULT 'unread',  -- unread, reading, finished
    speed_reader_word_index INTEGER,           -- last word index in speed reader
    speed_reader_percent    REAL,              -- speed reader progress (separate from normal)
    updated_at              INTEGER NOT NULL
);

CREATE TABLE reading_session (
    id          INTEGER PRIMARY KEY,
    book_id     INTEGER NOT NULL REFERENCES book(id),
    started_at  INTEGER NOT NULL,
    ended_at    INTEGER
);

-- Admin tracking
CREATE TABLE metadata_job (
    id               INTEGER PRIMARY KEY,
    job_type         TEXT NOT NULL,
    title            TEXT NOT NULL,
    status           TEXT NOT NULL,
    payload_json     TEXT,
    result_json      TEXT,
    total_items      INTEGER NOT NULL DEFAULT 0,
    completed_items  INTEGER NOT NULL DEFAULT 0,
    failed_items     INTEGER NOT NULL DEFAULT 0,
    error            TEXT,
    created_at       INTEGER NOT NULL,
    started_at       INTEGER,
    completed_at     INTEGER
);

CREATE TABLE app_notification (
    id          INTEGER PRIMARY KEY,
    kind        TEXT NOT NULL,
    title       TEXT NOT NULL,
    message     TEXT,
    url         TEXT,
    read_at     INTEGER,
    created_at  INTEGER NOT NULL
);

CREATE TABLE app_log (
    id          INTEGER PRIMARY KEY,
    level       TEXT NOT NULL,
    category    TEXT NOT NULL,
    message     TEXT NOT NULL,
    data_json   TEXT,
    created_at  INTEGER NOT NULL
);

-- Annotations
CREATE TABLE bookmark (
    id          INTEGER PRIMARY KEY,
    book_id     INTEGER NOT NULL REFERENCES book(id),
    file_id     INTEGER REFERENCES book_file(id),
    cfi         TEXT,
    label       TEXT,
    color       TEXT,
    created_at  INTEGER NOT NULL
);

CREATE TABLE annotation (
    id          INTEGER PRIMARY KEY,
    book_id     INTEGER NOT NULL REFERENCES book(id),
    file_id     INTEGER REFERENCES book_file(id),
    cfi_start   TEXT,
    cfi_end     TEXT,
    selected_text TEXT,
    note        TEXT,
    color       TEXT,
    created_at  INTEGER NOT NULL
);

-- Notebooks
CREATE TABLE notebook (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  INTEGER NOT NULL
);

CREATE TABLE notebook_entry (
    id          INTEGER PRIMARY KEY,
    notebook_id INTEGER NOT NULL REFERENCES notebook(id),
    book_id     INTEGER REFERENCES book(id),
    content     TEXT,
    created_at  INTEGER NOT NULL
);

-- BookDrop import queue
CREATE TABLE bookdrop_file (
    id          INTEGER PRIMARY KEY,
    filename    TEXT NOT NULL,
    path        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending, imported, failed
    error       TEXT,
    added_at    INTEGER NOT NULL
);

-- Key-value settings store (replaces all per-user preference tables)
CREATE TABLE app_settings (
    key         TEXT PRIMARY KEY,
    value       TEXT
);

-- Background task schedule
CREATE TABLE task_schedule (
    task_name   TEXT PRIMARY KEY,
    cron_expr   TEXT NOT NULL,
    last_run    INTEGER,
    enabled     INTEGER NOT NULL DEFAULT 1
);

-- FTS is automatically synced via content table

-- +goose Down

DROP TABLE task_schedule;
DROP TABLE app_settings;
DROP TABLE bookdrop_file;
DROP TABLE notebook_entry;
DROP TABLE notebook;
DROP TABLE annotation;
DROP TABLE bookmark;
DROP TABLE reading_session;
DROP TABLE reading_progress;
DROP TABLE book_shelf;
DROP TABLE shelf;
DROP TABLE book_fts;
DROP TABLE book_metadata;
DROP TABLE book_file;
DROP TABLE book;
DROP TABLE library_path;
DROP TABLE library;
