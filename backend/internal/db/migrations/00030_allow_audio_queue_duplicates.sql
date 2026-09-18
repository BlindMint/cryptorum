-- +goose Up
DROP INDEX IF EXISTS idx_audio_queue_item_audio;
DROP INDEX IF EXISTS idx_audio_queue_item_owner_position;

ALTER TABLE audio_queue_state RENAME TO audio_queue_state_unique_files;
ALTER TABLE audio_queue_item RENAME TO audio_queue_item_unique_files;

CREATE TABLE audio_queue_item (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_user_id INTEGER NOT NULL DEFAULT 1,
    book_id       INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    file_id       INTEGER NOT NULL REFERENCES book_file(id) ON DELETE CASCADE,
    position      INTEGER NOT NULL,
    added_at      INTEGER NOT NULL,
    audio_item_id INTEGER REFERENCES audio_item(id) ON DELETE CASCADE
);

CREATE INDEX idx_audio_queue_item_owner_position
    ON audio_queue_item(owner_user_id, position, id);
CREATE INDEX idx_audio_queue_item_audio
    ON audio_queue_item(owner_user_id, audio_item_id);

INSERT INTO audio_queue_item (id, owner_user_id, book_id, file_id, position, added_at, audio_item_id)
SELECT id, owner_user_id, book_id, file_id, position, added_at, audio_item_id
FROM audio_queue_item_unique_files;

CREATE TABLE audio_queue_state (
    owner_user_id   INTEGER PRIMARY KEY,
    current_item_id INTEGER REFERENCES audio_queue_item(id) ON DELETE SET NULL,
    updated_at      INTEGER NOT NULL
);

INSERT INTO audio_queue_state (owner_user_id, current_item_id, updated_at)
SELECT owner_user_id, current_item_id, updated_at
FROM audio_queue_state_unique_files;

DROP TABLE audio_queue_state_unique_files;
DROP TABLE audio_queue_item_unique_files;

-- +goose Down
DROP INDEX IF EXISTS idx_audio_queue_item_audio;
DROP INDEX IF EXISTS idx_audio_queue_item_owner_position;

ALTER TABLE audio_queue_state RENAME TO audio_queue_state_with_duplicates;
ALTER TABLE audio_queue_item RENAME TO audio_queue_item_with_duplicates;

CREATE TABLE audio_queue_item (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_user_id INTEGER NOT NULL DEFAULT 1,
    book_id       INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    file_id       INTEGER NOT NULL REFERENCES book_file(id) ON DELETE CASCADE,
    position      INTEGER NOT NULL,
    added_at      INTEGER NOT NULL,
    audio_item_id INTEGER REFERENCES audio_item(id) ON DELETE CASCADE,
    UNIQUE(owner_user_id, file_id)
);

CREATE INDEX idx_audio_queue_item_owner_position
    ON audio_queue_item(owner_user_id, position, id);
CREATE INDEX idx_audio_queue_item_audio
    ON audio_queue_item(owner_user_id, audio_item_id);

INSERT INTO audio_queue_item (id, owner_user_id, book_id, file_id, position, added_at, audio_item_id)
SELECT q.id, q.owner_user_id, q.book_id, q.file_id, q.position, q.added_at, q.audio_item_id
FROM audio_queue_item_with_duplicates q
WHERE q.id = (
    SELECT MIN(candidate.id)
    FROM audio_queue_item_with_duplicates candidate
    WHERE candidate.owner_user_id = q.owner_user_id AND candidate.file_id = q.file_id
);

CREATE TABLE audio_queue_state (
    owner_user_id   INTEGER PRIMARY KEY,
    current_item_id INTEGER REFERENCES audio_queue_item(id) ON DELETE SET NULL,
    updated_at      INTEGER NOT NULL
);

INSERT INTO audio_queue_state (owner_user_id, current_item_id, updated_at)
SELECT state.owner_user_id,
       COALESCE(
           (SELECT kept.id FROM audio_queue_item kept WHERE kept.id = state.current_item_id),
           (SELECT kept.id FROM audio_queue_item_with_duplicates old
            JOIN audio_queue_item kept ON kept.owner_user_id = old.owner_user_id AND kept.file_id = old.file_id
            WHERE old.id = state.current_item_id)
       ),
       state.updated_at
FROM audio_queue_state_with_duplicates state;

DROP TABLE audio_queue_state_with_duplicates;
DROP TABLE audio_queue_item_with_duplicates;
