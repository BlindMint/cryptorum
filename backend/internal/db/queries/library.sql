-- name: GetLibrary :one
SELECT * FROM library WHERE id = ?;

-- name: ListLibraries :many
SELECT * FROM library ORDER BY name;

-- name: CreateLibrary :exec
INSERT INTO library (name, icon) VALUES (?, ?);

-- name: UpdateLibrary :exec
UPDATE library SET name = ?, icon = ? WHERE id = ?;

-- name: DeleteLibrary :exec
DELETE FROM library WHERE id = ?;

-- name: GetLibraryPaths :many
SELECT * FROM library_path WHERE library_id = ?;

-- name: CreateLibraryPath :exec
INSERT INTO library_path (library_id, path) VALUES (?, ?);

-- name: DeleteLibraryPaths :exec
DELETE FROM library_path WHERE library_id = ?;

-- name: GetBook :one
SELECT * FROM book WHERE id = ?;

-- name: ListBooks :many
SELECT * FROM book ORDER BY added_at DESC LIMIT ? OFFSET ?;

-- name: ListBooksByLibrary :many
SELECT * FROM book WHERE library_id = ? ORDER BY added_at DESC;

-- name: CreateBook :one
INSERT INTO book (library_id, added_at, last_scanned) VALUES (?, ?, ?) RETURNING id;

-- name: UpdateBookScanTime :exec
UPDATE book SET last_scanned = ? WHERE id = ?;

-- name: DeleteBook :exec
DELETE FROM book WHERE id = ?;

-- name: GetBookByHash :one
SELECT b.* FROM book b JOIN book_file bf ON b.id = bf.book_id WHERE bf.hash = ?;

-- name: GetBookFile :one
SELECT * FROM book_file WHERE id = ?;

-- name: GetBookFiles :many
SELECT * FROM book_file WHERE book_id = ?;

-- name: CreateBookFile :exec
INSERT INTO book_file (book_id, path, format, size, hash, last_modified) VALUES (?, ?, ?, ?, ?, ?);

-- name: UpdateBookFileHash :exec
UPDATE book_file SET hash = ?, last_modified = ? WHERE id = ?;

-- name: DeleteBookFiles :exec
DELETE FROM book_file WHERE book_id = ?;

-- name: GetBookMetadata :one
SELECT * FROM book_metadata WHERE book_id = ?;

-- name: CreateBookMetadata :exec
INSERT INTO book_metadata (book_id, title, authors, series, series_number, publisher, pub_date, description, rating, genres, isbn, cover_path, cover_updated_on, page_count, language, locked_fields) 
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateBookMetadata :exec
UPDATE book_metadata SET title = ?, authors = ?, series = ?, series_number = ?, publisher = ?, pub_date = ?, description = ?, rating = ?, genres = ?, isbn = ?, cover_path = ?, cover_updated_on = ?, page_count = ?, language = ?, locked_fields = ? 
WHERE book_id = ?;

-- name: DeleteBookMetadata :exec
DELETE FROM book_metadata WHERE book_id = ?;

-- name: SearchBooks :many
SELECT bm.*, b.id as book_id, b.library_id, b.added_at
FROM book_metadata bm
JOIN book b ON bm.book_id = b.id
WHERE book_fts MATCH ? OR bm.title LIKE ? OR bm.authors LIKE ?
ORDER BY bm.title
LIMIT ? OFFSET ?;

-- name: GetShelf :one
SELECT * FROM shelf WHERE id = ?;

-- name: ListShelves :many
SELECT * FROM shelf ORDER BY name;

-- name: CreateShelf :exec
INSERT INTO shelf (name, icon, is_magic, rules_json, sort_by, sort_dir) VALUES (?, ?, ?, ?, ?, ?);

-- name: UpdateShelf :exec
UPDATE shelf SET name = ?, icon = ?, is_magic = ?, rules_json = ?, sort_by = ?, sort_dir = ? WHERE id = ?;

-- name: DeleteShelf :exec
DELETE FROM shelf WHERE id = ?;

-- name: GetBooksOnShelf :many
SELECT b.* FROM book b
JOIN book_shelf bs ON b.id = bs.book_id
WHERE bs.shelf_id = ?
ORDER BY b.added_at DESC;

-- name: AddBookToShelf :exec
INSERT INTO book_shelf (book_id, shelf_id) VALUES (?, ?);

-- name: RemoveBookFromShelf :exec
DELETE FROM book_shelf WHERE book_id = ? AND shelf_id = ?;

-- name: GetReadingProgress :one
SELECT * FROM reading_progress WHERE book_id = ?;

-- name: CreateReadingProgress :exec
INSERT INTO reading_progress (book_id, file_id, percent, cfi, page, status, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateReadingProgress :exec
UPDATE reading_progress SET file_id = ?, percent = ?, cfi = ?, page = ?, status = ?, updated_at = ? WHERE book_id = ?;

-- name: UpdateSpeedReaderProgress :exec
UPDATE reading_progress SET speed_reader_word_index = ?, speed_reader_percent = ?, updated_at = ? WHERE book_id = ?;

-- name: DeleteReadingProgress :exec
DELETE FROM reading_progress WHERE book_id = ?;

-- name: GetReadingSession :one
SELECT * FROM reading_session WHERE id = ?;

-- name: CreateReadingSession :one
INSERT INTO reading_session (book_id, started_at) VALUES (?, ?) RETURNING id;

-- name: EndReadingSession :exec
UPDATE reading_session SET ended_at = ? WHERE id = ?;

-- name: GetReadingSessions :many
SELECT * FROM reading_session WHERE book_id = ? ORDER BY started_at DESC;

-- name: GetReadingHistory :many
SELECT rs.*, bm.title, bm.authors, bm.cover_path, rp.percent, rp.status
FROM reading_session rs
JOIN book b ON rs.book_id = b.id
LEFT JOIN book_metadata bm ON b.id = bm.book_id
LEFT JOIN reading_progress rp ON b.id = rp.book_id
WHERE rs.started_at > ?
ORDER BY rs.started_at DESC;

-- name: GetBookmark :one
SELECT * FROM bookmark WHERE id = ?;

-- name: GetBookBookmarks :many
SELECT * FROM bookmark WHERE book_id = ? ORDER BY created_at DESC;

-- name: CreateBookmark :exec
INSERT INTO bookmark (book_id, file_id, cfi, label, color, created_at) VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteBookmark :exec
DELETE FROM bookmark WHERE id = ?;

-- name: GetAnnotation :one
SELECT * FROM annotation WHERE id = ?;

-- name: GetBookAnnotations :many
SELECT * FROM annotation WHERE book_id = ? ORDER BY created_at DESC;

-- name: CreateAnnotation :exec
INSERT INTO annotation (book_id, file_id, cfi_start, cfi_end, selected_text, note, color, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateAnnotation :exec
UPDATE annotation SET selected_text = ?, note = ?, color = ? WHERE id = ?;

-- name: DeleteAnnotation :exec
DELETE FROM annotation WHERE id = ?;

-- name: GetNotebook :one
SELECT * FROM notebook WHERE id = ?;

-- name: ListNotebooks :many
SELECT * FROM notebook ORDER BY name;

-- name: CreateNotebook :one
INSERT INTO notebook (name, created_at) VALUES (?, ?) RETURNING id;

-- name: DeleteNotebook :exec
DELETE FROM notebook WHERE id = ?;

-- name: GetNotebookEntries :many
SELECT ne.*, b.id as book_id, bm.title as book_title
FROM notebook_entry ne
LEFT JOIN book b ON ne.book_id = b.id
LEFT JOIN book_metadata bm ON b.id = bm.book_id
WHERE ne.notebook_id = ?
ORDER BY ne.created_at DESC;

-- name: CreateNotebookEntry :exec
INSERT INTO notebook_entry (notebook_id, book_id, content, created_at) VALUES (?, ?, ?, ?);

-- name: DeleteNotebookEntry :exec
DELETE FROM notebook_entry WHERE id = ?;

-- name: GetBookdropFile :one
SELECT * FROM bookdrop_file WHERE id = ?;

-- name: ListBookdropFiles :many
SELECT * FROM bookdrop_file WHERE status = 'pending' ORDER BY added_at;

-- name: CreateBookdropFile :exec
INSERT INTO bookdrop_file (filename, path, status, error, added_at) VALUES (?, ?, ?, ?, ?);

-- name: UpdateBookdropFileStatus :exec
UPDATE bookdrop_file SET status = ?, error = ? WHERE id = ?;

-- name: DeleteBookdropFile :exec
DELETE FROM bookdrop_file WHERE id = ?;

-- name: GetAppSetting :one
SELECT * FROM app_settings WHERE key = ?;

-- name: ListAppSettings :many
SELECT * FROM app_settings ORDER BY key;

-- name: SetAppSetting :exec
INSERT OR REPLACE INTO app_settings (key, value) VALUES (?, ?);

-- name: DeleteAppSetting :exec
DELETE FROM app_settings WHERE key = ?;

-- name: GetTaskSchedule :one
SELECT * FROM task_schedule WHERE task_name = ?;

-- name: ListTaskSchedules :many
SELECT * FROM task_schedule ORDER BY task_name;

-- name: SetTaskSchedule :exec
INSERT OR REPLACE INTO task_schedule (task_name, cron_expr, last_run, enabled) VALUES (?, ?, ?, ?);

-- name: UpdateTaskLastRun :exec
UPDATE task_schedule SET last_run = ? WHERE task_name = ?;
