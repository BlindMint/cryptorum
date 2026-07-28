package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"cryptorum/internal/db"
	"cryptorum/internal/metaprotection"

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
		"title": " Updated ",
		"authors": ["Author", " author ", "Second Author"],
		"series": " Series Name ",
		"series_number": "",
		"publisher": " Publisher ",
		"pub_date": " 2024 ",
		"description": " Description ",
		"status": "reading",
		"genres": ["Warhammer", " warhammer ", "Science Fiction"],
		"tags": ["zeta", " Alpha ", "beta", "alpha"],
		"isbn": " 9781234567890 ",
		"asin": " B000123 ",
		"language": " en "
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
	if response.Book.Title != "Updated" {
		t.Fatalf("title = %q, want trimmed title", response.Book.Title)
	}
	if response.Book.Series != "Series Name" {
		t.Fatalf("series = %q, want trimmed series", response.Book.Series)
	}
	if response.Book.Publisher != "Publisher" {
		t.Fatalf("publisher = %q, want trimmed publisher", response.Book.Publisher)
	}
	if response.Book.PubDate != "2024" {
		t.Fatalf("pub_date = %q, want trimmed pub_date", response.Book.PubDate)
	}
	if response.Book.Description != "Description" {
		t.Fatalf("description = %q, want trimmed description", response.Book.Description)
	}
	if response.Book.ISBN != "9781234567890" {
		t.Fatalf("isbn = %q, want trimmed isbn", response.Book.ISBN)
	}
	if response.Book.ASIN != "B000123" {
		t.Fatalf("asin = %q, want trimmed asin", response.Book.ASIN)
	}
	if response.Book.Language != "en" {
		t.Fatalf("language = %q, want trimmed language", response.Book.Language)
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
	if len(genres) != 0 {
		t.Fatalf("genres = %#v, want legacy genres cleared", genres)
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
	var rawTags string
	if err := appDB.QueryRow(`
		SELECT COALESCE(authors, '[]'), COALESCE(genres, '[]'), COALESCE(tags, '[]')
		FROM book_metadata
		WHERE book_id = 1
	`).Scan(&rawAuthors, &rawGenres, &rawTags); err != nil {
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
	if len(genres) != 0 {
		t.Fatalf("genres = %#v, want legacy genres empty", genres)
	}

	var tags []string
	if err := json.Unmarshal([]byte(rawTags), &tags); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	wantTags := []string{"Science Fiction", "Warhammer"}
	if !reflect.DeepEqual(tags, wantTags) {
		t.Fatalf("tags = %#v, want provider categories mapped to tags %#v", tags, wantTags)
	}
}

func TestUserEditsAndProviderUpdatesRespectMetadataProtection(t *testing.T) {
	setupMetadataUpdateTestDB(t)
	mustExec(t, `
		INSERT INTO book_metadata (
			book_id, title, authors, publisher, genres, tags, locked_fields, owner_user_id
		) VALUES (1, 'Original', '[]', 'Original Publisher', '[]', '[]', '[]', 1)
	`)

	body := `{
		"title": "User Title",
		"authors": [],
		"series": "",
		"series_number": "",
		"publisher": "Original Publisher",
		"pub_date": "",
		"description": "",
		"rating": 0,
		"status": "unread",
		"genres": [],
		"tags": [],
		"isbn": "",
		"asin": "",
		"language": "",
		"page_count": 0
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/books/1", strings.NewReader(body))
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true}))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("bookID", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	updateBookHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update metadata status = %d: %s", rec.Code, rec.Body.String())
	}

	var title, publisher, lockedFields string
	if err := appDB.QueryRow(`
		SELECT title, publisher, locked_fields FROM book_metadata WHERE book_id = 1
	`).Scan(&title, &publisher, &lockedFields); err != nil {
		t.Fatalf("load protected metadata: %v", err)
	}
	if title != "User Title" || !strings.Contains(lockedFields, `"title"`) {
		t.Fatalf("title=%q locked=%s, want user title protected", title, lockedFields)
	}
	if strings.Contains(lockedFields, `"publisher"`) {
		t.Fatalf("unchanged publisher was unexpectedly protected: %s", lockedFields)
	}

	candidate := MetadataCandidate{Title: "Provider Title", Publisher: "Provider Publisher"}
	if err := applyMetadataCandidateToBookWithOptions(1, candidate, false, false, 1); err != nil {
		t.Fatalf("apply protected provider metadata: %v", err)
	}
	if err := appDB.QueryRow(`
		SELECT title, publisher, locked_fields FROM book_metadata WHERE book_id = 1
	`).Scan(&title, &publisher, &lockedFields); err != nil {
		t.Fatalf("reload provider metadata: %v", err)
	}
	if title != "User Title" {
		t.Fatalf("protected title = %q, want User Title", title)
	}
	if publisher != "Provider Publisher" || !strings.Contains(lockedFields, `"publisher"`) {
		t.Fatalf("publisher=%q locked=%s, want applied provider value protected", publisher, lockedFields)
	}

	if err := applyMetadataCandidateToBookWithOptions(1, MetadataCandidate{Title: "Explicit Override"}, false, true, 1); err != nil {
		t.Fatalf("override protected provider metadata: %v", err)
	}
	if err := appDB.QueryRow(`SELECT title FROM book_metadata WHERE book_id = 1`).Scan(&title); err != nil {
		t.Fatalf("load overridden title: %v", err)
	}
	if title != "Explicit Override" {
		t.Fatalf("overridden title = %q, want Explicit Override", title)
	}

	var revisions int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM book_metadata_revision WHERE book_id = 1`).Scan(&revisions); err != nil {
		t.Fatalf("count metadata revisions: %v", err)
	}
	if revisions < 3 {
		t.Fatalf("metadata revisions = %d, want at least 3", revisions)
	}
}

func TestLibraryMetadataProtectionBlocksProviderUpdatesUnlessOverridden(t *testing.T) {
	setupMetadataUpdateTestDB(t)
	mustExec(t, `UPDATE library SET metadata_protection_enabled = 1 WHERE id = 1`)
	mustExec(t, `
		INSERT INTO book_metadata (
			book_id, title, authors, publisher, genres, tags, locked_fields, owner_user_id
		) VALUES (1, 'Current Title', '[]', 'Current Publisher', '[]', '[]', '[]', 1)
	`)

	candidate := MetadataCandidate{
		Title:     "Provider Title",
		Publisher: "Provider Publisher",
	}
	if err := applyMetadataCandidateToBookWithOptions(1, candidate, false, false, 1); err != nil {
		t.Fatalf("apply protected provider metadata: %v", err)
	}
	var title, publisher string
	if err := appDB.QueryRow(`
		SELECT title, publisher FROM book_metadata WHERE book_id = 1
	`).Scan(&title, &publisher); err != nil {
		t.Fatalf("load protected provider metadata: %v", err)
	}
	if title != "Current Title" || publisher != "Current Publisher" {
		t.Fatalf("protected provider metadata title=%q publisher=%q", title, publisher)
	}

	if err := applyMetadataCandidateToBookWithOptions(1, candidate, false, true, 1); err != nil {
		t.Fatalf("override library metadata protection: %v", err)
	}
	if err := appDB.QueryRow(`
		SELECT title, publisher FROM book_metadata WHERE book_id = 1
	`).Scan(&title, &publisher); err != nil {
		t.Fatalf("load overridden provider metadata: %v", err)
	}
	if title != "Provider Title" || publisher != "Provider Publisher" {
		t.Fatalf("overridden provider metadata title=%q publisher=%q", title, publisher)
	}
}

func TestLibraryMetadataProtectionHandlersUpdateOneOrAllLibraries(t *testing.T) {
	setupMetadataUpdateTestDB(t)
	mustExec(t, `INSERT INTO library (id, name, owner_user_id) VALUES (2, 'Other', 1)`)

	globalReq := httptest.NewRequest(
		http.MethodPut,
		"/api/libraries/metadata-protection",
		strings.NewReader(`{"enabled":true}`),
	)
	globalReq = globalReq.WithContext(authContextWithUser(
		globalReq.Context(),
		&AppUser{ID: 1, IsAdmin: true},
	))
	globalRec := httptest.NewRecorder()
	updateAllLibrariesMetadataProtectionHandler(globalRec, globalReq)
	if globalRec.Code != http.StatusOK {
		t.Fatalf("global protection status = %d: %s", globalRec.Code, globalRec.Body.String())
	}

	var enabledCount int
	if err := appDB.QueryRow(`
		SELECT COUNT(*) FROM library WHERE metadata_protection_enabled = 1
	`).Scan(&enabledCount); err != nil {
		t.Fatalf("count protected libraries: %v", err)
	}
	if enabledCount != 2 {
		t.Fatalf("protected libraries = %d, want 2", enabledCount)
	}

	libraryReq := httptest.NewRequest(
		http.MethodPut,
		"/api/libraries/1/metadata-protection",
		strings.NewReader(`{"enabled":false}`),
	)
	libraryReq = libraryReq.WithContext(authContextWithUser(
		libraryReq.Context(),
		&AppUser{ID: 1, IsAdmin: true},
	))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("libraryID", "1")
	libraryReq = libraryReq.WithContext(context.WithValue(
		libraryReq.Context(),
		chi.RouteCtxKey,
		routeCtx,
	))
	libraryRec := httptest.NewRecorder()
	updateLibraryMetadataProtectionHandler(libraryRec, libraryReq)
	if libraryRec.Code != http.StatusOK {
		t.Fatalf("library protection status = %d: %s", libraryRec.Code, libraryRec.Body.String())
	}

	var firstEnabled, secondEnabled int
	if err := appDB.QueryRow(`
		SELECT
			(SELECT metadata_protection_enabled FROM library WHERE id = 1),
			(SELECT metadata_protection_enabled FROM library WHERE id = 2)
	`).Scan(&firstEnabled, &secondEnabled); err != nil {
		t.Fatalf("load library protection states: %v", err)
	}
	if firstEnabled != 0 || secondEnabled != 1 {
		t.Fatalf("library protection states = (%d, %d), want (0, 1)", firstEnabled, secondEnabled)
	}
}

func TestBulkMetadataUpdateProtectsTouchedAndClearedFields(t *testing.T) {
	setupMetadataUpdateTestDB(t)
	mustExec(t, `
		INSERT INTO book_metadata (
			book_id, title, authors, publisher, language, genres, tags, locked_fields, owner_user_id
		) VALUES (1, 'Book', '[]', 'Old Publisher', 'en', '[]', '["Keep"]', '[]', 1)
	`)
	publisher := "Bulk Publisher"
	req := bulkMetadataRequest{
		Publisher:   &publisher,
		ClearFields: []string{"language"},
		AddTags:     []string{"Bulk"},
	}
	if err := applyBulkMetadataUpdate(1, 1, req); err != nil {
		t.Fatalf("apply bulk metadata: %v", err)
	}

	var gotPublisher, language, tags, lockedFields string
	if err := appDB.QueryRow(`
		SELECT publisher, language, tags, locked_fields
		FROM book_metadata WHERE book_id = 1
	`).Scan(&gotPublisher, &language, &tags, &lockedFields); err != nil {
		t.Fatalf("load bulk metadata: %v", err)
	}
	if gotPublisher != "Bulk Publisher" || language != "" {
		t.Fatalf("publisher=%q language=%q", gotPublisher, language)
	}
	for _, field := range []string{"publisher", "language", "tags"} {
		if !strings.Contains(lockedFields, `"`+field+`"`) {
			t.Fatalf("bulk field %q not protected: %s", field, lockedFields)
		}
	}
	if !strings.Contains(tags, "Bulk") || !strings.Contains(tags, "Keep") {
		t.Fatalf("bulk tags = %s, want existing and added tags", tags)
	}

	var revisions int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM book_metadata_revision WHERE book_id = 1`).Scan(&revisions); err != nil {
		t.Fatalf("count bulk metadata revisions: %v", err)
	}
	if revisions != 1 {
		t.Fatalf("bulk metadata revisions = %d, want 1", revisions)
	}
}

func TestRestoreMetadataRevisionRestoresChangedFieldsAndProtection(t *testing.T) {
	setupMetadataUpdateTestDB(t)
	mustExec(t, `
		INSERT INTO book_metadata (
			book_id, title, authors, genres, tags, locked_fields, owner_user_id
		) VALUES (1, 'Before', '[]', '[]', '[]', '[]', 1)
	`)
	before, exists, err := metaprotection.LoadSnapshot(appDB.DB, 1)
	if err != nil || !exists {
		t.Fatalf("load initial metadata: exists=%v err=%v", exists, err)
	}
	mustExec(t, `UPDATE book_metadata SET title = 'After', locked_fields = '["title"]' WHERE book_id = 1`)
	if err := metaprotection.RecordRevision(appDB.DB, before, []string{"title"}, "manual_edit", 1); err != nil {
		t.Fatalf("record revision: %v", err)
	}
	var revisionID int64
	if err := appDB.QueryRow(`SELECT id FROM book_metadata_revision WHERE book_id = 1`).Scan(&revisionID); err != nil {
		t.Fatalf("load revision id: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/1/metadata/revisions/1/restore", nil)
	req = req.WithContext(authContextWithUser(req.Context(), &AppUser{ID: 1, IsAdmin: true}))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("bookID", "1")
	routeCtx.URLParams.Add("revisionID", strconv.FormatInt(revisionID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	RestoreBookMetadataRevisionHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore revision status = %d: %s", rec.Code, rec.Body.String())
	}

	var title, lockedFields string
	if err := appDB.QueryRow(`SELECT title, locked_fields FROM book_metadata WHERE book_id = 1`).Scan(&title, &lockedFields); err != nil {
		t.Fatalf("load restored metadata: %v", err)
	}
	if title != "Before" || lockedFields != "[]" {
		t.Fatalf("restored title=%q locked=%s, want Before and []", title, lockedFields)
	}
}

func TestFetchBookDetailMergesLegacyGenresIntoTags(t *testing.T) {
	setupMetadataUpdateTestDB(t)
	mustExec(t, `
		INSERT INTO book_metadata (book_id, title, genres, tags, owner_user_id)
		VALUES (1, 'Legacy', '["Military","Space.Opera"]', '["Favorite","military"]', 1)
	`)

	book, err := fetchBookDetail(1, &AppUser{ID: 1, IsAdmin: true})
	if err != nil {
		t.Fatalf("fetch book detail: %v", err)
	}

	var tags []string
	if err := json.Unmarshal([]byte(book.Tags), &tags); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	want := []string{"Favorite", "Military", "Space.Opera"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("tags = %#v, want legacy genres merged into tags %#v", tags, want)
	}
}
