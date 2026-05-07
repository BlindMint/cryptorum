-- +goose Up
CREATE TABLE auth_session (
    id           INTEGER PRIMARY KEY,
    token_hash   TEXT NOT NULL UNIQUE,
    user_id      INTEGER NOT NULL,
    created_at   INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    revoked_at   INTEGER,
    FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_auth_session_token_hash ON auth_session(token_hash);
CREATE INDEX IF NOT EXISTS idx_auth_session_user_expires ON auth_session(user_id, expires_at);
CREATE INDEX IF NOT EXISTS idx_auth_session_expires ON auth_session(expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_auth_session_expires;
DROP INDEX IF EXISTS idx_auth_session_user_expires;
DROP INDEX IF EXISTS idx_auth_session_token_hash;
DROP TABLE IF EXISTS auth_session;
