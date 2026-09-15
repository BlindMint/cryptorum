-- +goose Up
-- Reading time is accumulated from visible, engaged reader heartbeats instead
-- of being inferred from the wall-clock span between opening and closing a tab.
ALTER TABLE reading_session ADD COLUMN active_seconds INTEGER NOT NULL DEFAULT 0 CHECK(active_seconds >= 0);
ALTER TABLE reading_session ADD COLUMN activity_tracked INTEGER NOT NULL DEFAULT 0 CHECK(activity_tracked IN (0, 1));
ALTER TABLE reading_session ADD COLUMN last_active_at INTEGER;

-- Sessions created by the unified position system can be identified reliably,
-- but their historical active time cannot. Retain a conservative estimate so
-- an upgrade does not turn known recent reading into multi-day elapsed time.
UPDATE reading_session
SET active_seconds = CASE
        WHEN COALESCE(ended_at, started_at) <= started_at THEN 0
        WHEN COALESCE(ended_at, started_at) - started_at > 1800 THEN 1800
        ELSE COALESCE(ended_at, started_at) - started_at
    END,
    activity_tracked = 1,
    last_active_at = CASE
        WHEN COALESCE(ended_at, started_at) <= started_at THEN started_at
        WHEN COALESCE(ended_at, started_at) - started_at > 1800 THEN started_at + 1800
        ELSE COALESCE(ended_at, started_at)
    END
WHERE COALESCE(reader_mode, '') <> '' OR last_client_sequence > 0;

-- +goose Down
-- Columns are intentionally retained because dropping columns is not safe on
-- every SQLite version supported by Cryptorum.
