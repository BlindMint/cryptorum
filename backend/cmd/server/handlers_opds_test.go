package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"cryptorum/internal/auth"
	"cryptorum/internal/config"
	"cryptorum/internal/db"
)

func setupOPDSTest(t *testing.T) (*chi.Mux, string) {
	t.Helper()
	dataPath := t.TempDir()
	testDB, err := db.New(dataPath)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	passwordHash, err := auth.HashPassword("library-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	previousDB, previousConfig, previousStore := appDB, appConfig, sessionStore
	appDB = testDB
	appConfig = &config.Config{
		Server: config.ServerConfig{DataPath: dataPath},
		Auth: config.AuthConfig{
			Mode: "password", Username: "reader", PasswordHash: passwordHash, SessionDuration: time.Hour,
		},
	}
	sessionStore = auth.NewStore(testDB.DB, time.Hour)
	maintenanceMode.Store(false)
	loginThrottle.Lock()
	loginThrottle.entries = make(map[string]loginThrottleEntry)
	loginThrottle.Unlock()

	now := time.Now().Unix()
	mustExec(t, `INSERT INTO app_user (id, username, password_hash, is_admin, is_bootstrap_admin, permissions_json, created_at, updated_at) VALUES (1, 'reader', ?, 1, 1, '[]', ?, ?)`, passwordHash, now, now)
	mustExec(t, `INSERT INTO library (id, name, owner_user_id) VALUES (1, 'Fiction', 1)`)

	epubPath := filepath.Join(dataPath, "alpha.epub")
	if err := os.WriteFile(epubPath, []byte("epub payload"), 0o600); err != nil {
		t.Fatalf("write test book: %v", err)
	}
	mustExec(t, `INSERT INTO book (id, library_id, added_at, last_scanned, owner_user_id) VALUES (1, 1, ?, ?, 1), (2, 1, ?, ?, 1)`, now-20, now-10, now-40, now-30)
	mustExec(t, `INSERT INTO book_metadata (book_id, title, authors, series, series_number, series_number_display, publisher, pub_date, description, genres, tags, page_count, language, owner_user_id) VALUES (1, 'Alpha Book', '["Jane Writer"]', 'Example Series', 2, '2', 'Example Press', '2024-01-02', 'A test publication.', '["Fiction"]', '["Favorite"]', 321, 'en', 1), (2, 'Missing Book', '[]', '', 0, '', '', '', '', '[]', '[]', 0, '', 1)`)
	mustExec(t, `INSERT INTO book_file (id, book_id, path, format, size, hash, last_modified, missing_at, owner_user_id) VALUES (1, 1, ?, 'epub', 12, 'epub-hash', ?, NULL, 1), (2, 1, ?, 'pdf', 34, 'pdf-hash', ?, NULL, 1), (3, 1, ?, 'mp3', 56, 'missing-hash', ?, ?, 1), (4, 2, ?, 'epub', 78, 'gone-hash', ?, ?, 1)`, epubPath, now, filepath.Join(dataPath, "alpha.pdf"), now, filepath.Join(dataPath, "alpha.mp3"), now, now, filepath.Join(dataPath, "missing.epub"), now, now)
	mustExec(t, `INSERT INTO shelf (id, name, is_magic, rules_json, sort_by, sort_dir, owner_user_id, sort_order) VALUES (1, 'Favorites', 0, '', 'name', 'asc', 1, 1)`)
	mustExec(t, `INSERT INTO book_shelf (book_id, shelf_id) VALUES (1, 1)`)

	router := chi.NewRouter()
	initRoutes(router)
	t.Cleanup(func() {
		appDB, appConfig, sessionStore = previousDB, previousConfig, previousStore
		_ = testDB.Close()
	})
	return router, epubPath
}

func opdsRequest(method, target string, body *strings.Reader) *http.Request {
	var request *http.Request
	if body == nil {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, body)
	}
	request.SetBasicAuth("reader", "library-password")
	return request
}

func TestOPDSAuthenticationAndRootDiscovery(t *testing.T) {
	router, _ := setupOPDSTest(t)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/opds/", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401: %s", unauthorized.Code, unauthorized.Body.String())
	}
	if !strings.HasPrefix(unauthorized.Header().Get("Content-Type"), opdsAuthenticationType) || unauthorized.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("missing OPDS authentication response headers: %+v", unauthorized.Header())
	}

	authDocument := httptest.NewRecorder()
	router.ServeHTTP(authDocument, httptest.NewRequest(http.MethodGet, "/opds/authentication", nil))
	if authDocument.Code != http.StatusOK {
		t.Fatalf("authentication document status = %d: %s", authDocument.Code, authDocument.Body.String())
	}

	root := httptest.NewRecorder()
	router.ServeHTTP(root, opdsRequest(http.MethodGet, "/opds/", nil))
	if root.Code != http.StatusOK {
		t.Fatalf("root status = %d: %s", root.Code, root.Body.String())
	}
	if !strings.HasPrefix(root.Header().Get("Content-Type"), opdsMediaType) {
		t.Fatalf("root content type = %q", root.Header().Get("Content-Type"))
	}
	var feed opdsFeed
	if err := json.NewDecoder(root.Body).Decode(&feed); err != nil {
		t.Fatalf("decode root: %v", err)
	}
	if feed.Metadata.Title != "Cryptorum Catalog" || len(feed.Navigation) != 6 {
		t.Fatalf("unexpected root feed: %+v", feed)
	}
	if feed.Links[2].Rel != "search" || !feed.Links[2].Templated {
		t.Fatalf("search discovery link missing: %+v", feed.Links)
	}
}

func TestOPDSPublicationsExposeAllActiveFormats(t *testing.T) {
	router, _ := setupOPDSTest(t)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, opdsRequest(http.MethodGet, "/opds/publications", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("publications status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var feed opdsFeed
	if err := json.NewDecoder(recorder.Body).Decode(&feed); err != nil {
		t.Fatalf("decode publications: %v", err)
	}
	if len(feed.Publications) != 1 {
		t.Fatalf("publications = %d, want only active book: %+v", len(feed.Publications), feed.Publications)
	}
	publication := feed.Publications[0]
	if publication.Metadata.Title != "Alpha Book" || len(publication.Metadata.Author) != 1 || publication.Metadata.NumberOfPages != 321 {
		t.Fatalf("unexpected publication metadata: %+v", publication.Metadata)
	}
	if len(publication.Links) != 3 {
		t.Fatalf("links = %+v, want self plus two active acquisitions", publication.Links)
	}
	if publication.Links[1].Rel != opdsAcquisitionRel || publication.Links[1].Type != "application/epub+zip" || publication.Links[2].Type != "application/pdf" {
		t.Fatalf("unexpected acquisition links: %+v", publication.Links)
	}
}

func TestOPDSSearchCategoriesAndLegacyDownload(t *testing.T) {
	router, _ := setupOPDSTest(t)
	for _, target := range []string{
		"/opds/search?query=Alpha", "/opds/publications?author=Jane+Writer", "/opds/publications?series=Example+Series", "/opds/publications?shelf=1",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, opdsRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d: %s", target, recorder.Code, recorder.Body.String())
		}
		var feed opdsFeed
		if err := json.NewDecoder(recorder.Body).Decode(&feed); err != nil || len(feed.Publications) != 1 {
			t.Fatalf("%s returned unexpected feed: err=%v feed=%+v", target, err, feed)
		}
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, opdsRequest(http.MethodGet, "/opds/1/download", nil))
	if download.Code != http.StatusOK || download.Body.String() != "epub payload" {
		t.Fatalf("legacy download status/body = %d/%q", download.Code, download.Body.String())
	}
}

func TestOPDSSettingsCanDisableCatalog(t *testing.T) {
	router, _ := setupOPDSTest(t)
	user := &AppUser{ID: 1, IsAdmin: true}

	getRecorder := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/api/settings/opds", nil)
	getOPDSSettingsHandler(getRecorder, getRequest.WithContext(authContextWithUser(getRequest.Context(), user)))
	var defaults OPDSSettingsResponse
	if err := json.NewDecoder(getRecorder.Body).Decode(&defaults); err != nil || !defaults.Enabled || defaults.CatalogPath != "/opds/" {
		t.Fatalf("unexpected default settings: err=%v settings=%+v", err, defaults)
	}

	updateRecorder := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/settings/opds", strings.NewReader(`{"enabled":false,"catalog_title":"My Library"}`))
	updateRequest = updateRequest.WithContext(authContextWithUser(updateRequest.Context(), user))
	updateOPDSSettingsHandler(updateRecorder, updateRequest)
	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("update settings status = %d: %s", updateRecorder.Code, updateRecorder.Body.String())
	}

	disabled := httptest.NewRecorder()
	router.ServeHTTP(disabled, opdsRequest(http.MethodGet, "/opds/", nil))
	if disabled.Code != http.StatusNotFound {
		t.Fatalf("disabled catalog status = %d, want 404", disabled.Code)
	}
	authDocument := httptest.NewRecorder()
	router.ServeHTTP(authDocument, httptest.NewRequest(http.MethodGet, "/opds/authentication", nil))
	if authDocument.Code != http.StatusNotFound {
		t.Fatalf("disabled authentication document status = %d, want 404", authDocument.Code)
	}
}
