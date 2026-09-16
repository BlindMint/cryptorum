-- +goose Up
CREATE TABLE audio_queue_item (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_user_id INTEGER NOT NULL DEFAULT 1,
    book_id       INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    file_id       INTEGER NOT NULL REFERENCES book_file(id) ON DELETE CASCADE,
    position      INTEGER NOT NULL,
    added_at      INTEGER NOT NULL,
    UNIQUE(owner_user_id, file_id)
);

CREATE INDEX idx_audio_queue_item_owner_position
    ON audio_queue_item(owner_user_id, position, id);

CREATE TABLE audio_queue_state (
    owner_user_id  INTEGER PRIMARY KEY,
    current_item_id INTEGER REFERENCES audio_queue_item(id) ON DELETE SET NULL,
    updated_at     INTEGER NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS audio_queue_state;
DROP INDEX IF EXISTS idx_audio_queue_item_owner_position;
DROP TABLE IF EXISTS audio_queue_item;
