package main

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"cryptorum/internal/auth"
)

const (
	opdsSettingsKey        = "opds_settings"
	opdsMediaType          = "application/opds+json"
	opdsPublicationType    = "application/opds-publication+json"
	opdsAuthenticationType = "application/opds-authentication+json"
	opdsAcquisitionRel     = "http://opds-spec.org/acquisition"
)

type opdsSettings struct {
	Enabled      bool   `json:"enabled"`
	CatalogTitle string `json:"catalog_title"`
}

type OPDSSettingsResponse struct {
	Enabled                bool   `json:"enabled"`
	CatalogTitle           string `json:"catalog_title"`
	CatalogPath            string `json:"catalog_path"`
	AuthenticationRequired bool   `json:"authentication_required"`
}

type opdsLink struct {
	Href       string         `json:"href"`
	Type       string         `json:"type,omitempty"`
	Rel        string         `json:"rel,omitempty"`
	Title      string         `json:"title,omitempty"`
	Templated  bool           `json:"templated,omitempty"`
	Width      int            `json:"width,omitempty"`
	Height     int            `json:"height,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

type opdsFeedMetadata struct {
	Title         string `json:"title"`
	Modified      string `json:"modified,omitempty"`
	NumberOfItems int    `json:"numberOfItems,omitempty"`
}

type opdsFeed struct {
	Metadata     opdsFeedMetadata  `json:"metadata"`
	Links        []opdsLink        `json:"links"`
	Navigation   []opdsLink        `json:"navigation,omitempty"`
	Publications []opdsPublication `json:"publications,omitempty"`
}

type opdsContributor struct {
	Name     string  `json:"name"`
	Position float64 `json:"position,omitempty"`
}

type opdsBelongsTo struct {
	Series []opdsContributor `json:"series,omitempty"`
}

type opdsSubject struct {
	Name string `json:"name"`
}

type opdsPublicationMetadata struct {
	Type          string            `json:"@type"`
	Identifier    string            `json:"identifier"`
	Title         string            `json:"title"`
	Author        []opdsContributor `json:"author,omitempty"`
	Publisher     []opdsContributor `json:"publisher,omitempty"`
	Language      []string          `json:"language,omitempty"`
	Modified      string            `json:"modified,omitempty"`
	Published     string            `json:"published,omitempty"`
	Description   string            `json:"description,omitempty"`
	Subject       []opdsSubject     `json:"subject,omitempty"`
	BelongsTo     *opdsBelongsTo    `json:"belongsTo,omitempty"`
	NumberOfPages int               `json:"numberOfPages,omitempty"`
}

type opdsPublication struct {
	Metadata opdsPublicationMetadata `json:"metadata"`
	Links    []opdsLink              `json:"links"`
	Images   []opdsLink              `json:"images,omitempty"`
}

type opdsBook struct {
	ID                  int64
	LibraryID           int64
	Title               string
	Authors             string
	Series              string
	SeriesNumber        float64
	SeriesNumberDisplay string
	Publisher           string
	Published           string
	Description         string
	Genres              string
	Tags                string
	Language            string
	CoverPath           string
	PageCount           int
	AddedAt             int64
	LastScanned         int64
	MetadataUpdatedAt   int64
	CoverUpdatedOn      int64
	Files               []opdsBookFile
}

type opdsBookFile struct {
	ID           int64
	Format       string
	Size         int64
	LastModified int64
}

type opdsBookScope struct {
	LibraryID string
	Author    string
	Series    string
	ShelfID   string
	Recent    bool
	BookID    string
}

func defaultOPDSSettings() opdsSettings {
	return opdsSettings{Enabled: true, CatalogTitle: "Cryptorum Catalog"}
}

func loadOPDSSettings() opdsSettings {
	settings := defaultOPDSSettings()
	var raw string
	if err := appDB.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, opdsSettingsKey).Scan(&raw); err != nil {
		return settings
	}
	var stored struct {
		Enabled      *bool  `json:"enabled"`
		CatalogTitle string `json:"catalog_title"`
	}
	if json.Unmarshal([]byte(raw), &stored) != nil {
		return settings
	}
	if stored.Enabled != nil {
		settings.Enabled = *stored.Enabled
	}
	if title := strings.TrimSpace(stored.CatalogTitle); title != "" {
		settings.CatalogTitle = title
	}
	return settings
}

func loadOPDSSettingsResponse() OPDSSettingsResponse {
	settings := loadOPDSSettings()
	return OPDSSettingsResponse{
		Enabled:                settings.Enabled,
		CatalogTitle:           settings.CatalogTitle,
		CatalogPath:            "/opds/",
		AuthenticationRequired: appConfig.Auth.Mode != "none",
	}
}

func getOPDSSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if !requirePermission(getUserFromContext(r.Context()), PermissionViewAdmin) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	jsonResponse(w, http.StatusOK, loadOPDSSettingsResponse())
}

func updateOPDSSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if !requirePermission(getUserFromContext(r.Context()), PermissionViewAdmin) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var request opdsSettings
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	request.CatalogTitle = strings.TrimSpace(request.CatalogTitle)
	if request.CatalogTitle == "" || len([]rune(request.CatalogTitle)) > 120 {
		errorResponse(w, http.StatusBadRequest, "Catalog title must be between 1 and 120 characters")
		return
	}
	encoded, _ := json.Marshal(request)
	if _, err := appDB.Exec(`INSERT OR REPLACE INTO app_settings (key, value) VALUES (?, ?)`, opdsSettingsKey, string(encoded)); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to save OPDS settings")
		return
	}
	jsonResponse(w, http.StatusOK, loadOPDSSettingsResponse())
}

func opdsEnabledMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loadOPDSSettings().Enabled {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func opdsAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maintenanceMode.Load() {
			opdsError(w, http.StatusServiceUnavailable, "Maintenance in progress")
			return
		}
		if appConfig.Auth.Mode == "none" {
			if user, err := loadUserByID(1); err == nil {
				next.ServeHTTP(w, r.WithContext(authContextWithUser(r.Context(), user)))
				return
			}
			opdsError(w, http.StatusInternalServerError, "Authentication store unavailable")
			return
		}

		if sessionID, err := r.Cookie(sessionCookieName); err == nil && sessionStore != nil {
			if session, err := sessionStore.ValidateSession(sessionID.Value); err == nil && session != nil && session.UserID == 1 {
				if user, err := loadUserByID(1); err == nil {
					ctx := authContextWithSession(r.Context(), session)
					ctx = authContextWithUser(ctx, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		key := loginThrottleKey(r)
		if retryAfter, blocked := loginRetryAfter(key); blocked {
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
			opdsError(w, http.StatusTooManyRequests, "Too many authentication attempts")
			return
		}
		username, password, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(username), []byte(appConfig.Auth.Username)) != 1 ||
			!auth.VerifyPasswordHash(password, appConfig.Auth.PasswordHash) {
			if ok {
				recordLoginFailure(key)
			}
			opdsUnauthorized(w)
			return
		}
		clearLoginFailures(key)
		user, err := loadUserByID(1)
		if err != nil {
			opdsError(w, http.StatusInternalServerError, "Authentication store unavailable")
			return
		}
		next.ServeHTTP(w, r.WithContext(authContextWithUser(r.Context(), user)))
	})
}

func opdsAuthenticationDocument() map[string]any {
	methods := []map[string]any{}
	if appConfig.Auth.Mode != "none" {
		methods = append(methods, map[string]any{
			"type":   "http://opds-spec.org/auth/basic",
			"labels": map[string]string{"login": "Username", "password": "Password"},
		})
	}
	return map[string]any{
		"id":             "/opds/authentication",
		"title":          loadOPDSSettings().CatalogTitle,
		"authentication": methods,
	}
}

func handleOPDSAuthenticationHandler(w http.ResponseWriter, r *http.Request) {
	opdsJSON(w, http.StatusOK, opdsAuthenticationType, opdsAuthenticationDocument())
}

func opdsUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Cryptorum OPDS", charset="UTF-8"`)
	w.Header().Set("Link", `</opds/authentication>; rel="http://opds-spec.org/auth/document"; type="application/opds-authentication+json"`)
	opdsJSON(w, http.StatusUnauthorized, opdsAuthenticationType, opdsAuthenticationDocument())
}

func opdsError(w http.ResponseWriter, status int, message string) {
	opdsJSON(w, status, "application/problem+json", map[string]any{"status": status, "title": message})
}

func opdsJSON(w http.ResponseWriter, status int, contentType string, value any) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func handleOPDSRootHandler(w http.ResponseWriter, r *http.Request) {
	settings := loadOPDSSettings()
	feed := opdsFeed{
		Metadata: opdsFeedMetadata{Title: settings.CatalogTitle, Modified: time.Now().UTC().Format(time.RFC3339)},
		Links: []opdsLink{
			{Href: "/opds/", Type: opdsMediaType, Rel: "self"},
			{Href: "/opds/", Type: opdsMediaType, Rel: "start"},
			{Href: "/opds/search{?query}", Type: opdsMediaType, Rel: "search", Templated: true},
		},
		Navigation: []opdsLink{
			{Href: "/opds/publications", Type: opdsMediaType, Rel: "subsection", Title: "All Books"},
			{Href: "/opds/recent", Type: opdsMediaType, Rel: "subsection", Title: "Recently Added"},
			{Href: "/opds/libraries", Type: opdsMediaType, Rel: "subsection", Title: "Libraries"},
			{Href: "/opds/authors", Type: opdsMediaType, Rel: "subsection", Title: "Authors"},
			{Href: "/opds/series", Type: opdsMediaType, Rel: "subsection", Title: "Series"},
			{Href: "/opds/shelves", Type: opdsMediaType, Rel: "subsection", Title: "Shelves"},
		},
	}
	opdsJSON(w, http.StatusOK, opdsMediaType, feed)
}

func handleOPDSCatalogHandler(w http.ResponseWriter, r *http.Request) {
	handleOPDSPublicationsHandler(w, r)
}

func handleOPDSPublicationsHandler(w http.ResponseWriter, r *http.Request) {
	scope := opdsBookScope{
		LibraryID: strings.TrimSpace(r.URL.Query().Get("library")),
		Author:    strings.TrimSpace(r.URL.Query().Get("author")),
		Series:    strings.TrimSpace(r.URL.Query().Get("series")),
		ShelfID:   strings.TrimSpace(r.URL.Query().Get("shelf")),
	}
	title := "All Books"
	if scope.LibraryID != "" {
		title = "Library"
		_ = appDB.QueryRow(`SELECT name FROM library WHERE id = ?`, scope.LibraryID).Scan(&title)
	} else if scope.Author != "" {
		title = scope.Author
	} else if scope.Series != "" {
		title = scope.Series
	} else if scope.ShelfID != "" {
		title = "Shelf"
		_ = appDB.QueryRow(`SELECT name FROM shelf WHERE id = ?`, scope.ShelfID).Scan(&title)
	}
	handleOPDSBookFeed(w, r, title, scope)
}

func handleOPDSRecentHandler(w http.ResponseWriter, r *http.Request) {
	handleOPDSBookFeed(w, r, "Recently Added", opdsBookScope{Recent: true})
}

func handleOPDSSearchHandler(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if query == "" {
		query = strings.TrimSpace(r.URL.Query().Get("q"))
	}
	if query == "" {
		handleOPDSBookFeed(w, r, "Search", opdsBookScope{})
		return
	}
	current := getUserFromContext(r.Context())
	var ids []int64
	for offset := 0; offset < searchMaxResults; offset += searchDefaultPageLimit {
		page, err := searchBooks(query, "", current, BookSearchFilters{}, offset, searchDefaultPageLimit)
		if err != nil {
			opdsError(w, http.StatusInternalServerError, "Search failed")
			return
		}
		for _, result := range page.Results {
			ids = append(ids, result.ID)
		}
		if !page.HasMore {
			break
		}
	}
	books, err := loadOPDSBooksByIDs(current, ids)
	if err != nil {
		opdsError(w, http.StatusInternalServerError, "Failed to generate catalog")
		return
	}
	writeOPDSBookFeed(w, r, fmt.Sprintf("Search results for %s", query), books)
}

func handleOPDSPublicationHandler(w http.ResponseWriter, r *http.Request) {
	books, err := loadOPDSBooks(getUserFromContext(r.Context()), opdsBookScope{BookID: chi.URLParam(r, "bookID")})
	if err != nil {
		opdsError(w, http.StatusInternalServerError, "Failed to load publication")
		return
	}
	if len(books) == 0 {
		opdsError(w, http.StatusNotFound, "Publication not found")
		return
	}
	opdsJSON(w, http.StatusOK, opdsPublicationType, makeOPDSPublication(books[0]))
}

func handleOPDSBookFeed(w http.ResponseWriter, r *http.Request, title string, scope opdsBookScope) {
	books, err := loadOPDSBooks(getUserFromContext(r.Context()), scope)
	if err != nil {
		opdsError(w, http.StatusInternalServerError, "Failed to generate catalog")
		return
	}
	writeOPDSBookFeed(w, r, title, books)
}

func writeOPDSBookFeed(w http.ResponseWriter, r *http.Request, title string, books []opdsBook) {
	publications := make([]opdsPublication, 0, len(books))
	for _, book := range books {
		publications = append(publications, makeOPDSPublication(book))
	}
	feed := opdsFeed{
		Metadata:     opdsFeedMetadata{Title: title, Modified: opdsBooksModified(books), NumberOfItems: len(books)},
		Links:        []opdsLink{{Href: r.URL.RequestURI(), Type: opdsMediaType, Rel: "self"}, {Href: "/opds/", Type: opdsMediaType, Rel: "start"}},
		Publications: publications,
	}
	opdsJSON(w, http.StatusOK, opdsMediaType, feed)
}

func loadOPDSBooks(current *AppUser, scope opdsBookScope) ([]opdsBook, error) {
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	conditions := []string{ownerClause, `EXISTS (SELECT 1 FROM book_file active_bf WHERE active_bf.book_id = b.id AND active_bf.missing_at IS NULL)`}
	args := append([]interface{}{userIDForScopedRows(current)}, ownerArgs...)
	if scope.BookID != "" {
		conditions = append(conditions, "b.id = ?")
		args = append(args, scope.BookID)
	}
	if scope.LibraryID != "" {
		conditions = append(conditions, "b.library_id = ?")
		args = append(args, scope.LibraryID)
	}
	if scope.Author != "" {
		key := normalizedAuthorMatchKey(scope.Author)
		conditions = append(conditions, `EXISTS (SELECT 1 FROM json_each(COALESCE(bm.authors, '[]')) WHERE `+normalizedAuthorSQLExpression("value")+` = ?)`)
		args = append(args, key)
	}
	if scope.Series != "" {
		conditions = append(conditions, "COALESCE(bm.series, '') = ?")
		args = append(args, scope.Series)
	}
	orderBy := titleSortSQL() + " ASC, b.id ASC"
	if scope.Recent {
		orderBy = "b.added_at DESC, b.id DESC"
	}
	if scope.ShelfID != "" {
		var isMagic int
		var rulesJSON, sortBy, sortDir string
		err := appDB.QueryRow(`SELECT is_magic, COALESCE(rules_json, ''), COALESCE(sort_by, ''), COALESCE(sort_dir, '') FROM shelf WHERE id = ?`, scope.ShelfID).Scan(&isMagic, &rulesJSON, &sortBy, &sortDir)
		if err == sql.ErrNoRows {
			return []opdsBook{}, nil
		}
		if err != nil {
			return nil, err
		}
		if isMagic == 1 {
			condition, conditionArgs, err := buildMagicShelfConditions(rulesJSON)
			if err != nil {
				return nil, err
			}
			conditions = append(conditions, "("+condition+")")
			args = append(args, conditionArgs...)
		} else {
			conditions = append(conditions, `EXISTS (SELECT 1 FROM book_shelf bs WHERE bs.book_id = b.id AND bs.shelf_id = ?)`)
			args = append(args, scope.ShelfID)
		}
		orderBy = bookListOrderBy(sortBy, sortDir)
	}

	query := `
		SELECT b.id, b.library_id, COALESCE(bm.title, 'Unknown'), COALESCE(bm.authors, '[]'),
		       COALESCE(bm.series, ''), COALESCE(bm.series_number, 0), COALESCE(bm.series_number_display, ''),
		       COALESCE(bm.publisher, ''), COALESCE(bm.pub_date, ''), COALESCE(bm.description, ''),
		       COALESCE(bm.genres, '[]'), COALESCE(bm.tags, '[]'), COALESCE(bm.language, ''),
		       COALESCE(bm.cover_path, ''), COALESCE(bm.page_count, 0), b.added_at, b.last_scanned,
		       COALESCE(bm.metadata_updated_at, 0), COALESCE(bm.cover_updated_on, 0)
		FROM book b
		JOIN library l ON l.id = b.library_id
		LEFT JOIN book_metadata bm ON bm.book_id = b.id
		LEFT JOIN reading_progress rp ON rp.book_id = b.id AND rp.owner_user_id = ?
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY ` + orderBy
	if scope.Recent {
		query += " LIMIT 100"
	}
	rows, err := appDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	books := []opdsBook{}
	for rows.Next() {
		var book opdsBook
		if err := rows.Scan(&book.ID, &book.LibraryID, &book.Title, &book.Authors, &book.Series, &book.SeriesNumber,
			&book.SeriesNumberDisplay, &book.Publisher, &book.Published, &book.Description, &book.Genres, &book.Tags,
			&book.Language, &book.CoverPath, &book.PageCount, &book.AddedAt, &book.LastScanned,
			&book.MetadataUpdatedAt, &book.CoverUpdatedOn); err != nil {
			return nil, err
		}
		files, err := loadOPDSBookFiles(book.ID)
		if err != nil {
			return nil, err
		}
		book.Files = files
		books = append(books, book)
	}
	return books, rows.Err()
}

func loadOPDSBooksByIDs(current *AppUser, ids []int64) ([]opdsBook, error) {
	if len(ids) == 0 {
		return []opdsBook{}, nil
	}
	byID := make(map[int64]opdsBook, len(ids))
	for _, id := range ids {
		books, err := loadOPDSBooks(current, opdsBookScope{BookID: strconv.FormatInt(id, 10)})
		if err != nil {
			return nil, err
		}
		if len(books) > 0 {
			byID[id] = books[0]
		}
	}
	books := make([]opdsBook, 0, len(byID))
	for _, id := range ids {
		if book, ok := byID[id]; ok {
			books = append(books, book)
		}
	}
	return books, nil
}

func loadOPDSBookFiles(bookID int64) ([]opdsBookFile, error) {
	rows, err := appDB.Query(`SELECT id, COALESCE(format, ''), size, last_modified FROM book_file WHERE book_id = ? AND missing_at IS NULL`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []opdsBookFile{}
	for rows.Next() {
		var file opdsBookFile
		if err := rows.Scan(&file.ID, &file.Format, &file.Size, &file.LastModified); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		left, right := opdsFormatPriority(files[i].Format), opdsFormatPriority(files[j].Format)
		if left == right {
			return files[i].ID < files[j].ID
		}
		return left < right
	})
	return files, rows.Err()
}

func makeOPDSPublication(book opdsBook) opdsPublication {
	authors := parseMetadataJSONList(book.Authors)
	contributors := make([]opdsContributor, 0, len(authors))
	for _, author := range authors {
		if name := strings.TrimSpace(author); name != "" {
			contributors = append(contributors, opdsContributor{Name: name})
		}
	}
	subjectNames := mergeMetadataTagLists(parseMetadataJSONList(book.Tags), parseMetadataJSONList(book.Genres))
	subjects := make([]opdsSubject, 0, len(subjectNames))
	for _, name := range subjectNames {
		subjects = append(subjects, opdsSubject{Name: name})
	}
	metadata := opdsPublicationMetadata{
		Type: "http://schema.org/Book", Identifier: fmt.Sprintf("urn:cryptorum:book:%d", book.ID),
		Title: book.Title, Author: contributors, Modified: opdsBookModified(book), Published: strings.TrimSpace(book.Published),
		Description: book.Description, Subject: subjects,
		NumberOfPages: book.PageCount,
	}
	if book.Publisher != "" {
		metadata.Publisher = []opdsContributor{{Name: book.Publisher}}
	}
	if book.Language != "" {
		metadata.Language = []string{book.Language}
	}
	if book.Series != "" {
		metadata.BelongsTo = &opdsBelongsTo{Series: []opdsContributor{{Name: book.Series, Position: book.SeriesNumber}}}
	}
	links := []opdsLink{{Href: fmt.Sprintf("/opds/publications/%d", book.ID), Type: opdsPublicationType, Rel: "self"}}
	for _, file := range book.Files {
		links = append(links, opdsLink{
			Href: fmt.Sprintf("/opds/books/%d/files/%d/download", book.ID, file.ID), Type: opdsMIMEType(file.Format),
			Rel: opdsAcquisitionRel, Title: strings.ToUpper(strings.TrimSpace(file.Format)),
		})
	}
	publication := opdsPublication{Metadata: metadata, Links: links}
	if coverPath, _, err := resolveCoverFile(strconv.FormatInt(book.ID, 10)); err == nil && coverPath != "" {
		publication.Images = []opdsLink{
			{Href: fmt.Sprintf("/opds/books/%d/thumbnail", book.ID), Type: "image/jpeg", Rel: "http://opds-spec.org/image/thumbnail", Width: 240},
			{Href: fmt.Sprintf("/opds/books/%d/cover", book.ID), Type: opdsImageMIMEType(coverPath), Rel: "http://opds-spec.org/image"},
		}
	}
	return publication
}

func opdsBookModified(book opdsBook) string {
	latest := max(book.AddedAt, book.LastScanned, book.MetadataUpdatedAt, book.CoverUpdatedOn)
	for _, file := range book.Files {
		latest = max(latest, file.LastModified)
	}
	if latest <= 0 {
		return ""
	}
	return time.Unix(latest, 0).UTC().Format(time.RFC3339)
}

func opdsBooksModified(books []opdsBook) string {
	var latest int64
	for _, book := range books {
		latest = max(latest, book.AddedAt, book.LastScanned, book.MetadataUpdatedAt, book.CoverUpdatedOn)
		for _, file := range book.Files {
			latest = max(latest, file.LastModified)
		}
	}
	if latest == 0 {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return time.Unix(latest, 0).UTC().Format(time.RFC3339)
}

func opdsFormatPriority(format string) int {
	priorities := map[string]int{"epub": 0, "pdf": 1, "fb2": 2, "cbz": 3, "cbr": 4, "cb7": 5, "txt": 6, "html": 7, "htm": 7, "md": 8, "mobi": 9, "azw": 10, "azw3": 10, "rtf": 11, "m4b": 12, "mp3": 13}
	if priority, ok := priorities[strings.ToLower(strings.TrimSpace(format))]; ok {
		return priority
	}
	return 100
}

func opdsMIMEType(format string) string {
	types := map[string]string{
		"epub": "application/epub+zip", "pdf": "application/pdf", "fb2": "application/fb2+zip",
		"cbz": "application/vnd.comicbook+zip", "cbr": "application/vnd.comicbook-rar", "cb7": "application/x-7z-compressed", "cbt": "application/x-tar",
		"txt": "text/plain", "html": "text/html", "htm": "text/html", "md": "text/markdown",
		"mobi": "application/x-mobipocket-ebook", "azw": "application/vnd.amazon.ebook", "azw3": "application/vnd.amazon.ebook",
		"rtf": "application/rtf", "m4b": "audio/mp4", "m4a": "audio/mp4", "mp3": "audio/mpeg", "aac": "audio/aac",
		"ogg": "audio/ogg", "opus": "audio/ogg", "flac": "audio/flac", "wav": "audio/wav",
	}
	if mime := types[strings.ToLower(strings.TrimSpace(format))]; mime != "" {
		return mime
	}
	return "application/octet-stream"
}

func opdsImageMIMEType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".webp":
		return "image/webp"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	default:
		return "image/jpeg"
	}
}

func handleOPDSLibrariesHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	ownerClause, args := userOwnershipClause(current, "l")
	rows, err := appDB.Query(`SELECT l.id, l.name, COUNT(DISTINCT b.id) FROM library l LEFT JOIN book b ON b.library_id = l.id AND EXISTS (SELECT 1 FROM book_file bf WHERE bf.book_id = b.id AND bf.missing_at IS NULL) WHERE `+ownerClause+` GROUP BY l.id, l.name ORDER BY l.name COLLATE NOCASE`, args...)
	if err != nil {
		opdsError(w, http.StatusInternalServerError, "Failed to load libraries")
		return
	}
	defer rows.Close()
	navigation := []opdsLink{}
	for rows.Next() {
		var id int64
		var name string
		var count int
		if rows.Scan(&id, &name, &count) == nil {
			navigation = append(navigation, opdsLink{Href: "/opds/publications?library=" + strconv.FormatInt(id, 10), Type: opdsMediaType, Rel: "subsection", Title: fmt.Sprintf("%s (%d)", name, count)})
		}
	}
	writeOPDSNavigationFeed(w, r, "Libraries", navigation)
}

func handleOPDSAuthorsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	ownerClause, args := userOwnershipClause(current, "l")
	rows, err := appDB.Query(`SELECT CAST(j.value AS TEXT), COUNT(DISTINCT b.id) FROM book b JOIN library l ON l.id = b.library_id JOIN book_metadata bm ON bm.book_id = b.id JOIN json_each(COALESCE(bm.authors, '[]')) j WHERE `+ownerClause+` AND TRIM(CAST(j.value AS TEXT)) != '' AND EXISTS (SELECT 1 FROM book_file bf WHERE bf.book_id = b.id AND bf.missing_at IS NULL) GROUP BY `+normalizedAuthorSQLExpression("j.value")+` ORDER BY CAST(j.value AS TEXT) COLLATE NOCASE`, args...)
	if err != nil {
		opdsError(w, http.StatusInternalServerError, "Failed to load authors")
		return
	}
	defer rows.Close()
	navigation := []opdsLink{}
	for rows.Next() {
		var name string
		var count int
		if rows.Scan(&name, &count) == nil {
			name = canonicalAuthorOptionName(name)
			navigation = append(navigation, opdsLink{Href: "/opds/publications?author=" + url.QueryEscape(name), Type: opdsMediaType, Rel: "subsection", Title: fmt.Sprintf("%s (%d)", name, count)})
		}
	}
	writeOPDSNavigationFeed(w, r, "Authors", navigation)
}

func handleOPDSSeriesHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	ownerClause, args := userOwnershipClause(current, "l")
	rows, err := appDB.Query(`SELECT bm.series, COUNT(DISTINCT b.id) FROM book b JOIN library l ON l.id = b.library_id JOIN book_metadata bm ON bm.book_id = b.id WHERE `+ownerClause+` AND TRIM(COALESCE(bm.series, '')) != '' AND EXISTS (SELECT 1 FROM book_file bf WHERE bf.book_id = b.id AND bf.missing_at IS NULL) GROUP BY bm.series ORDER BY bm.series COLLATE NOCASE`, args...)
	if err != nil {
		opdsError(w, http.StatusInternalServerError, "Failed to load series")
		return
	}
	defer rows.Close()
	navigation := []opdsLink{}
	for rows.Next() {
		var name string
		var count int
		if rows.Scan(&name, &count) == nil {
			navigation = append(navigation, opdsLink{Href: "/opds/publications?series=" + url.QueryEscape(name), Type: opdsMediaType, Rel: "subsection", Title: fmt.Sprintf("%s (%d)", name, count)})
		}
	}
	writeOPDSNavigationFeed(w, r, "Series", navigation)
}

func handleOPDSShelvesHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	ownerClause, args := userOwnershipClause(current, "s")
	rows, err := appDB.Query(`SELECT s.id, s.name, s.is_magic, COALESCE(s.rules_json, ''), COUNT(DISTINCT CASE WHEN bf.id IS NOT NULL THEN b.id END) FROM shelf s LEFT JOIN book_shelf bs ON bs.shelf_id = s.id LEFT JOIN book b ON b.id = bs.book_id LEFT JOIN book_file bf ON bf.book_id = b.id AND bf.missing_at IS NULL WHERE `+ownerClause+` GROUP BY s.id, s.name, s.is_magic, s.rules_json, s.sort_order ORDER BY CASE WHEN s.sort_order = 0 THEN 1 ELSE 0 END, s.sort_order, s.name COLLATE NOCASE`, args...)
	if err != nil {
		opdsError(w, http.StatusInternalServerError, "Failed to load shelves")
		return
	}
	defer rows.Close()
	navigation := []opdsLink{}
	for rows.Next() {
		var id int64
		var name, rules string
		var isMagic, count int
		if rows.Scan(&id, &name, &isMagic, &rules, &count) != nil {
			continue
		}
		if isMagic == 1 {
			if magicCount, err := countMagicShelfBooks(rules, current); err == nil {
				count = int(magicCount)
			}
		}
		navigation = append(navigation, opdsLink{Href: "/opds/publications?shelf=" + strconv.FormatInt(id, 10), Type: opdsMediaType, Rel: "subsection", Title: fmt.Sprintf("%s (%d)", name, count)})
	}
	writeOPDSNavigationFeed(w, r, "Shelves", navigation)
}

func writeOPDSNavigationFeed(w http.ResponseWriter, r *http.Request, title string, navigation []opdsLink) {
	feed := opdsFeed{Metadata: opdsFeedMetadata{Title: title, Modified: time.Now().UTC().Format(time.RFC3339), NumberOfItems: len(navigation)}, Links: []opdsLink{{Href: r.URL.RequestURI(), Type: opdsMediaType, Rel: "self"}, {Href: "/opds/", Type: opdsMediaType, Rel: "start"}}, Navigation: navigation}
	opdsJSON(w, http.StatusOK, opdsMediaType, feed)
}

func handleOPDSFileDownloadHandler(w http.ResponseWriter, r *http.Request) {
	ServeBookFileByIDHandler(w, r)
}

func handleOPDSCoverHandler(w http.ResponseWriter, r *http.Request) {
	ServeCoverHandler(w, r)
}

func handleOPDSThumbnailHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	query.Set("size", "small")
	r.URL.RawQuery = query.Encode()
	ServeCoverThumbHandler(w, r)
}

func downloadBookHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	var fileID int64
	rows, err := appDB.Query(`SELECT id, format FROM book_file WHERE book_id = ? AND missing_at IS NULL`, bookID)
	if err != nil {
		opdsError(w, http.StatusNotFound, "Book not found")
		return
	}
	defer rows.Close()
	bestPriority := 1000
	for rows.Next() {
		var id int64
		var format string
		if rows.Scan(&id, &format) == nil && opdsFormatPriority(format) < bestPriority {
			fileID, bestPriority = id, opdsFormatPriority(format)
		}
	}
	if fileID == 0 {
		opdsError(w, http.StatusNotFound, "Book not found")
		return
	}
	rctx := chi.RouteContext(r.Context())
	rctx.URLParams.Add("bookID", bookID)
	rctx.URLParams.Add("fileID", strconv.FormatInt(fileID, 10))
	ServeBookFileByIDHandler(w, r)
}
