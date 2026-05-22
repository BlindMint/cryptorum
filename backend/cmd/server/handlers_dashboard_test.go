package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cryptorum/internal/db"
)

func setupDashboardHandlerTestDB(t *testing.T) {
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
		INSERT INTO library (id, name, owner_user_id, exclude_from_suggestions)
		VALUES
			(1, 'Owner 1', 1, 0),
			(2, 'Owner 2', 2, 0),
			(3, 'Hidden', 1, 1)
	`)
	insertDashboardBook(t, 1, 1, 1, "Owner One Book", `["Owner One"]`, `["Fiction"]`, "reading", 100)
	insertDashboardBook(t, 2, 2, 2, "Owner Two Book", `["Owner Two"]`, `["History"]`, "finished", 220)
	insertDashboardBook(t, 3, 3, 1, "Hidden Book", `["Hidden Author"]`, `["Hidden Genre"]`, "reading", 330)
}

func mustExec(t *testing.T, query string, args ...interface{}) {
	t.Helper()
	if _, err := appDB.Exec(query, args...); err != nil {
		t.Fatalf("exec query: %v", err)
	}
}

func insertDashboardBook(t *testing.T, bookID, libraryID, ownerID int64, title, authors, genres, status string, addedAt int64) {
	t.Helper()
	mustExec(t, `
		INSERT INTO book (id, library_id, added_at, last_scanned, owner_user_id)
		VALUES (?, ?, ?, ?, ?)
	`, bookID, libraryID, addedAt, addedAt, ownerID)
	mustExec(t, `
		INSERT INTO book_file (id, book_id, path, format, size, hash, last_modified, owner_user_id)
		VALUES (?, ?, ?, 'epub', 123, ?, ?, ?)
	`, bookID, bookID, title+".epub", title+"-hash", addedAt, ownerID)
	mustExec(t, `
		INSERT INTO book_metadata (book_id, title, authors, genres, page_count, owner_user_id)
		VALUES (?, ?, ?, ?, 10, ?)
	`, bookID, title, authors, genres, ownerID)
	mustExec(t, `
		INSERT INTO reading_progress (book_id, percent, status, updated_at, owner_user_id)
		VALUES (?, 50, ?, ?, ?)
	`, bookID, status, addedAt, ownerID)
}

func requestWithUser(path string, user *AppUser) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	return req.WithContext(authContextWithUser(req.Context(), user))
}

func TestDashboardSummaryScopesToCurrentUser(t *testing.T) {
	setupDashboardHandlerTestDB(t)

	rec := httptest.NewRecorder()
	getDashboardSummaryHandler(rec, requestWithUser("/api/dashboard/summary", &AppUser{ID: 2}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var summary dashboardSummaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.TotalBooks != 1 || summary.Libraries != 1 || summary.Reading != 0 || summary.Finished != 1 {
		t.Fatalf("unexpected scoped summary: %+v", summary)
	}
}

func TestDashboardSummaryAdminSeesAllLibraries(t *testing.T) {
	setupDashboardHandlerTestDB(t)

	rec := httptest.NewRecorder()
	getDashboardSummaryHandler(rec, requestWithUser("/api/dashboard/summary", &AppUser{ID: 1, IsAdmin: true}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var summary dashboardSummaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.TotalBooks != 3 || summary.Libraries != 3 || summary.Reading != 2 || summary.Finished != 1 {
		t.Fatalf("unexpected admin summary: %+v", summary)
	}
}

func TestDiscoverBooksScopesAndExcludesHiddenLibraries(t *testing.T) {
	setupDashboardHandlerTestDB(t)

	rec := httptest.NewRecorder()
	getDiscoverBooksHandler(rec, requestWithUser("/api/books/discover?limit=10", &AppUser{ID: 1}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response dashboardBooksResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode discovery response: %v", err)
	}
	if len(response.Books) != 1 {
		t.Fatalf("expected one discoverable book, got %+v", response.Books)
	}
	if response.Books[0].ID != 1 {
		t.Fatalf("expected visible owner book, got %+v", response.Books[0])
	}
}

func TestStatsScopesToCurrentUser(t *testing.T) {
	setupDashboardHandlerTestDB(t)

	rec := httptest.NewRecorder()
	GetStatsHandler(rec, requestWithUser("/api/stats", &AppUser{ID: 2}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var stats map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats["total_books"].(float64) != 1 || stats["reading"].(float64) != 0 || stats["finished"].(float64) != 1 {
		t.Fatalf("unexpected scoped stats counts: %+v", stats)
	}
	if stats["total_authors"].(float64) != 1 || stats["total_genres"].(float64) != 1 {
		t.Fatalf("unexpected scoped metadata totals: %+v", stats)
	}
}
