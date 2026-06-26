-- +goose Up
-- Keep the legacy genres column, but move its values into tags so the app can
-- use tags as the canonical taxonomy field.
WITH merged AS (
    SELECT book_id, TRIM(value) AS value, LOWER(TRIM(value)) AS key
    FROM book_metadata, json_each(COALESCE(tags, '[]'))
    WHERE TRIM(value) != ''
    UNION ALL
    SELECT book_id, TRIM(value) AS value, LOWER(TRIM(value)) AS key
    FROM book_metadata, json_each(COALESCE(genres, '[]'))
    WHERE TRIM(value) != ''
),
deduped AS (
    SELECT book_id, MIN(value) AS value
    FROM merged
    GROUP BY book_id, key
),
ordered AS (
    SELECT book_id, value
    FROM deduped
    ORDER BY book_id, LOWER(value), value
),
combined AS (
    SELECT book_id, json_group_array(value) AS tags
    FROM ordered
    GROUP BY book_id
)
UPDATE book_metadata
SET tags = COALESCE((SELECT tags FROM combined WHERE combined.book_id = book_metadata.book_id), '[]'),
    genres = '[]'
WHERE book_id IN (SELECT book_id FROM combined);

-- +goose Down
-- Data migration only; leave merged tags in place.
