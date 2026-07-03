-- +goose Up
ALTER TABLE shelf ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

WITH ordered_shelves AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY owner_user_id
            ORDER BY name COLLATE NOCASE, id
        ) AS next_sort_order
    FROM shelf
)
UPDATE shelf
SET sort_order = (
    SELECT next_sort_order
    FROM ordered_shelves
    WHERE ordered_shelves.id = shelf.id
);

CREATE INDEX IF NOT EXISTS idx_shelf_owner_sort_order ON shelf(owner_user_id, sort_order, name);

-- +goose Down
DROP INDEX IF EXISTS idx_shelf_owner_sort_order;

-- SQLite cannot drop columns on older versions; leave sort_order in place.
