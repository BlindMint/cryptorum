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

func setupMetadataUpdateTestDB(t *testing.T) {
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
		VALUES (1, 'Main', 1)
	`)
	mustExec(t, `
		INSERT INTO book (id, library_id, added_at, last_scanned, owner_user_id)
		VALUES (1, 1, 100, 100, 1)
	`)
}

func TestUpdateBookHandlerDeduplicatesGenresAndTags(t *testing.T) {
	setupMetadataUpdateTestDB(t)

	body := `{
		"title": "Updated",
		"authors": ["Author"],
		"series_number": "",
		"status": "reading",
		"genres": ["Warhammer", " warhammer ", "Science Fiction"],
		"tags": ["Favorite", "favorite", "To Read"]
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/books/1", strings.NewReader(body))
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true}))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("bookID", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()

	updateBookHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Status string     `json:"status"`
		Book   BookDetail `json:"book"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "ok" {
		t.Fatalf("status = %q, want ok", response.Status)
	}

	var genres []string
	if err := json.Unmarshal([]byte(response.Book.Genres), &genres); err != nil {
		t.Fatalf("decode genres: %v", err)
	}
	if !sameStringSet(genres, []string{"Warhammer", "Science Fiction"}) || len(genres) != 2 {
		t.Fatalf("genres = %#v, want deduplicated values", genres)
	}

	var tags []string
	if err := json.Unmarshal([]byte(response.Book.Tags), &tags); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	if !sameStringSet(tags, []string{"Favorite", "To Read"}) || len(tags) != 2 {
		t.Fatalf("tags = %#v, want deduplicated values", tags)
	}
}
