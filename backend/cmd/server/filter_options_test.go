package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cryptorum/internal/db"
)

func setupFilterOptionsTestDB(t *testing.T) {
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
	insertFilterOptionBook(t, 1, 1, "Offensive", `["Will Crudge"]`, `["Military"]`, `["Starfleet"]`, "epub", "unread")
	insertFilterOptionBook(t, 2, 1, "Other Offensive", `["Other Author"]`, `["Military"]`, `["Other"]`, "pdf", "reading")
	insertFilterOptionBook(t, 3, 1, "Quiet Gardening", `["Garden Writer"]`, `["Gardening"]`, `["Plants"]`, "epub", "finished")
	insertFilterOptionBook(t, 4, 2, "Offensive Elsewhere", `["Outside Author"]`, `["Military"]`, `["Elsewhere"]`, "epub", "unread")
}

func insertFilterOptionBook(t *testing.T, bookID, libraryID int64, title, authors, genres, tags, format, status string) {
	t.Helper()
	mustExec(t, `
		INSERT INTO book (id, library_id, added_at, last_scanned, owner_user_id)
		VALUES (?, ?, ?, ?, 1)
	`, bookID, libraryID, bookID*100, bookID*100)
	mustExec(t, `
		INSERT INTO book_file (id, book_id, path, format, size, hash, last_modified, owner_user_id)
		VALUES (?, ?, ?, ?, 123, ?, ?, 1)
	`, bookID, bookID, title+"."+format, format, title+"-hash", bookID*100)
	mustExec(t, `
		INSERT INTO book_metadata (book_id, title, authors, genres, tags, owner_user_id)
		VALUES (?, ?, ?, ?, ?, 1)
	`, bookID, title, authors, genres, tags)
	mustExec(t, `
		INSERT INTO reading_progress (book_id, percent, status, updated_at, owner_user_id)
		VALUES (?, 0, ?, ?, 1)
	`, bookID, status, bookID*100)
}

func fetchFilterOptionsForTest(t *testing.T, path string) map[string][]metadataOption {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	getFilterOptionsHandler(rec, req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true})))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response map[string][]metadataOption
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode filter options: %v", err)
	}
	return response
}

func TestFilterOptionsScopeToSearchAndLibrary(t *testing.T) {
	setupFilterOptionsTestDB(t)

	options := fetchFilterOptionsForTest(t, "/api/filter-options?library_id=1&q=offensive")
	authors := optionNames(options["authors"])
	want := []string{"Other Author", "Will Crudge"}
	if !sameStringSet(authors, want) {
		t.Fatalf("authors = %#v, want %#v", authors, want)
	}
}

func TestFilterOptionsORFiltersStayInsideSearchScope(t *testing.T) {
	setupFilterOptionsTestDB(t)

	options := fetchFilterOptionsForTest(t, "/api/filter-options?q=offensive&filter_mode=OR&tags=Plants")
	authors := optionNames(options["authors"])
	if containsString(authors, "Garden Writer") {
		t.Fatalf("OR filter escaped search scope; authors = %#v", authors)
	}
}

func TestFilterOptionsTagsIncludeLegacyGenres(t *testing.T) {
	setupFilterOptionsTestDB(t)

	options := fetchFilterOptionsForTest(t, "/api/filter-options?library_id=1")
	tags := optionNames(options["tags"])
	want := []string{"Gardening", "Military", "Other", "Plants", "Starfleet"}
	if !sameStringSet(tags, want) {
		t.Fatalf("tags = %#v, want combined tags and legacy genres %#v", tags, want)
	}
}

func TestFilterOptionsBaseScopeCanIgnoreActiveFilters(t *testing.T) {
	setupFilterOptionsTestDB(t)

	filteredOptions := fetchFilterOptionsForTest(t, "/api/filter-options?q=offensive&author=Will%20Crudge")
	filteredAuthors := optionNames(filteredOptions["authors"])
	if !sameStringSet(filteredAuthors, []string{"Will Crudge"}) {
		t.Fatalf("filtered endpoint authors = %#v, want only selected author", filteredAuthors)
	}

	baseOptions := fetchFilterOptionsForTest(t, "/api/filter-options?q=offensive")
	baseAuthors := optionNames(baseOptions["authors"])
	if !sameStringSet(baseAuthors, []string{"Outside Author", "Other Author", "Will Crudge"}) {
		t.Fatalf("base-scope authors = %#v", baseAuthors)
	}
}

func TestBookFiltersDefaultToORWithinCategoriesAndANDAcrossCategories(t *testing.T) {
	setupFilterOptionsTestDB(t)

	books := fetchBooksForTest(t, "/api/books?q=offensive&author=Will%20Crudge&author=Other%20Author&genre=Military")
	titles := bookTitles(books)
	want := []string{"Offensive", "Other Offensive"}
	if !sameStringSet(titles, want) {
		t.Fatalf("titles = %#v, want %#v", titles, want)
	}
}

func TestTagFilterMatchesLegacyGenres(t *testing.T) {
	setupFilterOptionsTestDB(t)

	books := fetchBooksForTest(t, "/api/books?q=offensive&tags=Military")
	titles := bookTitles(books)
	want := []string{"Offensive", "Other Offensive", "Offensive Elsewhere"}
	if !sameStringSet(titles, want) {
		t.Fatalf("titles = %#v, want %#v", titles, want)
	}
}

func TestBookFiltersCanRequireAllValuesWithinCategory(t *testing.T) {
	setupFilterOptionsTestDB(t)

	books := fetchBooksForTest(t, "/api/books?q=offensive&author=Will%20Crudge&author=Other%20Author&value_filter_mode=AND")
	if len(books) != 0 {
		t.Fatalf("books = %#v, want none", books)
	}
}

func TestBookFiltersAcrossORStaysInsideSearchScope(t *testing.T) {
	setupFilterOptionsTestDB(t)

	books := fetchBooksForTest(t, "/api/books?q=offensive&author=Will%20Crudge&genre=Gardening&filter_mode=OR")
	titles := bookTitles(books)
	if !sameStringSet(titles, []string{"Offensive"}) {
		t.Fatalf("titles = %#v, want only Offensive", titles)
	}
}

func fetchBooksForTest(t *testing.T, path string) []struct {
	Title string `json:"title"`
} {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	getBooksHandler(rec, req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true})))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Books []struct {
			Title string `json:"title"`
		} `json:"books"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode books: %v", err)
	}
	return response.Books
}

func bookTitles(books []struct {
	Title string `json:"title"`
}) []string {
	titles := make([]string, 0, len(books))
	for _, book := range books {
		titles = append(titles, book.Title)
	}
	return titles
}

func optionNames(options []metadataOption) []string {
	names := make([]string, 0, len(options))
	for _, option := range options {
		names = append(names, option.Name)
	}
	return names
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]int, len(left))
	for _, value := range left {
		seen[value]++
	}
	for _, value := range right {
		seen[value]--
		if seen[value] < 0 {
			return false
		}
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
