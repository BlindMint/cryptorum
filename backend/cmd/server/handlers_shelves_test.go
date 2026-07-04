package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cryptorum/internal/db"

	"github.com/go-chi/chi/v5"
)

func setupShelfHandlerTestDB(t *testing.T) {
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
		VALUES (1, 'Main', 1), (2, 'Other', 2)
	`)
	insertShelfTestBook(t, 1, 1, 1, "Active Reading", "reading", false)
	insertShelfTestBook(t, 2, 1, 1, "Missing Reading", "reading", true)
	insertShelfTestBook(t, 3, 2, 2, "Other User Reading", "reading", false)
	mustExec(t, `
		INSERT INTO shelf (id, name, icon, is_magic, rules_json, sort_by, sort_dir, owner_user_id, sort_order)
		VALUES
			(1, 'Manual', 'bookmark', 0, '', 'name', 'asc', 1, 1),
			(2, 'Magic', 'sparkles', 1, '{"conditions":[{"field":"status","operator":"equals","value":"reading"}]}', 'added_at', 'desc', 1, 2)
	`)
	mustExec(t, `INSERT INTO book_shelf (book_id, shelf_id) VALUES (1, 1), (2, 1)`)
}

func insertShelfTestBook(t *testing.T, bookID, libraryID, ownerID int64, title, status string, missing bool) {
	t.Helper()
	mustExec(t, `
		INSERT INTO book (id, library_id, added_at, last_scanned, owner_user_id)
		VALUES (?, ?, ?, ?, ?)
	`, bookID, libraryID, bookID*100, bookID*100, ownerID)
	mustExec(t, `
		INSERT INTO book_file (id, book_id, path, format, size, hash, last_modified, missing_at, owner_user_id)
		VALUES (?, ?, ?, 'epub', 123, ?, ?, ?, ?)
	`, bookID, bookID, title+".epub", title+"-hash", bookID*100, nullableMissingAt(missing), ownerID)
	mustExec(t, `
		INSERT INTO book_metadata (book_id, title, authors, tags, owner_user_id)
		VALUES (?, ?, '[]', '[]', ?)
	`, bookID, title, ownerID)
	mustExec(t, `
		INSERT INTO reading_progress (book_id, percent, status, updated_at, owner_user_id)
		VALUES (?, 0, ?, ?, ?)
	`, bookID, status, bookID*100, ownerID)
}

func nullableMissingAt(missing bool) interface{} {
	if missing {
		return int64(999)
	}
	return nil
}

func TestGetShelvesCountsActiveManualAndMagicBooks(t *testing.T) {
	setupShelfHandlerTestDB(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/shelves", nil)
	getShelvesHandler(rec, req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1})))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var shelves []ShelfResponse
	if err := json.NewDecoder(rec.Body).Decode(&shelves); err != nil {
		t.Fatalf("decode shelves: %v", err)
	}
	if len(shelves) != 2 {
		t.Fatalf("expected 2 shelves, got %+v", shelves)
	}
	for _, shelf := range shelves {
		if shelf.BookCount != 1 {
			t.Fatalf("shelf %q count = %d, want 1 active in-scope book", shelf.Name, shelf.BookCount)
		}
	}
}

func TestGetShelfBooksExcludesMissingManualBooks(t *testing.T) {
	setupShelfHandlerTestDB(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/shelves/1/books", nil)
	req = requestWithShelfID(req, "1")
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1}))

	getShelfBooksHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var books []struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&books); err != nil {
		t.Fatalf("decode shelf books: %v", err)
	}
	if len(books) != 1 || books[0].ID != 1 {
		t.Fatalf("books = %+v, want only active book 1", books)
	}
}

func TestAddBookToMagicShelfIsRejected(t *testing.T) {
	setupShelfHandlerTestDB(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/shelves/2/books", strings.NewReader(`{"book_id":1}`))
	req = requestWithShelfID(req, "2")
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1}))

	addBookToShelfHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var count int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM book_shelf WHERE shelf_id = 2`).Scan(&count); err != nil {
		t.Fatalf("query magic shelf links: %v", err)
	}
	if count != 0 {
		t.Fatalf("magic shelf links = %d, want 0", count)
	}
}

func TestBulkAddToMagicShelfIsRejected(t *testing.T) {
	setupShelfHandlerTestDB(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/shelves/2/books/bulk", strings.NewReader(`{"book_ids":[1]}`))
	req = requestWithShelfID(req, "2")
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1}))

	bulkAddToShelfHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var count int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM book_shelf WHERE shelf_id = 2`).Scan(&count); err != nil {
		t.Fatalf("query magic shelf links: %v", err)
	}
	if count != 0 {
		t.Fatalf("magic shelf links = %d, want 0", count)
	}
}

func TestBulkAddByFilterToMagicShelfIsRejected(t *testing.T) {
	setupShelfHandlerTestDB(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/shelves/2/books/bulk-by-filter", strings.NewReader(`{"status":["reading"]}`))
	req = requestWithShelfID(req, "2")
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1}))

	bulkAddToShelfByFilterHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var count int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM book_shelf WHERE shelf_id = 2`).Scan(&count); err != nil {
		t.Fatalf("query magic shelf links: %v", err)
	}
	if count != 0 {
		t.Fatalf("magic shelf links = %d, want 0", count)
	}
}

func TestGetMagicShelfBooksUsesShelfSort(t *testing.T) {
	setupShelfHandlerTestDB(t)
	insertShelfTestBook(t, 4, 1, 1, "Newest Reading", "reading", false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/shelves/2/books", nil)
	req = requestWithShelfID(req, "2")
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1}))

	getShelfBooksHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var books []struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&books); err != nil {
		t.Fatalf("decode shelf books: %v", err)
	}
	if len(books) != 2 {
		t.Fatalf("books = %+v, want 2 matching books", books)
	}
	if books[0].ID != 4 || books[1].ID != 1 {
		t.Fatalf("book order = %+v, want newest added first", books)
	}
}

func TestUpdateShelfOrderPersistsOwnedShelves(t *testing.T) {
	setupShelfHandlerTestDB(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/shelves/order", strings.NewReader(`{"shelf_ids":[2,1]}`))
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{
		ID:          1,
		Permissions: []string{PermissionManageLibraries},
	}))

	updateShelfOrderHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var first, second int64
	if err := appDB.QueryRow(`SELECT sort_order FROM shelf WHERE id = 2`).Scan(&first); err != nil {
		t.Fatalf("query first shelf order: %v", err)
	}
	if err := appDB.QueryRow(`SELECT sort_order FROM shelf WHERE id = 1`).Scan(&second); err != nil {
		t.Fatalf("query second shelf order: %v", err)
	}
	if first != 1 || second != 2 {
		t.Fatalf("orders = shelf2:%d shelf1:%d, want 1 and 2", first, second)
	}
}

func requestWithShelfID(req *http.Request, shelfID string) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("shelfID", shelfID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}
