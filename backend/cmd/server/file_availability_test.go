package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFileAvailability(t *testing.T) {
	dir := t.TempDir()
	readable := filepath.Join(dir, "book.pdf")
	empty := filepath.Join(dir, "empty.pdf")
	if err := os.WriteFile(readable, []byte("book"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, code string
		status           int
	}{
		{"available", readable, "available", 200},
		{"missing", filepath.Join(dir, "gone.pdf"), "file_missing", 404},
		{"directory", dir, "file_unreadable", 403},
		{"empty", empty, "file_empty", 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := checkFileAvailability(tc.path)
			if got.Code != tc.code {
				t.Fatalf("got %v, want %s", got, tc.code)
			}
			w := httptest.NewRecorder()
			if requireAvailableFile(w, tc.path) != (tc.code == "available") {
				t.Fatal("incorrect access decision")
			}
			if w.Code != tc.status {
				t.Fatalf("status %d want %d", w.Code, tc.status)
			}
		})
	}
	if got := availabilityForError(os.ErrPermission); got.Code != "file_unreadable" {
		t.Fatal(got)
	}
}

func TestMissingBookFileResponsesPreserveRecords(t *testing.T) {
	setupCombineTestDB(t)
	insertCombineTestBook(t, 10, 1, "Missing", filepath.Join(t.TempDir(), "gone.pdf"), "pdf")
	mustExec(t, `INSERT INTO reading_progress (book_id, file_id, percent, status, updated_at, owner_user_id) VALUES (10,10,42,'reading',100,1)`)
	request := func(handler http.HandlerFunc) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/books/10/file?file_id=10", nil)
		req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true}))
		route := chi.NewRouteContext()
		route.URLParams.Add("bookID", "10")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec
	}
	rec := request(ServeBookFileHandler)
	if rec.Code != 404 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var failure map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure["code"] != "file_missing" {
		t.Fatal(failure)
	}
	rec = request(getBookFilesHandler)
	var files []struct {
		Availability fileAvailability `json:"availability"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Availability.Code != "file_missing" {
		t.Fatalf("files: %+v", files)
	}
	var missing sql.NullInt64
	var percent float64
	if err := appDB.QueryRow(`SELECT bf.missing_at,rp.percent FROM book_file bf JOIN reading_progress rp ON rp.file_id=bf.id WHERE bf.id=10`).Scan(&missing, &percent); err != nil {
		t.Fatal(err)
	}
	if missing.Valid || percent != 42 {
		t.Fatalf("availability check changed scan/progress state: %v %v", missing, percent)
	}
}
