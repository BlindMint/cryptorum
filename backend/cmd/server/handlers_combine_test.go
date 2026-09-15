package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cryptorum/internal/db"
)

func setupCombineTestDB(t *testing.T) {
	t.Helper()

	previousDB := appDB
	testDB, err := db.New(t.TempDir())
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	appDB = testDB
	t.Cleanup(func() {
		_ = testDB.Close()
		appDB = previousDB
	})

	mustExec(t, `
		INSERT INTO library (id, name, owner_user_id)
		VALUES (1, 'Main', 1), (2, 'Other', 1)
	`)
	mustExec(t, `
		INSERT INTO shelf (id, name, owner_user_id)
		VALUES (1, 'Keep', 1), (2, 'Also Keep', 1)
	`)
}

func insertCombineTestBook(t *testing.T, bookID, libraryID int64, title, path, format string) {
	t.Helper()
	mustExec(t, `
		INSERT INTO book (id, library_id, added_at, last_scanned, owner_user_id)
		VALUES (?, ?, 100, 100, 1)
	`, bookID, libraryID)
	mustExec(t, `
		INSERT INTO book_file (id, book_id, path, format, size, hash, hash_algorithm, last_modified, owner_user_id)
		VALUES (?, ?, ?, ?, 10, ?, 'sha256-full-v1', 100, 1)
	`, bookID, bookID, path, format, title+"-hash")
	mustExec(t, `
		INSERT INTO book_metadata (book_id, title, authors, genres, tags, cover_path, owner_user_id)
		VALUES (?, ?, '[]', '[]', '[]', ?, 1)
	`, bookID, title, "/covers/"+title+".jpg")
}

func TestCombineBooksMergesFilesAndRelatedRows(t *testing.T) {
	setupCombineTestDB(t)
	insertCombineTestBook(t, 10, 1, "Primary Title", "/books/primary.pdf", "pdf")
	insertCombineTestBook(t, 11, 1, "Secondary Title", "/books/secondary.pdf", "pdf")
	mustExec(t, `INSERT INTO book_shelf (book_id, shelf_id) VALUES (10, 1), (11, 2), (11, 1)`)
	mustExec(t, `INSERT INTO bookmark (book_id, file_id, cfi, label, created_at) VALUES (11, 11, 'cfi', 'mark', 100)`)
	mustExec(t, `INSERT INTO annotation (book_id, file_id, selected_text, created_at) VALUES (11, 11, 'note', 100)`)
	mustExec(t, `INSERT INTO reading_session (book_id, started_at, owner_user_id, file_id) VALUES (11, 100, 1, 11)`)
	mustExec(t, `
		INSERT INTO reading_progress (book_id, file_id, percent, status, updated_at, owner_user_id)
		VALUES (10, 10, 12, 'reading', 100, 1), (11, 11, 80, 'finished', 200, 1)
	`)
	mustExec(t, `
		INSERT INTO reading_position (book_id, file_id, owner_user_id, channel, percent, active_reader_mode, locators_json, source_hash, revision, updated_at_ms)
		VALUES (11, 11, 1, 'standard', 80, 'pdf', '{}', '', 1, 200)
	`)
	mustExec(t, `
		INSERT INTO book_fts(rowid, title, authors, description, series)
		SELECT id, title, '', '', '' FROM book_metadata WHERE book_id = 11
	`)

	req := httptest.NewRequest(http.MethodPost, "/api/books/combine", strings.NewReader(`{"primary_book_id":10,"book_ids":[10,11]}`))
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true}))
	rec := httptest.NewRecorder()
	combineBooksHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response combineBooksResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.PrimaryBookID != 10 || response.FileCount != 2 || len(response.MergedBookIDs) != 1 || response.MergedBookIDs[0] != 11 {
		t.Fatalf("unexpected response: %+v", response)
	}

	var fileCount int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM book_file WHERE book_id = 10`).Scan(&fileCount); err != nil {
		t.Fatalf("count files: %v", err)
	}
	if fileCount != 2 {
		t.Fatalf("primary file count = %d, want 2", fileCount)
	}

	var leftover int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM book WHERE id = 11`).Scan(&leftover); err != nil {
		t.Fatalf("count secondary book: %v", err)
	}
	if leftover != 0 {
		t.Fatalf("secondary book still exists")
	}

	var leftoverFTS int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM book_fts_docsize WHERE id IN (SELECT id FROM book_metadata WHERE book_id = 11)`).Scan(&leftoverFTS); err != nil {
		t.Fatalf("count leftover fts: %v", err)
	}
	if leftoverFTS != 0 {
		t.Fatalf("secondary fts rows still exist")
	}

	var primaryTitle string
	if err := appDB.QueryRow(`SELECT title FROM book_metadata WHERE book_id = 10`).Scan(&primaryTitle); err != nil {
		t.Fatalf("load primary title: %v", err)
	}
	if primaryTitle != "Primary Title" {
		t.Fatalf("primary title = %q, want Primary Title", primaryTitle)
	}

	var bookmarkBookID, annotationBookID, sessionBookID, positionBookID int64
	mustScan(t, `SELECT book_id FROM bookmark WHERE file_id = 11`, &bookmarkBookID)
	mustScan(t, `SELECT book_id FROM annotation WHERE file_id = 11`, &annotationBookID)
	mustScan(t, `SELECT book_id FROM reading_session WHERE file_id = 11`, &sessionBookID)
	mustScan(t, `SELECT book_id FROM reading_position WHERE file_id = 11`, &positionBookID)
	if bookmarkBookID != 10 || annotationBookID != 10 || sessionBookID != 10 || positionBookID != 10 {
		t.Fatalf("related rows not moved: bookmark=%d annotation=%d session=%d position=%d", bookmarkBookID, annotationBookID, sessionBookID, positionBookID)
	}

	var shelfCount int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM book_shelf WHERE book_id = 10 AND shelf_id IN (1, 2)`).Scan(&shelfCount); err != nil {
		t.Fatalf("count shelves: %v", err)
	}
	if shelfCount != 2 {
		t.Fatalf("primary shelf count = %d, want 2", shelfCount)
	}

	var status string
	var percent float64
	if err := appDB.QueryRow(`SELECT status, percent FROM reading_progress WHERE book_id = 10 AND owner_user_id = 1`).Scan(&status, &percent); err != nil {
		t.Fatalf("load primary progress: %v", err)
	}
	if status != "reading" || percent != 12 {
		t.Fatalf("primary progress status=%q percent=%v, want reading 12", status, percent)
	}

	var secondaryProgress int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM reading_progress WHERE book_id = 11`).Scan(&secondaryProgress); err != nil {
		t.Fatalf("count secondary progress: %v", err)
	}
	if secondaryProgress != 0 {
		t.Fatalf("secondary progress still present")
	}

	var pathCount int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM book_file WHERE path IN ('/books/primary.pdf', '/books/secondary.pdf')`).Scan(&pathCount); err != nil {
		t.Fatalf("count original paths: %v", err)
	}
	if pathCount != 2 {
		t.Fatalf("original file paths lost, count=%d", pathCount)
	}
}

func TestCombineBooksRejectsDifferentLibraries(t *testing.T) {
	setupCombineTestDB(t)
	insertCombineTestBook(t, 10, 1, "Main Book", "/books/main.pdf", "pdf")
	insertCombineTestBook(t, 12, 2, "Other Book", "/books/other.pdf", "pdf")

	req := httptest.NewRequest(http.MethodPost, "/api/books/combine", strings.NewReader(`{"primary_book_id":10,"book_ids":[10,12]}`))
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true}))
	rec := httptest.NewRecorder()
	combineBooksHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestCombineBooksRejectsPrimaryOutsideSelection(t *testing.T) {
	setupCombineTestDB(t)
	insertCombineTestBook(t, 10, 1, "Main Book", "/books/main.pdf", "pdf")
	insertCombineTestBook(t, 11, 1, "Other Book", "/books/other.pdf", "pdf")
	insertCombineTestBook(t, 13, 1, "Third Book", "/books/third.pdf", "pdf")

	req := httptest.NewRequest(http.MethodPost, "/api/books/combine", strings.NewReader(`{"primary_book_id":13,"book_ids":[10,11]}`))
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true}))
	rec := httptest.NewRecorder()
	combineBooksHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestCombineBooksRejectsSingleBook(t *testing.T) {
	setupCombineTestDB(t)
	insertCombineTestBook(t, 10, 1, "Main Book", "/books/main.pdf", "pdf")

	req := httptest.NewRequest(http.MethodPost, "/api/books/combine", strings.NewReader(`{"primary_book_id":10,"book_ids":[10,10]}`))
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true}))
	rec := httptest.NewRecorder()
	combineBooksHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func mustScan(t *testing.T, query string, dest ...any) {
	t.Helper()
	if err := appDB.QueryRow(query).Scan(dest...); err != nil {
		t.Fatalf("scan %s: %v", query, err)
	}
}
