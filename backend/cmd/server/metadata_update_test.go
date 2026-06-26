package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestUpdateBookHandlerDeduplicatesMetadataLists(t *testing.T) {
	setupMetadataUpdateTestDB(t)

	body := `{
		"title": "Updated",
		"authors": ["Author", " author ", "Second Author"],
		"series_number": "",
		"status": "reading",
		"genres": ["Warhammer", " warhammer ", "Science Fiction"],
		"tags": ["zeta", " Alpha ", "beta", "alpha"]
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

	var authors []string
	if err := json.Unmarshal([]byte(response.Book.Authors), &authors); err != nil {
		t.Fatalf("decode authors: %v", err)
	}
	if !sameStringSet(authors, []string{"Author", "Second Author"}) || len(authors) != 2 {
		t.Fatalf("authors = %#v, want deduplicated values", authors)
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
	wantTags := []string{"Alpha", "beta", "Science Fiction", "Warhammer", "zeta"}
	if !reflect.DeepEqual(tags, wantTags) {
		t.Fatalf("tags = %#v, want sorted deduplicated values %#v", tags, wantTags)
	}
}

func TestApplyMetadataCandidateToBookDeduplicatesMetadataLists(t *testing.T) {
	setupMetadataUpdateTestDB(t)

	candidate := MetadataCandidate{
		Title:   "Applied",
		Authors: []string{"Author", " author ", "Second Author"},
		Genres:  []string{"Warhammer", " warhammer ", "Science Fiction"},
	}

	if err := applyMetadataCandidateToBook(1, candidate, false); err != nil {
		t.Fatalf("apply metadata candidate: %v", err)
	}

	var rawAuthors string
	var rawGenres string
	if err := appDB.QueryRow(`
		SELECT COALESCE(authors, '[]'), COALESCE(genres, '[]')
		FROM book_metadata
		WHERE book_id = 1
	`).Scan(&rawAuthors, &rawGenres); err != nil {
		t.Fatalf("fetch metadata: %v", err)
	}

	var authors []string
	if err := json.Unmarshal([]byte(rawAuthors), &authors); err != nil {
		t.Fatalf("decode authors: %v", err)
	}
	if !sameStringSet(authors, []string{"Author", "Second Author"}) || len(authors) != 2 {
		t.Fatalf("authors = %#v, want deduplicated values", authors)
	}

	var genres []string
	if err := json.Unmarshal([]byte(rawGenres), &genres); err != nil {
		t.Fatalf("decode genres: %v", err)
	}
	if !sameStringSet(genres, []string{"Warhammer", "Science Fiction"}) || len(genres) != 2 {
		t.Fatalf("genres = %#v, want deduplicated values", genres)
	}
}
