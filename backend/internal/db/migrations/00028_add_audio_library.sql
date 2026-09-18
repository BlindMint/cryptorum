-- +goose Up
ALTER TABLE library ADD COLUMN audio_default_category TEXT NOT NULL DEFAULT 'audiobook';

CREATE TABLE audio_item (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_user_id      INTEGER NOT NULL DEFAULT 1,
    book_id            INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    file_id            INTEGER NOT NULL REFERENCES book_file(id) ON DELETE CASCADE,
    category           TEXT NOT NULL DEFAULT 'audiobook' CHECK(category IN ('audiobook', 'music', 'podcast')),
    title              TEXT NOT NULL DEFAULT '',
    artists            TEXT NOT NULL DEFAULT '[]',
    album_artist       TEXT NOT NULL DEFAULT '',
    album              TEXT NOT NULL DEFAULT '',
    track_number       INTEGER,
    disc_number        INTEGER,
    release_date       TEXT NOT NULL DEFAULT '',
    genre              TEXT NOT NULL DEFAULT '',
    duration_seconds   REAL NOT NULL DEFAULT 0,
    show_title         TEXT NOT NULL DEFAULT '',
    episode_number     INTEGER,
    published_at       TEXT NOT NULL DEFAULT '',
    metadata_source    TEXT NOT NULL DEFAULT 'backfill',
    locked_fields      TEXT NOT NULL DEFAULT '[]',
    source_hash        TEXT NOT NULL DEFAULT '',
    playback_speed     REAL,
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL,
    UNIQUE(owner_user_id, file_id)
);

CREATE INDEX idx_audio_item_category ON audio_item(owner_user_id, category, title COLLATE NOCASE);
CREATE INDEX idx_audio_item_album ON audio_item(owner_user_id, category, album COLLATE NOCASE, disc_number, track_number);
CREATE INDEX idx_audio_item_book ON audio_item(owner_user_id, book_id);

INSERT INTO audio_item (
    owner_user_id, book_id, file_id, category, title, artists, metadata_source,
    source_hash, created_at, updated_at
)
SELECT
    bf.owner_user_id, bf.book_id, bf.id, 'audiobook',
    COALESCE(NULLIF(bm.title, ''), bf.path),
    COALESCE(NULLIF(bm.authors, ''), '[]'), 'backfill', '',
    CAST(strftime('%s', 'now') AS INTEGER), CAST(strftime('%s', 'now') AS INTEGER)
FROM book_file bf
LEFT JOIN book_metadata bm ON bm.book_id = bf.book_id
WHERE bf.missing_at IS NULL
  AND lower(bf.format) IN ('mp3', 'm4a', 'm4b', 'flac', 'ogg', 'wav');

CREATE TABLE audio_chapter (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    audio_item_id     INTEGER NOT NULL REFERENCES audio_item(id) ON DELETE CASCADE,
    position          INTEGER NOT NULL,
    title             TEXT NOT NULL DEFAULT '',
    start_seconds     REAL NOT NULL,
    end_seconds       REAL NOT NULL,
    UNIQUE(audio_item_id, position)
);

CREATE TABLE audio_bookmark (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_user_id     INTEGER NOT NULL DEFAULT 1,
    audio_item_id     INTEGER NOT NULL REFERENCES audio_item(id) ON DELETE CASCADE,
    seconds           REAL NOT NULL CHECK(seconds >= 0),
    label             TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL
);

CREATE INDEX idx_audio_bookmark_item ON audio_bookmark(owner_user_id, audio_item_id, seconds);

CREATE TABLE audio_listening_state (
    owner_user_id     INTEGER NOT NULL DEFAULT 1,
    audio_item_id     INTEGER NOT NULL REFERENCES audio_item(id) ON DELETE CASCADE,
    position_seconds  REAL NOT NULL DEFAULT 0,
    duration_seconds  REAL NOT NULL DEFAULT 0,
    status            TEXT NOT NULL DEFAULT 'unplayed' CHECK(status IN ('unplayed', 'in_progress', 'played')),
    play_count        INTEGER NOT NULL DEFAULT 0,
    last_played_at    INTEGER,
    updated_at        INTEGER NOT NULL,
    PRIMARY KEY(owner_user_id, audio_item_id)
);

CREATE TABLE audio_playback_preference (
    owner_user_id     INTEGER NOT NULL DEFAULT 1,
    category          TEXT NOT NULL CHECK(category IN ('audiobook', 'podcast')),
    group_key         TEXT NOT NULL,
    playback_speed    REAL NOT NULL CHECK(playback_speed >= 0.5 AND playback_speed <= 3),
    updated_at        INTEGER NOT NULL,
    PRIMARY KEY(owner_user_id, category, group_key)
);

CREATE TABLE audio_playlist (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_user_id     INTEGER NOT NULL DEFAULT 1,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);

CREATE INDEX idx_audio_playlist_name ON audio_playlist(owner_user_id, name COLLATE NOCASE);

CREATE TABLE audio_playlist_item (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    playlist_id       INTEGER NOT NULL REFERENCES audio_playlist(id) ON DELETE CASCADE,
    audio_item_id     INTEGER NOT NULL REFERENCES audio_item(id) ON DELETE CASCADE,
    position          INTEGER NOT NULL,
    added_at          INTEGER NOT NULL,
    UNIQUE(playlist_id, audio_item_id)
);

CREATE INDEX idx_audio_playlist_item_position ON audio_playlist_item(playlist_id, position, id);

ALTER TABLE audio_queue_item ADD COLUMN audio_item_id INTEGER REFERENCES audio_item(id) ON DELETE CASCADE;
UPDATE audio_queue_item
SET audio_item_id = (SELECT ai.id FROM audio_item ai WHERE ai.file_id = audio_queue_item.file_id AND ai.owner_user_id = audio_queue_item.owner_user_id);
CREATE INDEX idx_audio_queue_item_audio ON audio_queue_item(owner_user_id, audio_item_id);

-- +goose Down
DROP INDEX IF EXISTS idx_audio_queue_item_audio;
ALTER TABLE audio_queue_item DROP COLUMN audio_item_id;
DROP TABLE IF EXISTS audio_playlist_item;
DROP TABLE IF EXISTS audio_playlist;
DROP TABLE IF EXISTS audio_listening_state;
DROP TABLE IF EXISTS audio_playback_preference;
DROP INDEX IF EXISTS idx_audio_bookmark_item;
DROP TABLE IF EXISTS audio_bookmark;
DROP TABLE IF EXISTS audio_chapter;
DROP INDEX IF EXISTS idx_audio_item_book;
DROP INDEX IF EXISTS idx_audio_item_album;
DROP INDEX IF EXISTS idx_audio_item_category;
DROP TABLE IF EXISTS audio_item;
ALTER TABLE library DROP COLUMN audio_default_category;
