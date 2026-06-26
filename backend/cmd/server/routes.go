package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"cryptorum/internal/auth"
	"cryptorum/internal/config"
	"cryptorum/internal/coverprefs"
	"cryptorum/internal/db"
	"cryptorum/internal/scanner"
	"cryptorum/internal/seriesnum"
)

type filterList []string

func (f *filterList) UnmarshalJSON(data []byte) error {
	if strings.TrimSpace(string(data)) == "null" {
		*f = nil
		return nil
	}

	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		*f = cleanFilterValues(values, false)
		return nil
	}

	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*f = cleanFilterValues([]string{value}, false)
	return nil
}

type bulkFilterRequest struct {
	LibraryID       string     `json:"library_id"`
	Author          filterList `json:"author"`
	Series          filterList `json:"series"`
	Genre           filterList `json:"genre"`
	Tags            filterList `json:"tags"`
	Format          filterList `json:"format"`
	Status          filterList `json:"status"`
	Query           string     `json:"q"`
	FilterMode      string     `json:"filter_mode"`
	ValueFilterMode string     `json:"value_filter_mode"`
}

func cleanFilterValues(values []string, splitComma bool) []string {
	var cleaned []string
	seen := make(map[string]bool)
	for _, raw := range values {
		parts := []string{raw}
		if splitComma {
			parts = strings.Split(raw, ",")
		}
		for _, part := range parts {
			value := strings.TrimSpace(part)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func addHierarchicalJSONFilterCondition(
	addFilterCondition func(string, ...interface{}),
	column string,
	value string,
) {
	addFilterCondition(hierarchicalJSONFilterCondition(column), hierarchicalJSONFilterArgs(value)...)
}

func addTagOrGenreFilterCondition(addFilterCondition func(string, ...interface{}), value string) {
	tagCondition := hierarchicalJSONFilterCondition("bm.tags")
	genreCondition := hierarchicalJSONFilterCondition("bm.genres")
	args := append(hierarchicalJSONFilterArgs(value), hierarchicalJSONFilterArgs(value)...)
	addFilterCondition("("+tagCondition+" OR "+genreCondition+")", args...)
}

func hierarchicalJSONFilterCondition(column string) string {
	return fmt.Sprintf(
		`EXISTS (SELECT 1 FROM json_each(COALESCE(%s, '[]')) WHERE value = ? OR value LIKE ? OR value LIKE ? OR value LIKE ?)`,
		column,
	)
}

func hierarchicalJSONFilterArgs(value string) []interface{} {
	return []interface{}{
		value,
		value + ".%",
		"%." + value,
		"%." + value + ".%",
	}
}

type sqlFilterGroup struct {
	condition string
	args      []interface{}
}

func normalizeAcrossFilterMode(mode string) string {
	mode = strings.ToUpper(strings.TrimSpace(mode))
	if mode == "OR" || mode == "NOT" {
		return mode
	}
	return "AND"
}

func normalizeValueFilterMode(mode string) string {
	mode = strings.ToUpper(strings.TrimSpace(mode))
	if mode == "AND" || mode == "NOT" {
		return mode
	}
	return "OR"
}

func buildFilterGroup(valueFilterMode string, build func(add func(string, ...interface{}))) (sqlFilterGroup, bool) {
	var conditions []string
	var args []interface{}
	add := func(condition string, values ...interface{}) {
		conditions = append(conditions, condition)
		args = append(args, values...)
	}

	build(add)
	if len(conditions) == 0 {
		return sqlFilterGroup{}, false
	}

	mode := normalizeValueFilterMode(valueFilterMode)
	switch mode {
	case "AND":
		return sqlFilterGroup{condition: "(" + strings.Join(conditions, " AND ") + ")", args: args}, true
	case "NOT":
		return sqlFilterGroup{condition: "NOT (" + strings.Join(conditions, " OR ") + ")", args: args}, true
	default:
		return sqlFilterGroup{condition: "(" + strings.Join(conditions, " OR ") + ")", args: args}, true
	}
}

func combineFilterGroups(groups []sqlFilterGroup, filterMode string) (string, []interface{}) {
	if len(groups) == 0 {
		return "", nil
	}

	conditions := make([]string, 0, len(groups))
	args := make([]interface{}, 0)
	for _, group := range groups {
		conditions = append(conditions, group.condition)
		args = append(args, group.args...)
	}

	mode := normalizeAcrossFilterMode(filterMode)
	switch mode {
	case "OR":
		return "(" + strings.Join(conditions, " OR ") + ")", args
	case "NOT":
		return "NOT (" + strings.Join(conditions, " OR ") + ")", args
	default:
		return "(" + strings.Join(conditions, " AND ") + ")", args
	}
}

func buildBulkFilterQuery(user *AppUser, req bulkFilterRequest) (string, []interface{}) {
	query := `
		SELECT b.id
		FROM book b
		JOIN library l ON b.library_id = l.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id`
	var args []interface{}
	var conditions []string
	var filterGroups []sqlFilterGroup

	addFilterGroup := func(build func(add func(string, ...interface{}))) {
		if group, ok := buildFilterGroup(req.ValueFilterMode, build); ok {
			filterGroups = append(filterGroups, group)
		}
	}

	if req.LibraryID != "" {
		conditions = append(conditions, "b.library_id = ?")
		args = append(args, req.LibraryID)
	}
	if user != nil && !userCanAccessAllData(user) {
		conditions = append(conditions, "l.owner_user_id = ?")
		args = append(args, user.ID)
	}
	addFilterGroup(func(add func(string, ...interface{})) {
		for _, value := range req.Author {
			addAuthorFilterCondition(add, "bm.authors", value)
		}
	})
	addFilterGroup(func(add func(string, ...interface{})) {
		for _, value := range req.Series {
			add("COALESCE(bm.series, '') = ?", value)
		}
	})
	addFilterGroup(func(add func(string, ...interface{})) {
		for _, value := range cleanFilterValues(req.Genre, true) {
			addTagOrGenreFilterCondition(add, value)
		}
	})
	addFilterGroup(func(add func(string, ...interface{})) {
		for _, value := range cleanFilterValues(req.Tags, true) {
			addTagOrGenreFilterCondition(add, value)
		}
	})
	addFilterGroup(func(add func(string, ...interface{})) {
		for _, value := range req.Format {
			format := strings.ToLower(strings.TrimSpace(value))
			if format != "" {
				add("EXISTS (SELECT 1 FROM book_file filter_bf WHERE filter_bf.book_id = b.id AND filter_bf.missing_at IS NULL AND LOWER(filter_bf.format) = ?)", format)
			}
		}
	})
	addFilterGroup(func(add func(string, ...interface{})) {
		for _, value := range req.Status {
			add("COALESCE(rp.status, 'unread') = ?", value)
		}
	})
	if strings.TrimSpace(req.Query) != "" {
		searchText := `LOWER(
			COALESCE(bm.title, '') || ' ' ||
			COALESCE(bm.authors, '') || ' ' ||
			REPLACE(REPLACE(COALESCE(bm.authors, ''), '.', ''), ' ', '') || ' ' ||
			COALESCE(bm.description, '') || ' ' ||
			COALESCE(bm.series, '') || ' ' ||
			COALESCE(bm.series_number_display, '') || ' ' ||
			CASE WHEN COALESCE(bm.series_number, 0) != 0 THEN CAST(bm.series_number AS TEXT) ELSE '' END || ' ' ||
			COALESCE(bm.isbn, '') || ' ' ||
			COALESCE(bm.asin, '') || ' ' ||
			` + activeFileSearchTextSQL + `
		)`
		for _, token := range searchTokens(req.Query) {
			conditions = append(conditions, searchText+" LIKE ?")
			args = append(args, "%"+token+"%")
		}
	}

	if filterCondition, filterArgs := combineFilterGroups(filterGroups, req.FilterMode); filterCondition != "" {
		conditions = append(conditions, filterCondition)
		args = append(args, filterArgs...)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	return query, args
}

func initRoutes(r *chi.Mux) {
	// Health check - public
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Auth routes - public
	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/login", loginHandler)
		r.Post("/logout", logoutHandler)
		r.Get("/check", authCheckHandler)
	})

	// Static files
	FileServer(r, "/_app", http.Dir("./static/_app"))

	// API routes - protected
	r.Route("/api", func(r chi.Router) {
		r.Use(authMiddleware)

		// Books
		r.Route("/books", func(r chi.Router) {
			r.Get("/", getBooksHandler)
			r.Get("/discover", getDiscoverBooksHandler)
			r.Get("/navigation", getBookNavigationHandler)
			r.Post("/bulk-delete", bulkDeleteBooksHandler)
			r.Post("/bulk-delete-by-filter", bulkDeleteByFilterHandler)
			r.Post("/bulk-metadata", bulkUpdateMetadataHandler)
			r.Route("/{bookID}", func(r chi.Router) {
				r.Get("/", getBookHandler)
				r.Put("/", updateBookHandler)
				r.Patch("/status", updateBookStatusHandler)
				r.Delete("/", deleteBookHandler)
				r.Get("/files", getBookFilesHandler)
				r.Get("/pdf/pages", getPdfPageCountHandler)
				r.Get("/files/{fileID}/download", ServeBookFileByIDHandler)
				r.Get("/files/{fileID}/convert", ConvertBookFileHandler)
				r.Get("/progress", GetReadingProgressHandler)
				r.Put("/progress", UpdateReadingProgressHandler)
				r.Put("/speed-reader", UpdateSpeedReaderProgressHandler)
				r.Post("/cover/regenerate", RegenerateBookCoverHandler)
				r.Post("/cover/custom", UploadBookCoverHandler)
				r.Delete("/cover/custom", ResetBookCoverHandler)
				r.Get("/annotations", GetAnnotationsHandler)
				r.Post("/annotations", CreateAnnotationHandler)
				r.Delete("/annotations/{id}", DeleteAnnotationHandler)
				r.Get("/bookmarks", GetBookmarksHandler)
				r.Post("/bookmarks", CreateBookmarkHandler)
				r.Delete("/bookmarks/{id}", DeleteBookmarkHandler)
				r.Post("/sessions", StartReadingSessionHandler)
				r.Get("/sessions", GetBookSessionsHandler)
				r.Put("/sessions/{sessionID}", EndReadingSessionHandler)
				r.Delete("/sessions/{sessionID}", DeleteReadingSessionHandler)
				r.Get("/similar", getSimilarBooksHandler)
				r.Get("/continuous", ServeContinuousBookHandler)
				r.Get("/continuous/toc", ServeContinuousTocHandler)
				r.Get("/continuous/media/*", ServeContinuousMediaHandler)
				r.Get("/continuous/styles", ServeContinuousStylesHandler)
			})
		})

		// Libraries
		r.Route("/libraries", func(r chi.Router) {
			r.Get("/", getLibrariesHandler)
			r.Post("/", createLibraryHandler)
			r.Patch("/order", updateLibraryOrderHandler)
			r.Route("/{libraryID}", func(r chi.Router) {
				r.Get("/", getLibraryHandler)
				r.Put("/", updateLibraryHandler)
				r.Delete("/", deleteLibraryHandler)
				r.Post("/scan", scanLibraryHandler)
				r.Get("/books", getLibraryBooksHandler)
			})
		})

		// Shelves
		r.Route("/shelves", func(r chi.Router) {
			r.Get("/", getShelvesHandler)
			r.Post("/", createShelfHandler)
			r.Route("/{shelfID}", func(r chi.Router) {
				r.Get("/", getShelfHandler)
				r.Put("/", updateShelfHandler)
				r.Delete("/", deleteShelfHandler)
				r.Get("/books", getShelfBooksHandler)
				r.Post("/books", addBookToShelfHandler)
				r.Post("/books/bulk", bulkAddToShelfHandler)
				r.Post("/books/bulk-by-filter", bulkAddToShelfByFilterHandler)
				r.Delete("/books/bulk", bulkRemoveBooksFromShelfHandler)
				r.Delete("/books/{bookID}", removeBookFromShelfHandler)
			})
		})

		// Search
		r.Get("/search", searchBooksHandler)

		// Authors and Series
		r.Get("/authors", getAuthorsHandler)
		r.Get("/series", getSeriesHandler)
		r.Get("/filter-options", getFilterOptionsHandler)

		// Metadata management
		r.Get("/metadata/{type}", getMetadataHandler)
		r.Get("/metadata/suggestions", getMetadataSuggestionsHandler)

		// Statistics
		r.Get("/dashboard/summary", getDashboardSummaryHandler)
		r.Get("/stats", GetStatsHandler)

		// Reading history
		r.Get("/history", GetReadingHistoryHandler)

		// Admin workflow
		r.Route("/jobs", func(r chi.Router) {
			r.Get("/", ListJobsHandler)
			r.Get("/{jobID}", GetJobHandler)
			r.Post("/{jobID}/cancel", CancelJobHandler)
			r.Post("/metadata-lookup", QueueMetadataLookupJobHandler)
			r.Post("/metadata-apply", QueueMetadataApplyJobHandler)
			r.Delete("/{jobID}", DeleteJobHandler)
		})
		r.Route("/backups", func(r chi.Router) {
			r.Get("/", ListBackupsHandler)
			r.Post("/", CreateBackupHandler)
			r.Post("/{backupName}/restore", RestoreBackupHandler)
			r.Delete("/{backupName}", DeleteBackupHandler)
			r.Get("/{backupName}/download", DownloadBackupHandler)
		})
		r.Route("/users", func(r chi.Router) {
			r.Get("/", ListUsersHandler)
			r.Post("/", CreateUserHandler)
			r.Put("/{userID}", UpdateUserHandler)
			r.Delete("/{userID}", DeleteUserHandler)
		})
		r.Route("/notifications", func(r chi.Router) {
			r.Get("/", ListNotificationsHandler)
			r.Delete("/", DeleteAllNotificationsHandler)
			r.Post("/{notificationID}/read", MarkNotificationReadHandler)
			r.Delete("/{notificationID}", DeleteNotificationHandler)
		})
		r.Get("/logs", ListLogsHandler)

		// Metadata enrichment
		r.Get("/providers", ListProvidersHandler)
		r.Get("/metadata/search", SearchMetadataHandler)
		r.Post("/metadata/apply", ApplyMetadataHandler)
		r.Post("/metadata/lock", LockMetadataFieldHandler)
		r.Post("/metadata/unlock", UnlockMetadataFieldHandler)

		// Library scan
		r.Post("/scan", TriggerScanHandler)
		r.Post("/rebuild-fts", RebuildFTSHandler)

		// Settings
		r.Get("/settings", getSettingsHandler)
		r.Put("/settings/reader", updateReaderSettingsHandler)
		r.Put("/settings/book-covers", updateBookCoverSettingsHandler)
		r.Post("/settings/book-covers/regenerate", regenerateBookCoversHandler)
		r.Put("/settings/backups", updateBackupSettingsHandler)
		r.Put("/bookdrop", updateBookdropHandler)

		// Directory browsing
		r.Get("/directories", getDirectoriesHandler)

		// BookDrop
		r.Get("/bookdrop", getBookdropFilesHandler)
		r.Post("/bookdrop/{id}/import", importBookdropFileHandler)
		r.Delete("/bookdrop/{id}", deleteBookdropFileHandler)

		// SSE
		r.Get("/events", handleSSEHandler)

		// File serving for readers
		r.Get("/books/{bookID}/file", ServeBookFileHandler)
		r.Get("/books/{bookID}/processed-file", ServeProcessedBookFileHandler)
		r.Get("/books/{bookID}/text", GetBookTextHandler)
		r.Get("/epub/{bookID}/resource/*", ServeEpubResourceHandler)
		r.Get("/epub/{bookID}/text", GetEpubTextHandler)
		r.Get("/cbx/{bookID}/page/{pageNum}", ServeCbxPageHandler)
		r.Get("/cbx/{bookID}/pages", getCbxPageCountHandler)
		r.Get("/covers/{bookID}", ServeCoverHandler)
		r.Get("/covers/{bookID}/thumb", ServeCoverThumbHandler)
	})

	// OPDS feed - protected
	r.Route("/opds", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/", handleOPDSRootHandler)
		r.Get("/catalog", handleOPDSCatalogHandler)
		r.Get("/{id}/download", downloadBookHandler)
	})

	// Kobo sync - protected
	r.Route("/kobo", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/{token}/auth/checkcheck", handleKoboAuthHandler)
		r.Get("/{token}/v1/library/sync", handleKoboSyncHandler)
		r.Post("/{token}/v1/library/sync", handleKoboSyncHandler)
	})

	// Frontend SPA
	r.Get("/*", serveSPAHandler)
}

// FileServer sets up a static file server
func FileServer(r chi.Router, path string, root http.FileSystem) {
	prefix := path
	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/_app/immutable/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case r.URL.Path == "/_app/version.json" || r.URL.Path == "/_app/env.js":
			w.Header().Set("Cache-Control", "no-store, max-age=0")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		case strings.HasPrefix(prefix, "/_app"):
			w.Header().Set("Cache-Control", "no-cache, max-age=0")
		}
		fs := http.StripPrefix(prefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	}).ServeHTTP)
}

// serveSPAHandler serves the frontend SPA
func serveSPAHandler(w http.ResponseWriter, r *http.Request) {
	cleanPath := filepath.Clean(r.URL.Path)
	if cleanPath == "." || cleanPath == "/" {
		cleanPath = "/index.html"
	}

	staticPath := filepath.Join("./static", strings.TrimPrefix(cleanPath, "/"))
	if info, err := os.Stat(staticPath); err == nil && !info.IsDir() && filepath.Base(staticPath) != "index.html" {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.ServeFile(w, r, staticPath)
		return
	}

	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	http.ServeFile(w, r, "./static/index.html")
}

// getBooksHandler lists books with pagination, including reading status
func getBooksHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	libraryID := r.URL.Query().Get("library_id")
	status := r.URL.Query().Get("status")
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	author := r.URL.Query().Get("author")
	series := r.URL.Query().Get("series")
	genre := r.URL.Query().Get("genre")
	tags := r.URL.Query().Get("tags")
	format := r.URL.Query().Get("format")
	publisher := r.URL.Query().Get("publisher")
	language := r.URL.Query().Get("language")
	pubDate := r.URL.Query().Get("pub_date")
	filterMode := strings.ToUpper(r.URL.Query().Get("filter_mode"))
	valueFilterMode := r.URL.Query().Get("value_filter_mode")
	sortBy := r.URL.Query().Get("sort")
	sortDir := strings.ToLower(r.URL.Query().Get("sort_dir"))
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	discoveryOnly := r.URL.Query().Get("discovery") == "true"
	includeTotal := r.URL.Query().Get("include_total") != "false"
	filterMode = normalizeAcrossFilterMode(filterMode)

	// Default limit of 50 for lazy loading
	limit := 50
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 200 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	baseQuery := `
		FROM book b
		LEFT JOIN library l ON b.library_id = l.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		LEFT JOIN (
			SELECT book_id, MIN(format) AS format
			FROM book_file
			WHERE missing_at IS NULL
			GROUP BY book_id
		) bf ON b.id = bf.book_id`

	var args []interface{}
	var conditions []string
	var filterGroups []sqlFilterGroup

	queryValues := func(key string, splitComma bool) []string {
		values := r.URL.Query()[key]
		if len(values) == 0 {
			values = []string{r.URL.Query().Get(key)}
		}
		var cleaned []string
		for _, raw := range values {
			if splitComma {
				for _, value := range strings.Split(raw, ",") {
					if trimmed := strings.TrimSpace(value); trimmed != "" {
						cleaned = append(cleaned, trimmed)
					}
				}
				continue
			}
			if trimmed := strings.TrimSpace(raw); trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		return cleaned
	}

	addFilterGroup := func(build func(add func(string, ...interface{}))) {
		if group, ok := buildFilterGroup(valueFilterMode, build); ok {
			filterGroups = append(filterGroups, group)
		}
	}

	if libraryID != "" {
		conditions = append(conditions, "b.library_id = ?")
		args = append(args, libraryID)
	}

	if current != nil && !userCanAccessAllData(current) {
		conditions = append(conditions, "l.owner_user_id = ?")
		args = append(args, current.ID)
	}
	conditions = append(conditions, "EXISTS (SELECT 1 FROM book_file active_bf WHERE active_bf.book_id = b.id AND active_bf.missing_at IS NULL)")
	if discoveryOnly {
		conditions = append(conditions, "COALESCE(l.exclude_from_suggestions, 0) = 0")
	}

	if status != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			for _, value := range queryValues("status", false) {
				add("COALESCE(rp.status, 'unread') = ?", value)
			}
		})
	}

	if searchQuery != "" {
		searchText := `LOWER(
			COALESCE(bm.title, '') || ' ' ||
			COALESCE(bm.authors, '') || ' ' ||
			REPLACE(REPLACE(COALESCE(bm.authors, ''), '.', ''), ' ', '') || ' ' ||
			COALESCE(bm.description, '') || ' ' ||
			COALESCE(bm.series, '') || ' ' ||
			COALESCE(bm.series_number_display, '') || ' ' ||
			CASE WHEN COALESCE(bm.series_number, 0) != 0 THEN CAST(bm.series_number AS TEXT) ELSE '' END || ' ' ||
			COALESCE(bm.isbn, '') || ' ' ||
			COALESCE(bm.asin, '') || ' ' ||
			` + activeFileSearchTextSQL + `
		)`
		for _, token := range searchTokens(searchQuery) {
			conditions = append(conditions, searchText+" LIKE ?")
			args = append(args, "%"+token+"%")
		}
	}

	// Author filter - searches in JSON authors array
	if author != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			for _, value := range queryValues("author", false) {
				addAuthorFilterCondition(add, "bm.authors", value)
			}
		})
	}

	// Series filter
	if series != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			for _, value := range queryValues("series", false) {
				add("COALESCE(bm.series, '') = ?", value)
			}
		})
	}

	// Genre filter
	if genre != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			for _, value := range queryValues("genre", true) {
				addTagOrGenreFilterCondition(add, value)
			}
		})
	}

	// Tags filter
	if tags != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			for _, value := range queryValues("tags", true) {
				addTagOrGenreFilterCondition(add, value)
			}
		})
	}

	if format != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			for _, value := range queryValues("format", false) {
				add("EXISTS (SELECT 1 FROM book_file filter_bf WHERE filter_bf.book_id = b.id AND filter_bf.missing_at IS NULL AND LOWER(filter_bf.format) = ?)", strings.ToLower(value))
			}
		})
	}

	// Publisher filter
	if publisher != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			add("COALESCE(bm.publisher, '') = ?", publisher)
		})
	}

	// Language filter
	if language != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			add("COALESCE(bm.language, '') = ?", language)
		})
	}

	// Publication date filter (exact match on pub_date field)
	if pubDate != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			add("COALESCE(bm.pub_date, '') = ?", pubDate)
		})
	}

	if filterCondition, filterArgs := combineFilterGroups(filterGroups, filterMode); filterCondition != "" {
		conditions = append(conditions, filterCondition)
		args = append(args, filterArgs...)
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	query := `
		SELECT b.id, b.library_id, b.added_at,
		       COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.series, '') as series,
		       COALESCE(bm.series_number, 0) as series_number,
		       COALESCE(bm.series_number_display, '') as series_number_display,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(bm.cover_updated_on, 0) as cover_updated_on,
		       COALESCE(rp.status, 'unread') as status,
		       COALESCE(rp.percent, 0) as percent,
		       CASE WHEN rp.book_id IS NOT NULL THEN 1 ELSE 0 END as opened,
		       COALESCE(rp.updated_at, 0) as last_read_at,
		       COALESCE(bf.format, '') as format`
	query += baseQuery

	var total int
	if includeTotal {
		countQuery := "SELECT COUNT(*) " + baseQuery
		countArgs := make([]interface{}, len(args))
		copy(countArgs, args)
		err := appDB.QueryRow(countQuery, countArgs...).Scan(&total)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to count books")
			return
		}
	}

	orderBy := bookListOrderBy(sortBy, sortDir)

	queryLimit := limit
	if !includeTotal {
		queryLimit = limit + 1
	}
	query += " ORDER BY " + orderBy + " LIMIT ? OFFSET ?"
	args = append(args, queryLimit, offset)

	rows, err := appDB.Query(query, args...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch books")
		return
	}
	defer rows.Close()

	type BookResponse struct {
		ID                  int64   `json:"id"`
		LibraryID           int64   `json:"library_id"`
		AddedAt             int64   `json:"added_at"`
		Title               string  `json:"title"`
		Authors             string  `json:"authors"`
		Series              string  `json:"series,omitempty"`
		SeriesNumber        float64 `json:"series_number,omitempty"`
		SeriesNumberDisplay string  `json:"series_number_display,omitempty"`
		CoverPath           string  `json:"cover_path"`
		CoverUpdatedOn      int64   `json:"cover_updated_on"`
		Status              string  `json:"status"`
		Percent             float64 `json:"percent"`
		Opened              bool    `json:"opened"`
		LastReadAt          int64   `json:"last_read_at"`
		Format              string  `json:"format"`
	}

	type BooksResponse struct {
		Books   []BookResponse `json:"books"`
		Total   *int           `json:"total,omitempty"`
		Offset  int            `json:"offset"`
		Limit   int            `json:"limit"`
		HasMore bool           `json:"has_more"`
	}

	books := []BookResponse{}
	for rows.Next() {
		var b BookResponse
		var opened int
		if err := rows.Scan(&b.ID, &b.LibraryID, &b.AddedAt, &b.Title, &b.Authors, &b.Series, &b.SeriesNumber, &b.SeriesNumberDisplay, &b.CoverPath, &b.CoverUpdatedOn, &b.Status, &b.Percent, &opened, &b.LastReadAt, &b.Format); err != nil {
			continue
		}
		b.Opened = opened == 1
		books = append(books, b)
	}

	hasMore := false
	if includeTotal {
		hasMore = offset+len(books) < total
	} else if len(books) > limit {
		hasMore = true
		books = books[:limit]
	}

	var totalPtr *int
	if includeTotal {
		totalPtr = &total
	}

	jsonResponse(w, http.StatusOK, BooksResponse{
		Books:   books,
		Total:   totalPtr,
		Offset:  offset,
		Limit:   limit,
		HasMore: hasMore,
	})
}

type bookListQuery struct {
	baseQuery string
	args      []interface{}
	orderBy   string
}

func buildBookListQuery(r *http.Request, current *AppUser) bookListQuery {
	libraryID := r.URL.Query().Get("library_id")
	status := r.URL.Query().Get("status")
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	author := r.URL.Query().Get("author")
	series := r.URL.Query().Get("series")
	genre := r.URL.Query().Get("genre")
	tags := r.URL.Query().Get("tags")
	format := r.URL.Query().Get("format")
	publisher := r.URL.Query().Get("publisher")
	language := r.URL.Query().Get("language")
	pubDate := r.URL.Query().Get("pub_date")
	filterMode := strings.ToUpper(r.URL.Query().Get("filter_mode"))
	valueFilterMode := r.URL.Query().Get("value_filter_mode")
	sortBy := r.URL.Query().Get("sort")
	sortDir := strings.ToLower(r.URL.Query().Get("sort_dir"))
	discoveryOnly := r.URL.Query().Get("discovery") == "true"
	filterMode = normalizeAcrossFilterMode(filterMode)

	baseQuery := `
		FROM book b
		LEFT JOIN library l ON b.library_id = l.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		LEFT JOIN (
			SELECT book_id, MIN(format) AS format
			FROM book_file
			WHERE missing_at IS NULL
			GROUP BY book_id
		) bf ON b.id = bf.book_id`

	var args []interface{}
	var conditions []string
	var filterGroups []sqlFilterGroup

	queryValues := func(key string, splitComma bool) []string {
		values := r.URL.Query()[key]
		if len(values) == 0 {
			values = []string{r.URL.Query().Get(key)}
		}
		var cleaned []string
		for _, raw := range values {
			if splitComma {
				for _, value := range strings.Split(raw, ",") {
					if trimmed := strings.TrimSpace(value); trimmed != "" {
						cleaned = append(cleaned, trimmed)
					}
				}
				continue
			}
			if trimmed := strings.TrimSpace(raw); trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		return cleaned
	}

	addFilterGroup := func(build func(add func(string, ...interface{}))) {
		if group, ok := buildFilterGroup(valueFilterMode, build); ok {
			filterGroups = append(filterGroups, group)
		}
	}

	if libraryID != "" {
		conditions = append(conditions, "b.library_id = ?")
		args = append(args, libraryID)
	}

	if current != nil && !userCanAccessAllData(current) {
		conditions = append(conditions, "l.owner_user_id = ?")
		args = append(args, current.ID)
	}
	conditions = append(conditions, "EXISTS (SELECT 1 FROM book_file active_bf WHERE active_bf.book_id = b.id AND active_bf.missing_at IS NULL)")
	if discoveryOnly {
		conditions = append(conditions, "COALESCE(l.exclude_from_suggestions, 0) = 0")
	}

	if status != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			for _, value := range queryValues("status", false) {
				add("COALESCE(rp.status, 'unread') = ?", value)
			}
		})
	}

	if searchQuery != "" {
		searchText := `LOWER(
			COALESCE(bm.title, '') || ' ' ||
			COALESCE(bm.authors, '') || ' ' ||
			REPLACE(REPLACE(COALESCE(bm.authors, ''), '.', ''), ' ', '') || ' ' ||
			COALESCE(bm.description, '') || ' ' ||
			COALESCE(bm.series, '') || ' ' ||
			COALESCE(bm.series_number_display, '') || ' ' ||
			CASE WHEN COALESCE(bm.series_number, 0) != 0 THEN CAST(bm.series_number AS TEXT) ELSE '' END || ' ' ||
			COALESCE(bm.isbn, '') || ' ' ||
			COALESCE(bm.asin, '') || ' ' ||
			` + activeFileSearchTextSQL + `
		)`
		for _, token := range searchTokens(searchQuery) {
			conditions = append(conditions, searchText+" LIKE ?")
			args = append(args, "%"+token+"%")
		}
	}

	if author != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			for _, value := range queryValues("author", false) {
				addAuthorFilterCondition(add, "bm.authors", value)
			}
		})
	}

	if series != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			for _, value := range queryValues("series", false) {
				add("COALESCE(bm.series, '') = ?", value)
			}
		})
	}

	if genre != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			for _, value := range queryValues("genre", true) {
				addTagOrGenreFilterCondition(add, value)
			}
		})
	}

	if tags != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			for _, value := range queryValues("tags", true) {
				addTagOrGenreFilterCondition(add, value)
			}
		})
	}

	if format != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			for _, value := range queryValues("format", false) {
				add("EXISTS (SELECT 1 FROM book_file filter_bf WHERE filter_bf.book_id = b.id AND filter_bf.missing_at IS NULL AND LOWER(filter_bf.format) = ?)", strings.ToLower(value))
			}
		})
	}

	if publisher != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			add("COALESCE(bm.publisher, '') = ?", publisher)
		})
	}

	if language != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			add("COALESCE(bm.language, '') = ?", language)
		})
	}

	if pubDate != "" {
		addFilterGroup(func(add func(string, ...interface{})) {
			add("COALESCE(bm.pub_date, '') = ?", pubDate)
		})
	}

	if filterCondition, filterArgs := combineFilterGroups(filterGroups, filterMode); filterCondition != "" {
		conditions = append(conditions, filterCondition)
		args = append(args, filterArgs...)
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	orderBy := bookListOrderBy(sortBy, sortDir)

	return bookListQuery{baseQuery: baseQuery, args: args, orderBy: orderBy}
}

func bookListOrderBy(sortBy, sortDir string) string {
	dir := "ASC"
	if strings.EqualFold(sortDir, "desc") {
		dir = "DESC"
	}

	titleSort := titleSortSQL()
	orderBy := titleSort + " " + dir + ", b.id " + dir
	switch sortBy {
	case "random":
		orderBy = "RANDOM()"
	case "authors":
		orderBy = authorsSortSQL() + " " + dir + ", " + titleSort + " ASC, b.id ASC"
	case "added_at":
		orderBy = "b.added_at " + dir + ", b.id " + dir
	case "last_read":
		orderBy = "COALESCE(rp.updated_at, 0) " + dir + ", " + titleSort + " ASC, b.id ASC"
	case "series":
		orderBy = "CASE WHEN COALESCE(bm.series, '') = '' THEN 1 ELSE 0 END ASC, " +
			seriesSortSQL() + " " + dir + ", " +
			"CASE WHEN COALESCE(bm.series_number, 0) = 0 THEN 1 ELSE 0 END ASC, " +
			"bm.series_number " + dir + ", " + titleSort + " ASC, b.id ASC"
	}
	return orderBy
}

func getBookNavigationHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	currentID, err := strconv.ParseInt(r.URL.Query().Get("current_id"), 10, 64)
	if err != nil || currentID <= 0 {
		errorResponse(w, http.StatusBadRequest, "Invalid current book ID")
		return
	}

	direction := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("direction")))
	if direction != "previous" && direction != "next" {
		errorResponse(w, http.StatusBadRequest, "Invalid navigation direction")
		return
	}
	if strings.EqualFold(r.URL.Query().Get("sort"), "random") {
		errorResponse(w, http.StatusBadRequest, "Random order cannot be navigated beyond the loaded snapshot")
		return
	}

	listQuery := buildBookListQuery(r, current)
	rows, err := appDB.Query("SELECT b.id "+listQuery.baseQuery+" ORDER BY "+listQuery.orderBy, listQuery.args...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to resolve book navigation")
		return
	}
	defer rows.Close()

	bookIDs := make([]int64, 0)
	currentIndex := -1
	for rows.Next() {
		var bookID int64
		if err := rows.Scan(&bookID); err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to read book navigation")
			return
		}
		if bookID == currentID {
			currentIndex = len(bookIDs)
		}
		bookIDs = append(bookIDs, bookID)
	}
	if err := rows.Err(); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to read book navigation")
		return
	}
	if currentIndex < 0 {
		errorResponse(w, http.StatusNotFound, "Current book is not in this navigation context")
		return
	}

	targetIndex := currentIndex
	if direction == "previous" {
		targetIndex--
	} else {
		targetIndex++
	}

	type navigationResponse struct {
		BookID *int64 `json:"book_id"`
		Index  int    `json:"index"`
		Total  int    `json:"total"`
	}

	if targetIndex < 0 || targetIndex >= len(bookIDs) {
		jsonResponse(w, http.StatusOK, navigationResponse{BookID: nil, Index: currentIndex, Total: len(bookIDs)})
		return
	}

	targetID := bookIDs[targetIndex]
	jsonResponse(w, http.StatusOK, navigationResponse{BookID: &targetID, Index: targetIndex, Total: len(bookIDs)})
}

type BookDetail struct {
	ID                  int64    `json:"id"`
	LibraryID           int64    `json:"library_id"`
	LibraryName         string   `json:"library_name"`
	AddedAt             int64    `json:"added_at"`
	Title               string   `json:"title"`
	Authors             string   `json:"authors"`
	Series              string   `json:"series"`
	SeriesNumber        float64  `json:"series_number"`
	SeriesNumberDisplay string   `json:"series_number_display"`
	Publisher           string   `json:"publisher"`
	PubDate             string   `json:"pub_date"`
	Description         string   `json:"description"`
	CoverPath           string   `json:"cover_path"`
	CoverSource         string   `json:"cover_source"`
	CoverUpdatedOn      int64    `json:"cover_updated_on"`
	Rating              float64  `json:"rating"`
	Genres              string   `json:"genres"`
	Tags                string   `json:"tags"`
	ISBN                string   `json:"isbn"`
	ASIN                string   `json:"asin"`
	Language            string   `json:"language"`
	PageCount           int      `json:"page_count"`
	ComicSpreadFallback string   `json:"comic_spread_fallback"`
	Status              string   `json:"status"`
	Percent             float64  `json:"percent"`
	SpeedReaderPercent  float64  `json:"speed_reader_percent"`
	Opened              bool     `json:"opened"`
	LibraryPaths        []string `json:"library_paths"`
}

func fetchBookDetail(bookID int64) (BookDetail, error) {
	var book BookDetail
	var opened int
	err := appDB.QueryRow(`
		SELECT b.id, b.library_id, b.added_at,
		       COALESCE(l.name, '') as library_name,
		       COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.series, '') as series,
		       COALESCE(bm.series_number, 0) as series_number,
		       COALESCE(bm.series_number_display, '') as series_number_display,
		       COALESCE(bm.publisher, '') as publisher,
		       COALESCE(bm.pub_date, '') as pub_date,
		       COALESCE(bm.description, '') as description,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(bm.cover_source, '') as cover_source,
		       COALESCE(bm.cover_updated_on, 0) as cover_updated_on,
		       COALESCE(bm.rating, 0) as rating,
		       COALESCE(bm.genres, '[]') as genres,
		       COALESCE(bm.tags, '[]') as tags,
		       COALESCE(bm.isbn, '') as isbn,
		       COALESCE(bm.asin, '') as asin,
		       COALESCE(bm.language, '') as language,
		       COALESCE(bm.page_count, 0) as page_count,
		       COALESCE(bm.comic_spread_fallback, 'inherit') as comic_spread_fallback,
		       COALESCE(rp.status, 'unread') as status,
		       COALESCE(rp.percent, 0) as percent,
		       COALESCE(rp.speed_reader_percent, 0) as speed_reader_percent,
		       CASE WHEN rp.book_id IS NOT NULL THEN 1 ELSE 0 END as opened
		FROM book b
		LEFT JOIN library l ON b.library_id = l.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		WHERE b.id = ?
	`, bookID).Scan(
		&book.ID, &book.LibraryID, &book.AddedAt, &book.LibraryName,
		&book.Title, &book.Authors, &book.Series, &book.SeriesNumber,
		&book.SeriesNumberDisplay, &book.Publisher, &book.PubDate, &book.Description, &book.CoverPath, &book.CoverSource,
		&book.CoverUpdatedOn, &book.Rating, &book.Genres, &book.Tags, &book.ISBN, &book.ASIN, &book.Language, &book.PageCount, &book.ComicSpreadFallback,
		&book.Status, &book.Percent, &book.SpeedReaderPercent, &opened,
	)

	if err != nil {
		return book, err
	}

	book.Tags = mergeMetadataTagJSON(book.Genres, book.Tags)

	pathRows, err := appDB.Query(`SELECT path FROM library_path WHERE library_id = ? ORDER BY length(path) DESC`, book.LibraryID)
	if err == nil {
		defer pathRows.Close()
		for pathRows.Next() {
			var libraryPath string
			if scanErr := pathRows.Scan(&libraryPath); scanErr == nil && strings.TrimSpace(libraryPath) != "" {
				book.LibraryPaths = append(book.LibraryPaths, libraryPath)
			}
		}
	} else {
		slog.Warn("Failed to fetch library paths for book detail", "book_id", bookID, "library_id", book.LibraryID, "error", err)
	}

	return book, nil
}

func getBookHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	book, err := fetchBookDetail(bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Book not found")
		return
	}

	jsonResponse(w, http.StatusOK, book)
}

// updateBookHandler updates book metadata
func updateBookHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req struct {
		Title               string          `json:"title"`
		Authors             []string        `json:"authors"`
		Series              string          `json:"series"`
		SeriesNumber        json.RawMessage `json:"series_number"`
		Publisher           string          `json:"publisher"`
		PubDate             string          `json:"pub_date"`
		Description         string          `json:"description"`
		Rating              float64         `json:"rating"`
		Status              string          `json:"status"`
		Genres              []string        `json:"genres"`
		Tags                []string        `json:"tags"`
		ISBN                string          `json:"isbn"`
		ASIN                string          `json:"asin"`
		Language            string          `json:"language"`
		PageCount           int             `json:"page_count"`
		ComicSpreadFallback *string         `json:"comic_spread_fallback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	seriesNumber, seriesNumberDisplay, err := seriesnum.ParseJSON(req.SeriesNumber)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	req.Authors = normalizeMetadataStringList(req.Authors)
	req.Genres = normalizeMetadataStringList(req.Genres)
	req.Tags = mergeMetadataTagLists(req.Genres, req.Tags)

	authorsJSON, _ := json.Marshal(req.Authors)
	genresJSON, _ := json.Marshal(req.Genres)
	tagsJSON, _ := json.Marshal(req.Tags)
	comicSpreadFallback := coverprefs.ComicSpreadFallbackInherit
	updateComicSpreadFallback := 0
	if req.ComicSpreadFallback != nil {
		comicSpreadFallback = coverprefs.NormalizeComicSpreadFallback(*req.ComicSpreadFallback, true)
		updateComicSpreadFallback = 1
	}

	// Upsert metadata
	_, err = appDB.Exec(`
		INSERT INTO book_metadata (book_id, title, authors, series, series_number, series_number_display, publisher, pub_date,
		                           description, rating, genres, tags, isbn, asin, language, page_count, comic_spread_fallback, owner_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(book_id) DO UPDATE SET
		    title = excluded.title,
		    authors = excluded.authors,
		    series = excluded.series,
		    series_number = excluded.series_number,
		    series_number_display = excluded.series_number_display,
		    publisher = excluded.publisher,
		    pub_date = excluded.pub_date,
		    description = excluded.description,
		    rating = excluded.rating,
		    genres = excluded.genres,
		    tags = excluded.tags,
		    isbn = excluded.isbn,
		    asin = excluded.asin,
		    language = excluded.language,
		    page_count = excluded.page_count,
		    comic_spread_fallback = CASE WHEN ? = 1 THEN excluded.comic_spread_fallback ELSE comic_spread_fallback END,
		    owner_user_id = excluded.owner_user_id
	`, bookID, req.Title, string(authorsJSON), req.Series, seriesNumber, seriesNumberDisplay, req.Publisher, req.PubDate,
		req.Description, req.Rating, string(genresJSON), string(tagsJSON), req.ISBN, req.ASIN, req.Language, req.PageCount,
		comicSpreadFallback, current.ID, updateComicSpreadFallback)

	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update book metadata")
		return
	}

	// Update reading status if provided
	if req.Status != "" {
		bookIDInt, _ := strconv.ParseInt(bookID, 10, 64)
		_, err = appDB.Exec(`
			INSERT INTO reading_progress (book_id, status, percent, updated_at, owner_user_id)
			VALUES (?, ?, 0, ?, ?)
			ON CONFLICT(book_id) DO UPDATE SET status = excluded.status, updated_at = excluded.updated_at, owner_user_id = excluded.owner_user_id
		`, bookIDInt, req.Status, time.Now().Unix(), current.ID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to update reading status")
			return
		}
	}

	updatedBook, err := fetchBookDetail(bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load updated book metadata")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{"status": "ok", "book": updatedBook})
}

func updateBookStatusHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Status != "unread" && req.Status != "reading" && req.Status != "finished" {
		errorResponse(w, http.StatusBadRequest, "Invalid reading status")
		return
	}

	now := time.Now().Unix()
	_, err = appDB.Exec(`
		INSERT INTO reading_progress (book_id, status, percent, updated_at, owner_user_id)
		VALUES (?, ?, 0, ?, ?)
		ON CONFLICT(book_id) DO UPDATE SET
			status = excluded.status,
			updated_at = excluded.updated_at,
			owner_user_id = excluded.owner_user_id
	`, bookIDInt, req.Status, now, current.ID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update reading status")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":     req.Status,
		"updated_at": now,
	})
}

// deleteBookHandler deletes a book and all associated data
func deleteBookHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	// Get cover path before deleting
	var coverPath string
	appDB.QueryRow("SELECT COALESCE(cover_path, '') FROM book_metadata WHERE book_id = ?", bookID).Scan(&coverPath)

	// Cascade delete in dependency order
	for _, stmt := range []string{
		"DELETE FROM book_shelf WHERE book_id = ?",
		"DELETE FROM annotation WHERE book_id = ?",
		"DELETE FROM bookmark WHERE book_id = ?",
		"DELETE FROM reading_session WHERE book_id = ?",
		"DELETE FROM reading_progress WHERE book_id = ?",
		"DELETE FROM notebook_entry WHERE book_id = ?",
		"DELETE FROM bookdrop_file WHERE id IN (SELECT id FROM bookdrop_file WHERE path IN (SELECT path FROM book_file WHERE book_id = ?))",
		"DELETE FROM book_metadata WHERE book_id = ?",
		"DELETE FROM book_file WHERE book_id = ?",
		"DELETE FROM book WHERE id = ?",
	} {
		if _, err := appDB.Exec(stmt, bookID); err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to delete book")
			return
		}
	}

	// Remove cover file if present (best effort)
	if coverPath != "" {
		os.Remove(coverPath)
	}

	w.WriteHeader(http.StatusNoContent)
}

// bulkDeleteBooksHandler deletes multiple books
func bulkDeleteBooksHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	var req struct {
		BookIDs []int64 `json:"book_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.BookIDs) == 0 {
		errorResponse(w, http.StatusBadRequest, "Invalid request: book_ids array is required")
		return
	}

	for _, bookID := range req.BookIDs {
		allowed, err := canAccessBook(current, bookID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
			return
		}
		if !allowed {
			continue
		}

		// Get cover path before deleting
		var coverPath string
		appDB.QueryRow("SELECT COALESCE(cover_path, '') FROM book_metadata WHERE book_id = ?", bookID).Scan(&coverPath)

		// Cascade delete in dependency order
		for _, stmt := range []string{
			"DELETE FROM book_shelf WHERE book_id = ?",
			"DELETE FROM annotation WHERE book_id = ?",
			"DELETE FROM bookmark WHERE book_id = ?",
			"DELETE FROM reading_session WHERE book_id = ?",
			"DELETE FROM reading_progress WHERE book_id = ?",
			"DELETE FROM notebook_entry WHERE book_id = ?",
			"DELETE FROM bookdrop_file WHERE id IN (SELECT id FROM bookdrop_file WHERE path IN (SELECT path FROM book_file WHERE book_id = ?))",
			"DELETE FROM book_metadata WHERE book_id = ?",
			"DELETE FROM book_file WHERE book_id = ?",
			"DELETE FROM book WHERE id = ?",
		} {
			appDB.Exec(stmt, bookID)
		}

		// Remove cover file if present (best effort)
		if coverPath != "" {
			os.Remove(coverPath)
		}
	}

	jsonResponse(w, http.StatusOK, map[string]int{"deleted": len(req.BookIDs)})
}

// bulkDeleteByFilterHandler deletes books matching filter criteria
func bulkDeleteByFilterHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	var req bulkFilterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// Build query to find matching book IDs
	filterQuery, filterArgs := buildBulkFilterQuery(current, req)

	rows, err := appDB.Query(filterQuery, filterArgs...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to find books")
		return
	}
	defer rows.Close()

	var bookIDs []int64
	for rows.Next() {
		var bookID int64
		if err := rows.Scan(&bookID); err == nil {
			bookIDs = append(bookIDs, bookID)
		}
	}

	deleted := 0
	for _, bookID := range bookIDs {
		var coverPath string
		appDB.QueryRow("SELECT COALESCE(cover_path, '') FROM book_metadata WHERE book_id = ?", bookID).Scan(&coverPath)

		for _, stmt := range []string{
			"DELETE FROM book_shelf WHERE book_id = ?",
			"DELETE FROM annotation WHERE book_id = ?",
			"DELETE FROM bookmark WHERE book_id = ?",
			"DELETE FROM reading_session WHERE book_id = ?",
			"DELETE FROM reading_progress WHERE book_id = ?",
			"DELETE FROM notebook_entry WHERE book_id = ?",
			"DELETE FROM book_metadata WHERE book_id = ?",
			"DELETE FROM book_file WHERE book_id = ?",
			"DELETE FROM book WHERE id = ?",
		} {
			appDB.Exec(stmt, bookID)
		}

		if coverPath != "" {
			os.Remove(coverPath)
		}
		deleted++
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{"deleted": deleted, "filter_applied": true})
}

// bulkAddToShelfHandler adds multiple books to a shelf
func bulkAddToShelfHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	shelfID := chi.URLParam(r, "shelfID")
	allowed, err := canAccessShelf(current, mustInt64(shelfID))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify shelf access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req struct {
		BookIDs []int64 `json:"book_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.BookIDs) == 0 {
		errorResponse(w, http.StatusBadRequest, "Invalid request: book_ids array is required")
		return
	}

	added := 0
	for _, bookID := range req.BookIDs {
		bookAllowed, err := canAccessBook(current, bookID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
			return
		}
		if !bookAllowed {
			continue
		}

		result, err := appDB.Exec(`
			INSERT OR IGNORE INTO book_shelf (book_id, shelf_id) VALUES (?, ?)
		`, bookID, shelfID)
		if err == nil {
			affected, _ := result.RowsAffected()
			added += int(affected)
		}
	}

	jsonResponse(w, http.StatusOK, map[string]int{"added": added})
}

// bulkAddToShelfByFilterHandler adds all books matching filter to a shelf
func bulkAddToShelfByFilterHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	shelfID := chi.URLParam(r, "shelfID")
	allowed, err := canAccessShelf(current, mustInt64(shelfID))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify shelf access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req bulkFilterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// Build query to find matching book IDs
	filterQuery, filterArgs := buildBulkFilterQuery(current, req)

	rows, err := appDB.Query(filterQuery, filterArgs...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to find books")
		return
	}
	defer rows.Close()

	added := 0
	for rows.Next() {
		var bookID int64
		if err := rows.Scan(&bookID); err == nil {
			result, err := appDB.Exec(`
				INSERT OR IGNORE INTO book_shelf (book_id, shelf_id) VALUES (?, ?)
			`, bookID, shelfID)
			if err == nil {
				affected, _ := result.RowsAffected()
				added += int(affected)
			}
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{"added": added, "filter_applied": true})
}

func getBookFilesHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	current := getUserFromContext(r.Context())
	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	rows, err := appDB.Query(`
		SELECT id, path, format, size, hash, last_modified
		FROM book_file WHERE book_id = ?
	`, bookID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch files")
		return
	}
	defer rows.Close()

	type FileResponse struct {
		ID           int64  `json:"id"`
		Path         string `json:"path"`
		Format       string `json:"format"`
		Size         int64  `json:"size"`
		Hash         string `json:"hash"`
		LastModified int64  `json:"last_modified"`
	}

	files := []FileResponse{}
	for rows.Next() {
		var f FileResponse
		if err := rows.Scan(&f.ID, &f.Path, &f.Format, &f.Size, &f.Hash, &f.LastModified); err != nil {
			continue
		}
		files = append(files, f)
	}

	jsonResponse(w, http.StatusOK, files)
}

// getSimilarBooksHandler returns similar books using hierarchical fallback matching
func getSimilarBooksHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	current := getUserFromContext(r.Context())
	limitStr := r.URL.Query().Get("limit")
	limit := 6
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 20 {
			limit = parsedLimit
		}
	}

	var targetTitle, targetGenres, targetTags, targetAuthors, targetPublisher, targetSeries string
	var targetLibraryID *int64
	err := appDB.QueryRow(`
		SELECT COALESCE(bm.title, ''), COALESCE(bm.genres, '[]'), COALESCE(bm.tags, '[]'), 
		       COALESCE(bm.authors, '[]'), COALESCE(bm.publisher, ''), COALESCE(bm.series, ''),
		       b.library_id
		FROM book b
		JOIN book_metadata bm ON b.id = bm.book_id
		WHERE b.id = ?
	`, bookID).Scan(&targetTitle, &targetGenres, &targetTags, &targetAuthors, &targetPublisher, &targetSeries, &targetLibraryID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Book not found")
		return
	}

	var targetGenreList, targetTagList, targetAuthorList []string
	json.Unmarshal([]byte(targetGenres), &targetGenreList)
	json.Unmarshal([]byte(targetTags), &targetTagList)
	json.Unmarshal([]byte(targetAuthors), &targetAuthorList)

	type scoredBook struct {
		ID             int64
		Title          string
		Authors        string
		CoverPath      string
		CoverUpdatedOn int64
		Format         string
		Score          int
		MatchType      string
	}

	genreParts := make(map[string]bool)
	for _, g := range targetGenreList {
		parts := strings.Split(g, ".")
		for i := range parts {
			genreParts[strings.Join(parts[:i+1], ".")] = true
		}
	}

	rows, err := appDB.Query(`
		SELECT b.id, COALESCE(bm.title, ''), COALESCE(bm.authors, '[]'), COALESCE(bm.cover_path, ''),
		       COALESCE(bm.cover_updated_on, 0),
		       COALESCE(bm.genres, '[]'), COALESCE(bm.tags, '[]'), COALESCE(bm.publisher, ''),
		       (SELECT bf.format FROM book_file bf WHERE bf.book_id = b.id ORDER BY bf.format ASC LIMIT 1) as format
		FROM book b
		JOIN library l ON b.library_id = l.id
		JOIN book_metadata bm ON b.id = bm.book_id
		WHERE b.id != ?
		  AND COALESCE(l.exclude_from_suggestions, 0) = 0
		  AND `+func() string { clause, _ := userOwnershipClause(current, "l"); return clause }()+`
	`, append([]interface{}{bookID}, func() []interface{} { _, args := userOwnershipClause(current, "l"); return args }()...)...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch books")
		return
	}
	defer rows.Close()

	tagMatches := []scoredBook{}
	genreMatches := []scoredBook{}
	authorMatches := []scoredBook{}
	publisherMatches := []scoredBook{}
	similarNameMatches := []scoredBook{}
	libraryMatches := []scoredBook{}
	allBooks := []scoredBook{}

	titleWords := strings.Fields(strings.ToLower(targetTitle))

	for rows.Next() {
		var id int64
		var title, authors, coverPath, genres, tags, publisher, format string
		var coverUpdatedOn int64
		if err := rows.Scan(&id, &title, &authors, &coverPath, &coverUpdatedOn, &genres, &tags, &publisher, &format); err != nil {
			continue
		}

		var bookGenreList, bookTagList, bookAuthorList []string
		json.Unmarshal([]byte(genres), &bookGenreList)
		json.Unmarshal([]byte(tags), &bookTagList)
		json.Unmarshal([]byte(authors), &bookAuthorList)

		score := 0
		matchType := ""

		for _, tag := range bookTagList {
			for _, targetTag := range targetTagList {
				if strings.EqualFold(strings.TrimSpace(tag), strings.TrimSpace(targetTag)) {
					score += 30
					matchType = "tag"
					break
				}
			}
		}

		bookGenrePartSet := make(map[string]bool)
		for _, g := range bookGenreList {
			parts := strings.Split(g, ".")
			for i := range parts {
				bookGenrePartSet[strings.Join(parts[:i+1], ".")] = true
			}
		}
		bestGenreMatch := 0
		for gp := range bookGenrePartSet {
			if genreParts[gp] {
				matchScore := 20
				if bestGenreMatch < matchScore {
					bestGenreMatch = matchScore
					if matchType == "" {
						matchType = "genre"
					}
				}
				score += matchScore
			}
		}

		for _, author := range bookAuthorList {
			for _, targetAuthor := range targetAuthorList {
				if authorNamesMatch(author, targetAuthor) {
					score += 50
					if matchType == "" {
						matchType = "author"
					}
					break
				}
			}
		}

		if targetPublisher != "" && strings.EqualFold(strings.TrimSpace(publisher), strings.TrimSpace(targetPublisher)) {
			score += 10
			if matchType == "" {
				matchType = "publisher"
			}
		}

		titleLower := strings.ToLower(title)
		titleWordCount := 0
		for _, word := range titleWords {
			if len(word) > 3 && strings.Contains(titleLower, word) {
				titleWordCount++
			}
		}
		if titleWordCount >= 2 {
			score += 15
			if matchType == "" {
				matchType = "title"
			}
		}

		allBooks = append(allBooks, scoredBook{
			ID:             id,
			Title:          title,
			Authors:        authors,
			CoverPath:      coverPath,
			CoverUpdatedOn: coverUpdatedOn,
			Format:         format,
			Score:          score,
			MatchType:      matchType,
		})

		if score >= 30 {
			tagMatches = append(tagMatches, scoredBook{
				ID: id, Title: title, Authors: authors, CoverPath: coverPath, CoverUpdatedOn: coverUpdatedOn, Format: format, Score: score, MatchType: matchType,
			})
		}
		if score >= 20 {
			genreMatches = append(genreMatches, scoredBook{
				ID: id, Title: title, Authors: authors, CoverPath: coverPath, CoverUpdatedOn: coverUpdatedOn, Format: format, Score: score, MatchType: matchType,
			})
		}
		if score >= 50 {
			authorMatches = append(authorMatches, scoredBook{
				ID: id, Title: title, Authors: authors, CoverPath: coverPath, CoverUpdatedOn: coverUpdatedOn, Format: format, Score: score, MatchType: matchType,
			})
		}
		if score >= 10 {
			publisherMatches = append(publisherMatches, scoredBook{
				ID: id, Title: title, Authors: authors, CoverPath: coverPath, CoverUpdatedOn: coverUpdatedOn, Format: format, Score: score, MatchType: matchType,
			})
		}
		if score >= 15 {
			similarNameMatches = append(similarNameMatches, scoredBook{
				ID: id, Title: title, Authors: authors, CoverPath: coverPath, CoverUpdatedOn: coverUpdatedOn, Format: format, Score: score, MatchType: matchType,
			})
		}
	}

	sort.Slice(tagMatches, func(i, j int) bool {
		if tagMatches[i].Score == tagMatches[j].Score {
			return tagMatches[i].ID < tagMatches[j].ID
		}
		return tagMatches[i].Score > tagMatches[j].Score
	})
	sort.Slice(genreMatches, func(i, j int) bool {
		if genreMatches[i].Score == genreMatches[j].Score {
			return genreMatches[i].ID < genreMatches[j].ID
		}
		return genreMatches[i].Score > genreMatches[j].Score
	})
	sort.Slice(authorMatches, func(i, j int) bool {
		if authorMatches[i].Score == authorMatches[j].Score {
			return authorMatches[i].ID < authorMatches[j].ID
		}
		return authorMatches[i].Score > authorMatches[j].Score
	})
	sort.Slice(publisherMatches, func(i, j int) bool {
		if publisherMatches[i].Score == publisherMatches[j].Score {
			return publisherMatches[i].ID < publisherMatches[j].ID
		}
		return publisherMatches[i].Score > publisherMatches[j].Score
	})
	sort.Slice(similarNameMatches, func(i, j int) bool {
		if similarNameMatches[i].Score == similarNameMatches[j].Score {
			return similarNameMatches[i].ID < similarNameMatches[j].ID
		}
		return similarNameMatches[i].Score > similarNameMatches[j].Score
	})
	sort.Slice(allBooks, func(i, j int) bool {
		if allBooks[i].Score == allBooks[j].Score {
			return allBooks[i].ID < allBooks[j].ID
		}
		return allBooks[i].Score > allBooks[j].Score
	})

	seen := make(map[int64]bool)
	result := []scoredBook{}

	addToResult := func(books []scoredBook, needed int) int {
		count := 0
		for _, book := range books {
			if !seen[book.ID] && count < needed {
				result = append(result, book)
				seen[book.ID] = true
				count++
			}
		}
		return count
	}

	remaining := limit
	remaining -= addToResult(tagMatches, remaining)
	remaining -= addToResult(genreMatches, remaining)
	remaining -= addToResult(authorMatches, remaining)
	remaining -= addToResult(publisherMatches, remaining)
	remaining -= addToResult(similarNameMatches, remaining)
	remaining -= addToResult(libraryMatches, remaining)
	remaining -= addToResult(allBooks, remaining)

	type SimilarBook struct {
		ID             int64  `json:"id"`
		Title          string `json:"title"`
		Authors        string `json:"authors"`
		CoverPath      string `json:"cover_path"`
		CoverUpdatedOn int64  `json:"cover_updated_on"`
		Format         string `json:"format"`
		Score          int    `json:"score"`
		MatchType      string `json:"match_type"`
	}

	similarResult := make([]SimilarBook, len(result))
	for i, c := range result {
		similarResult[i] = SimilarBook{
			ID:             c.ID,
			Title:          c.Title,
			Authors:        c.Authors,
			CoverPath:      c.CoverPath,
			CoverUpdatedOn: c.CoverUpdatedOn,
			Format:         c.Format,
			Score:          c.Score,
			MatchType:      c.MatchType,
		}
	}

	jsonResponse(w, http.StatusOK, similarResult)
}

// Library handlers
func getLibrariesHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	rows, err := appDB.Query(`
		SELECT l.id, l.name, COALESCE(l.icon, '') as icon,
		       COALESCE(l.exclude_from_suggestions, 0) as exclude_from_suggestions,
		       COALESCE(l.comic_spread_fallback, 'inherit') as comic_spread_fallback,
		       COALESCE(l.sort_order, 0) as sort_order,
		       COUNT(DISTINCT CASE WHEN bf.id IS NOT NULL THEN b.id END) as book_count
		FROM library l
		LEFT JOIN book b ON l.id = b.library_id
		LEFT JOIN book_file bf ON bf.book_id = b.id AND bf.missing_at IS NULL
		WHERE `+ownerClause+`
		GROUP BY l.id, l.name, l.icon, l.exclude_from_suggestions, l.comic_spread_fallback, l.sort_order
		ORDER BY CASE WHEN COALESCE(l.sort_order, 0) = 0 THEN 1 ELSE 0 END, l.sort_order, l.name
	`, ownerArgs...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch libraries")
		return
	}
	defer rows.Close()

	type LibraryResponse struct {
		ID                     int64  `json:"id"`
		Name                   string `json:"name"`
		Icon                   string `json:"icon"`
		ExcludeFromSuggestions bool   `json:"exclude_from_suggestions"`
		ComicSpreadFallback    string `json:"comic_spread_fallback"`
		SortOrder              int64  `json:"sort_order"`
		BookCount              int64  `json:"book_count"`
		IsImporting            bool   `json:"is_importing"`
	}

	libraries := []LibraryResponse{}
	for rows.Next() {
		var lib LibraryResponse
		var excludeFromSuggestions int
		if err := rows.Scan(&lib.ID, &lib.Name, &lib.Icon, &excludeFromSuggestions, &lib.ComicSpreadFallback, &lib.SortOrder, &lib.BookCount); err != nil {
			continue
		}
		lib.ExcludeFromSuggestions = excludeFromSuggestions == 1
		lib.IsImporting = isLibraryScanning(lib.ID)
		libraries = append(libraries, lib)
	}

	jsonResponse(w, http.StatusOK, libraries)
}

func getLibraryHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	libraryID := chi.URLParam(r, "libraryID")
	ownerClause, ownerArgs := userOwnershipClause(current, "l")

	type LibraryResponse struct {
		ID                     int64    `json:"id"`
		Name                   string   `json:"name"`
		Icon                   string   `json:"icon"`
		ExcludeFromSuggestions bool     `json:"exclude_from_suggestions"`
		ComicSpreadFallback    string   `json:"comic_spread_fallback"`
		BookCount              int64    `json:"book_count"`
		Paths                  []string `json:"paths"`
	}

	var lib LibraryResponse
	query := `
		SELECT l.id, l.name, COALESCE(l.icon, '') as icon,
		       COALESCE(l.exclude_from_suggestions, 0) as exclude_from_suggestions,
		       COALESCE(l.comic_spread_fallback, 'inherit') as comic_spread_fallback,
		       COUNT(DISTINCT CASE WHEN bf.id IS NOT NULL THEN b.id END) as book_count
		FROM library l
		LEFT JOIN book b ON l.id = b.library_id
		LEFT JOIN book_file bf ON bf.book_id = b.id AND bf.missing_at IS NULL
		WHERE l.id = ? AND ` + ownerClause + `
		GROUP BY l.id, l.exclude_from_suggestions, l.comic_spread_fallback
	`
	var excludeFromSuggestions int
	err := appDB.QueryRow(query, append([]interface{}{libraryID}, ownerArgs...)...).Scan(&lib.ID, &lib.Name, &lib.Icon, &excludeFromSuggestions, &lib.ComicSpreadFallback, &lib.BookCount)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Library not found")
		return
	}
	lib.ExcludeFromSuggestions = excludeFromSuggestions == 1

	rows, _ := appDB.Query(`
		SELECT lp.path
		FROM library_path lp
		JOIN library l ON lp.library_id = l.id
		WHERE lp.library_id = ? AND `+ownerClause+`
	`, append([]interface{}{libraryID}, ownerArgs...)...)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var p string
			rows.Scan(&p)
			lib.Paths = append(lib.Paths, p)
		}
	}

	jsonResponse(w, http.StatusOK, lib)
}

func getLibraryBooksHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	libraryID := chi.URLParam(r, "libraryID")
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 200
	offset := (page - 1) * limit

	rows, err := appDB.Query(`
		SELECT b.id, b.library_id, b.added_at,
		       COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(rp.status, 'unread') as status,
		       COALESCE(rp.percent, 0) as percent
		FROM book b
		JOIN library l ON b.library_id = l.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		WHERE b.library_id = ? AND `+ownerClause+`
		  AND EXISTS (SELECT 1 FROM book_file bf WHERE bf.book_id = b.id AND bf.missing_at IS NULL)
		ORDER BY b.added_at DESC
		LIMIT ? OFFSET ?
	`, append([]interface{}{libraryID}, append(ownerArgs, limit, offset)...)...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch books")
		return
	}
	defer rows.Close()

	type BookResponse struct {
		ID        int64   `json:"id"`
		LibraryID int64   `json:"library_id"`
		AddedAt   int64   `json:"added_at"`
		Title     string  `json:"title"`
		Authors   string  `json:"authors"`
		CoverPath string  `json:"cover_path"`
		Status    string  `json:"status"`
		Percent   float64 `json:"percent"`
	}

	books := []BookResponse{}
	for rows.Next() {
		var b BookResponse
		if err := rows.Scan(&b.ID, &b.LibraryID, &b.AddedAt, &b.Title, &b.Authors, &b.CoverPath, &b.Status, &b.Percent); err != nil {
			continue
		}
		books = append(books, b)
	}

	jsonResponse(w, http.StatusOK, books)
}

func getFirstAvailableLibraryID(database *db.DB) (int64, error) {
	rows, err := database.Query(`SELECT id FROM library ORDER BY id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	used := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		used[id] = true
	}

	for i := int64(1); ; i++ {
		if !used[i] {
			return i, nil
		}
	}
}

func isLibraryScanning(libraryID int64) bool {
	scanningLibrariesMu.RLock()
	scanning := scanningLibraries[libraryID]
	scanningLibrariesMu.RUnlock()
	if scanning {
		return true
	}
	return hasActiveLibraryScanJob(libraryID)
}

func markLibraryScanning(libraryID int64, scanning bool) {
	scanningLibrariesMu.Lock()
	defer scanningLibrariesMu.Unlock()
	if scanning {
		scanningLibraries[libraryID] = true
		return
	}
	delete(scanningLibraries, libraryID)
}

func hasActiveLibraryScanJob(libraryID int64) bool {
	_, ok := activeLibraryScanJobForLibrary(libraryID)
	return ok
}

func libraryScanDedupeKey(libraryID int64) string {
	return fmt.Sprintf("library_scan:library:%d", libraryID)
}

func activeLibraryScanJobForLibrary(libraryID int64) (int64, bool) {
	if appDB == nil {
		return 0, false
	}

	var jobID int64
	err := appDB.QueryRow(`
		SELECT id
		FROM metadata_job
		WHERE dedupe_key = ?
		  AND status IN ('queued', 'running', 'cancelling')
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, libraryScanDedupeKey(libraryID)).Scan(&jobID)
	if err == nil {
		return jobID, true
	}

	rows, err := appDB.Query(`
		SELECT id, COALESCE(payload_json, '')
		FROM metadata_job
		WHERE job_type = ? AND status IN ('queued', 'running', 'cancelling')
		ORDER BY created_at DESC
		LIMIT 100
	`, "library_scan")
	if err != nil {
		return 0, false
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var payloadRaw string
		if err := rows.Scan(&id, &payloadRaw); err != nil || strings.TrimSpace(payloadRaw) == "" {
			continue
		}
		var payload struct {
			LibraryID int64 `json:"library_id"`
		}
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err == nil && payload.LibraryID == libraryID {
			return id, true
		}
	}
	return 0, false
}

func queueLibraryScan(libraryID int64, paths []string) (int64, bool, bool, error) {
	return queueLibraryScanWithWorker(libraryID, paths, true)
}

func queueLibraryScanWithWorker(libraryID int64, paths []string, startWorker bool) (int64, bool, bool, error) {
	if jobID, ok := activeLibraryScanJobForLibrary(libraryID); ok || isLibraryScanning(libraryID) {
		return jobID, false, ok, nil
	}

	var libraryName string
	if err := appDB.QueryRow(`SELECT name FROM library WHERE id = ?`, libraryID).Scan(&libraryName); err != nil {
		libraryName = fmt.Sprintf("Library %d", libraryID)
	}

	now := time.Now().Unix()
	title := fmt.Sprintf("Scan library: %s", libraryName)
	payload, _ := json.Marshal(map[string]any{
		"library_id":   libraryID,
		"library_name": libraryName,
		"paths":        paths,
	})
	dedupeKey := libraryScanDedupeKey(libraryID)
	jobResult, err := appDB.Exec(`
		INSERT OR IGNORE INTO metadata_job (
			job_type, title, status, payload_json,
			total_items, completed_items, failed_items, created_at, dedupe_key
		) VALUES (?, ?, ?, ?, 0, 0, 0, ?, ?)
	`, "library_scan", title, "queued", nullString(payload), now, dedupeKey)
	if err != nil {
		return 0, false, false, err
	}
	if affected, _ := jobResult.RowsAffected(); affected == 0 {
		if jobID, ok := activeLibraryScanJobForLibrary(libraryID); ok {
			return jobID, false, true, nil
		}
		return 0, false, false, fmt.Errorf("scan job already exists but could not be loaded")
	}
	jobID, _ := jobResult.LastInsertId()
	createAdminNotification(
		"library_scan_queued",
		title,
		"Queued a library scan.",
		"/settings?tab=jobs",
	)
	recordAppLog("info", "library", "Queued library scan", map[string]any{
		"job_id":     jobID,
		"library_id": libraryID,
	})

	if startWorker {
		signalLibraryScanWorker()
	}

	return jobID, true, false, nil
}

func requeueInterruptedLibraryScans() {
	if appDB == nil {
		return
	}
	now := time.Now().Unix()
	_, cancelErr := appDB.Exec(`
		UPDATE metadata_job
		SET status = 'cancelled', completed_at = COALESCE(completed_at, ?)
		WHERE job_type = 'library_scan'
		  AND status = 'cancelling'
		  AND completed_at IS NULL
	`, now)
	if cancelErr != nil {
		slog.Warn("Failed to finalize interrupted cancelling library scans", "error", cancelErr)
	}
	_, err := appDB.Exec(`
		UPDATE metadata_job
		SET status = 'queued', started_at = NULL
		WHERE job_type = 'library_scan'
		  AND status = 'running'
		  AND completed_at IS NULL
	`)
	if err != nil {
		slog.Warn("Failed to requeue interrupted library scans", "error", err)
	}
}

func signalLibraryScanWorker() {
	if appDB == nil || appScanner == nil {
		return
	}
	if !libraryScanWorker.CompareAndSwap(false, true) {
		return
	}
	go runLibraryScanWorker()
}

func runLibraryScanWorker() {
	defer func() {
		libraryScanWorker.Store(false)
		if hasQueuedLibraryScanJobs() {
			signalLibraryScanWorker()
		}
	}()

	for {
		jobID, libraryID, libraryName, paths, title, ok := claimNextLibraryScanJob()
		if !ok {
			return
		}
		processLibraryScanJob(jobID, libraryID, libraryName, paths, title)
	}
}

func hasQueuedLibraryScanJobs() bool {
	var exists bool
	if err := appDB.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM metadata_job
			WHERE job_type = ? AND status = ?
		)
	`, "library_scan", "queued").Scan(&exists); err != nil {
		return false
	}
	return exists
}

func claimNextLibraryScanJob() (int64, int64, string, []string, string, bool) {
	for {
		var jobID int64
		var title, payloadRaw string
		err := appDB.QueryRow(`
			SELECT id, title, COALESCE(payload_json, '')
			FROM metadata_job
			WHERE job_type = ? AND status = ?
			ORDER BY created_at, id
			LIMIT 1
		`, "library_scan", "queued").Scan(&jobID, &title, &payloadRaw)
		if err == sql.ErrNoRows {
			return 0, 0, "", nil, "", false
		}
		if err != nil {
			slog.Warn("Failed to load queued library scan", "error", err)
			return 0, 0, "", nil, "", false
		}

		startedAt := time.Now().Unix()
		result, err := appDB.Exec(`
			UPDATE metadata_job
			SET status = ?, started_at = ?
			WHERE id = ? AND status = ?
		`, "running", startedAt, jobID, "queued")
		if err != nil {
			slog.Warn("Failed to claim queued library scan", "job_id", jobID, "error", err)
			return 0, 0, "", nil, "", false
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			continue
		}

		var payload struct {
			LibraryID int64    `json:"library_id"`
			Paths     []string `json:"paths"`
		}
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil || payload.LibraryID == 0 || len(payload.Paths) == 0 {
			completedAt := time.Now().Unix()
			_, _ = appDB.Exec(`
				UPDATE metadata_job
				SET status = ?, error = ?, completed_at = ?
				WHERE id = ?
			`, "failed", "Invalid library scan payload", completedAt, jobID)
			continue
		}

		var libraryName string
		if err := appDB.QueryRow(`SELECT name FROM library WHERE id = ?`, payload.LibraryID).Scan(&libraryName); err != nil {
			libraryName = fmt.Sprintf("Library %d", payload.LibraryID)
		}

		return jobID, payload.LibraryID, libraryName, payload.Paths, title, true
	}
}

func processLibraryScanJob(jobID, libraryID int64, libraryName string, paths []string, title string) {
	markLibraryScanning(libraryID, true)
	defer markLibraryScanning(libraryID, false)

	type libraryScanJobItem struct {
		Path   string `json:"path"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}

	lastJobUpdate := time.Time{}
	scanItems := []libraryScanJobItem{}
	statusCounts := map[string]int{}
	lastRecordedItemKey := ""
	recordScanItem := func(progress scanner.ScanProgress) {
		if progress.CurrentPath == "" || progress.CurrentStatus == "" {
			return
		}
		key := progress.CurrentPath + "\x00" + progress.CurrentStatus + "\x00" + progress.CurrentError
		if key == lastRecordedItemKey {
			return
		}
		lastRecordedItemKey = key
		statusCounts[progress.CurrentStatus]++
		scanItems = append(scanItems, libraryScanJobItem{
			Path:   progress.CurrentPath,
			Status: progress.CurrentStatus,
			Error:  progress.CurrentError,
		})
	}
	buildResultPayload := func(progress scanner.ScanProgress, includeAllItems bool) []byte {
		recentItems := scanItems
		if len(recentItems) > 50 {
			recentItems = recentItems[len(recentItems)-50:]
		}
		payload := map[string]any{
			"library_id":      libraryID,
			"library_name":    libraryName,
			"imported_books":  progress.ImportedBooks,
			"scanned_files":   progress.ScannedFiles,
			"failed_files":    progress.FailedFiles,
			"total_files":     progress.TotalFiles,
			"current_path":    progress.CurrentPath,
			"unchanged_files": progress.UnchangedFiles,
			"missing_files":   progress.MissingFiles,
			"changed_files":   progress.ChangedFiles,
			"status_counts":   statusCounts,
			"recent_items":    recentItems,
			"phase":           progress.Phase,
		}
		if includeAllItems {
			payload["items"] = scanItems
		}
		resultPayload, _ := json.Marshal(payload)
		return resultPayload
	}
	var lastProgress scanner.ScanProgress
	updateProgress := func(progress scanner.ScanProgress) {
		lastProgress = progress
		recordScanItem(progress)
		if time.Since(lastJobUpdate) < time.Second &&
			progress.ScannedFiles < progress.TotalFiles {
			return
		}
		lastJobUpdate = time.Now()
		resultPayload := buildResultPayload(progress, false)
		_, _ = appDB.Exec(`
			UPDATE metadata_job
			SET total_items = ?, completed_items = ?, failed_items = ?, result_json = ?
			WHERE id = ?
		`, progress.TotalFiles, progress.ScannedFiles, progress.FailedFiles, nullString(resultPayload), jobID)
	}

	cancelRequested := false
	lastCancelCheck := time.Time{}
	shouldCancel := func() bool {
		if cancelRequested {
			return true
		}
		if time.Since(lastCancelCheck) < time.Second {
			return false
		}
		lastCancelCheck = time.Now()
		cancelRequested = isJobCancelRequested(jobID)
		return cancelRequested
	}
	imported, err := appScanner.ScanLibraryWithProgressAndCancel(libraryID, paths, updateProgress, shouldCancel)
	completedAt := time.Now().Unix()
	if err == scanner.ErrScanCancelled {
		resultPayload := buildResultPayload(lastProgress, true)
		_, _ = appDB.Exec(`
			UPDATE metadata_job
			SET status = ?, result_json = ?, completed_at = ?
			WHERE id = ?
		`, "cancelled", nullString(resultPayload), completedAt, jobID)
		createAdminNotification(
			"library_scan_cancelled",
			title,
			fmt.Sprintf("%s scan was cancelled.", libraryName),
			"/settings?tab=jobs",
		)
		recordAppLog("info", "library", "Library scan cancelled", map[string]any{
			"job_id":     jobID,
			"library_id": libraryID,
			"imported":   imported,
		})
		return
	}
	if err != nil {
		resultPayload := buildResultPayload(lastProgress, true)
		_, _ = appDB.Exec(`
			UPDATE metadata_job
			SET status = ?, result_json = ?, error = ?, completed_at = ?
			WHERE id = ?
		`, "failed", nullString(resultPayload), err.Error(), completedAt, jobID)
		recordAppLog("error", "library", "Library scan failed", map[string]any{
			"job_id":     jobID,
			"library_id": libraryID,
			"error":      err.Error(),
		})
		createAdminNotification(
			"library_scan_failed",
			"Library scan failed",
			fmt.Sprintf("%s (ID %d) scan failed: %s", libraryName, libraryID, err.Error()),
			"/settings?tab=jobs",
		)
		return
	}

	resultPayload := buildResultPayload(lastProgress, true)
	_, _ = appDB.Exec(`
		UPDATE metadata_job
		SET status = ?, result_json = ?, completed_at = ?
		WHERE id = ?
	`, "completed", nullString(resultPayload), completedAt, jobID)
	createAdminNotification(
		"library_scan_completed",
		title,
		fmt.Sprintf("%s scan finished: %d new books imported.", libraryName, imported),
		"/settings?tab=jobs",
	)
	recordAppLog("info", "library", "Library scan completed", map[string]any{
		"job_id":     jobID,
		"library_id": libraryID,
		"imported":   imported,
	})
}

func createLibraryHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageLibraries) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	if current == nil {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	var req struct {
		Name                   string   `json:"name"`
		Icon                   string   `json:"icon"`
		ExcludeFromSuggestions bool     `json:"exclude_from_suggestions"`
		ComicSpreadFallback    string   `json:"comic_spread_fallback"`
		Paths                  []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		errorResponse(w, http.StatusBadRequest, "Invalid request: name is required")
		return
	}

	libraryID, err := getFirstAvailableLibraryID(appDB)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to find available ID")
		return
	}

	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to start transaction")
		return
	}
	defer tx.Rollback()

	comicSpreadFallback := coverprefs.NormalizeComicSpreadFallback(req.ComicSpreadFallback, true)
	var nextSortOrder int64
	_ = tx.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM library WHERE owner_user_id = ?`, current.ID).Scan(&nextSortOrder)
	result, err := tx.Exec(`INSERT INTO library (id, name, icon, owner_user_id, exclude_from_suggestions, comic_spread_fallback, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?)`, libraryID, req.Name, req.Icon, current.ID, boolToInt(req.ExcludeFromSuggestions), comicSpreadFallback, nextSortOrder)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to create library")
		return
	}

	_ = result

	for _, path := range req.Paths {
		_, err = tx.Exec(`INSERT INTO library_path (library_id, path) VALUES (?, ?)`, libraryID, path)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to add path")
			return
		}
	}

	if err = tx.Commit(); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to commit transaction")
		return
	}

	scanJobID, scanQueued, scanExisting, scanErr := queueLibraryScan(libraryID, req.Paths)
	if scanErr != nil {
		slog.Warn("Failed to queue initial library scan", "libraryID", libraryID, "error", scanErr)
	}

	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"id":                       libraryID,
		"name":                     req.Name,
		"icon":                     req.Icon,
		"exclude_from_suggestions": req.ExcludeFromSuggestions,
		"comic_spread_fallback":    comicSpreadFallback,
		"paths":                    req.Paths,
		"scan_job_id":              scanJobID,
		"scan_queued":              scanQueued,
		"scan_existing":            scanExisting,
	})
}

func updateLibraryOrderHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageLibraries) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	if current == nil {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var req struct {
		LibraryIDs []int64 `json:"library_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.LibraryIDs) == 0 {
		errorResponse(w, http.StatusBadRequest, "Invalid library order")
		return
	}

	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to start transaction")
		return
	}
	defer tx.Rollback()

	for index, id := range req.LibraryIDs {
		result, err := tx.Exec(`UPDATE library SET sort_order = ? WHERE id = ? AND owner_user_id = ?`, index+1, id, current.ID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to update library order")
			return
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			errorResponse(w, http.StatusForbidden, "Permission denied")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to save library order")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func updateLibraryHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageLibraries) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	libraryID := chi.URLParam(r, "libraryID")
	if !userCanAccessAllData(current) {
		var exists bool
		if err := appDB.QueryRow(`SELECT EXISTS(SELECT 1 FROM library WHERE id = ? AND owner_user_id = ?)`, libraryID, current.ID).Scan(&exists); err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to verify library ownership")
			return
		}
		if !exists {
			errorResponse(w, http.StatusForbidden, "Permission denied")
			return
		}
	}

	var req struct {
		Name                   string   `json:"name"`
		Icon                   string   `json:"icon"`
		ExcludeFromSuggestions bool     `json:"exclude_from_suggestions"`
		ComicSpreadFallback    string   `json:"comic_spread_fallback"`
		Paths                  []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		errorResponse(w, http.StatusBadRequest, "Invalid request: name is required")
		return
	}

	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to start transaction")
		return
	}
	defer tx.Rollback()

	comicSpreadFallback := coverprefs.NormalizeComicSpreadFallback(req.ComicSpreadFallback, true)
	_, err = tx.Exec(`UPDATE library SET name = ?, icon = ?, exclude_from_suggestions = ?, comic_spread_fallback = ? WHERE id = ?`, req.Name, req.Icon, boolToInt(req.ExcludeFromSuggestions), comicSpreadFallback, libraryID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update library")
		return
	}

	_, err = tx.Exec(`DELETE FROM library_path WHERE library_id = ?`, libraryID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to clear paths")
		return
	}

	for _, path := range req.Paths {
		_, err = tx.Exec(`INSERT INTO library_path (library_id, path) VALUES (?, ?)`, libraryID, path)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to add path")
			return
		}
	}

	if err = tx.Commit(); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to commit transaction")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func deleteLibraryHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageLibraries) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	libraryID := chi.URLParam(r, "libraryID")
	if !userCanAccessAllData(current) {
		var exists bool
		if err := appDB.QueryRow(`SELECT EXISTS(SELECT 1 FROM library WHERE id = ? AND owner_user_id = ?)`, libraryID, current.ID).Scan(&exists); err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to verify library ownership")
			return
		}
		if !exists {
			errorResponse(w, http.StatusForbidden, "Permission denied")
			return
		}
	}

	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to start transaction")
		return
	}
	defer tx.Rollback()

	// First get all book IDs for this library so we can clean up related data
	rows, err := tx.Query(`SELECT id FROM book WHERE library_id = ?`, libraryID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch books")
		return
	}

	var bookIDs []int64
	for rows.Next() {
		var bookID int64
		if err := rows.Scan(&bookID); err == nil {
			bookIDs = append(bookIDs, bookID)
		}
	}
	rows.Close()

	// Delete related data for each book (order matters due to foreign keys)
	for _, bookID := range bookIDs {
		_, err = tx.Exec(`DELETE FROM reading_progress WHERE book_id = ?`, bookID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to delete reading progress")
			return
		}

		_, err = tx.Exec(`DELETE FROM reading_session WHERE book_id = ?`, bookID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to delete reading sessions")
			return
		}

		_, err = tx.Exec(`DELETE FROM book_shelf WHERE book_id = ?`, bookID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to delete book shelves")
			return
		}

		_, err = tx.Exec(`DELETE FROM book_metadata WHERE book_id = ?`, bookID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to delete book metadata")
			return
		}

		_, err = tx.Exec(`DELETE FROM book_file WHERE book_id = ?`, bookID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to delete book files")
			return
		}
	}

	// Delete all books for this library
	_, err = tx.Exec(`DELETE FROM book WHERE library_id = ?`, libraryID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to delete books")
		return
	}

	// Delete library paths
	_, err = tx.Exec(`DELETE FROM library_path WHERE library_id = ?`, libraryID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to delete paths")
		return
	}

	// Delete the library itself
	_, err = tx.Exec(`DELETE FROM library WHERE id = ? AND owner_user_id = ?`, libraryID, current.ID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to delete library")
		return
	}

	if err = tx.Commit(); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to commit transaction")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func scanLibraryHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageLibraries) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	libraryIDStr := chi.URLParam(r, "libraryID")
	libraryID, err := strconv.ParseInt(libraryIDStr, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid library ID")
		return
	}
	if !userCanAccessAllData(current) {
		var exists bool
		if err := appDB.QueryRow(`SELECT EXISTS(SELECT 1 FROM library WHERE id = ? AND owner_user_id = ?)`, libraryID, current.ID).Scan(&exists); err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to verify library ownership")
			return
		}
		if !exists {
			errorResponse(w, http.StatusForbidden, "Permission denied")
			return
		}
	}

	var paths []string
	rows, err := appDB.Query(`SELECT path FROM library_path WHERE library_id = ?`, libraryID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch library paths")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var path string
		rows.Scan(&path)
		paths = append(paths, path)
	}

	if len(paths) == 0 {
		errorResponse(w, http.StatusBadRequest, "No paths configured for this library")
		return
	}

	jobID, queued, existing, err := queueLibraryScan(libraryID, paths)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to create scan job")
		return
	}
	if !queued {
		jsonResponse(w, http.StatusAccepted, map[string]any{
			"status":   "scanning",
			"job_id":   jobID,
			"existing": existing,
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"status": "scanning",
		"job_id": jobID,
	})
}

func getDirectoriesHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	// For security, restrict to certain base paths when not starting from root
	allowedBasePaths := []string{"/books", "/bookdrop", "/data"}
	if path != "/" {
		allowed := false
		for _, basePath := range allowedBasePaths {
			if strings.HasPrefix(path, basePath) || strings.HasPrefix(path+"/", basePath+"/") {
				allowed = true
				break
			}
		}
		if !allowed {
			// Check if it's a subdirectory of any allowed path
			for _, basePath := range allowedBasePaths {
				if strings.HasPrefix(path, basePath) {
					allowed = true
					break
				}
			}
		}
		if !allowed && path != "/" {
			errorResponse(w, http.StatusForbidden, "Access denied to this directory")
			return
		}
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to read directory")
		return
	}

	type DirectoryEntry struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Path string `json:"path"`
	}

	var result []DirectoryEntry

	// Add parent directory entry if not at root
	if path != "/" {
		parentPath := filepath.Dir(path)
		if parentPath != path { // Avoid infinite loop
			result = append(result, DirectoryEntry{
				Name: "..",
				Type: "directory",
				Path: parentPath,
			})
		}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		fullPath := filepath.Join(path, entry.Name())
		result = append(result, DirectoryEntry{
			Name: entry.Name(),
			Type: "directory",
			Path: fullPath,
		})
	}

	jsonResponse(w, http.StatusOK, result)
}

// searchBooksHandler combines FTS5 token matching with typo-tolerant scoring.
func searchBooksHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	query := r.URL.Query().Get("q")
	libraryID := r.URL.Query().Get("library_id")

	limit := searchDefaultPageLimit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}
	offset, limit = normalizeSearchPagination(offset, limit)

	if strings.TrimSpace(query) == "" {
		jsonResponse(w, http.StatusOK, SearchResultsPage{
			Results: []SearchResult{},
			Offset:  offset,
			Limit:   limit,
		})
		return
	}

	filters := BookSearchFilters{
		Author:          searchQueryValues(r, "author", false),
		Series:          searchQueryValues(r, "series", false),
		Genre:           searchQueryValues(r, "genre", true),
		Tags:            searchQueryValues(r, "tags", true),
		Status:          searchQueryValues(r, "status", false),
		Format:          searchQueryValues(r, "format", false),
		FilterMode:      r.URL.Query().Get("filter_mode"),
		ValueFilterMode: r.URL.Query().Get("value_filter_mode"),
		Sort:            r.URL.Query().Get("sort"),
		SortDir:         r.URL.Query().Get("sort_dir"),
	}
	results, err := searchBooks(query, libraryID, current, filters, offset, limit)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Search failed")
		return
	}

	jsonResponse(w, http.StatusOK, results)
}

func searchQueryValues(r *http.Request, key string, splitComma bool) []string {
	values := r.URL.Query()[key]
	if len(values) == 0 {
		values = []string{r.URL.Query().Get(key)}
	}
	cleaned := []string{}
	for _, raw := range values {
		if splitComma {
			for _, value := range strings.Split(raw, ",") {
				if trimmed := strings.TrimSpace(value); trimmed != "" {
					cleaned = append(cleaned, trimmed)
				}
			}
			continue
		}
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

type metadataOption struct {
	Name      string `json:"name"`
	BookCount int64  `json:"book_count"`
}

func sortedMetadataOptions(counts map[string]int64) []metadataOption {
	items := make([]metadataOption, 0, len(counts))
	for name, count := range counts {
		if strings.TrimSpace(name) == "" {
			continue
		}
		items = append(items, metadataOption{Name: name, BookCount: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].BookCount == items[j].BookCount {
			return items[i].Name < items[j].Name
		}
		return items[i].BookCount > items[j].BookCount
	})
	return items
}

func getJSONMetadataOptions(column string, hierarchical bool, current *AppUser) ([]metadataOption, error) {
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	rows, err := appDB.Query(fmt.Sprintf(`
		SELECT bm.%s, COUNT(*) as book_count
		FROM book_metadata bm
		JOIN book b ON bm.book_id = b.id
		JOIN library l ON b.library_id = l.id
		WHERE %s AND bm.%s IS NOT NULL AND bm.%s != '[]' AND bm.%s != ''
		GROUP BY bm.%s
	`, column, ownerClause, column, column, column, column), ownerArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var jsonStr string
		var bookCount int64
		if err := rows.Scan(&jsonStr, &bookCount); err != nil {
			continue
		}

		var values []string
		if err := json.Unmarshal([]byte(jsonStr), &values); err != nil {
			continue
		}

		prefixesInRow := make(map[string]bool)
		for _, value := range values {
			if !hierarchical {
				value = strings.TrimSpace(value)
				key := normalizedAuthorMatchKey(value)
				if key != "" && !prefixesInRow[key] {
					prefixesInRow[key] = true
					counts[canonicalAuthorOptionName(value)] += bookCount
				}
				continue
			}

			parts := strings.Split(value, ".")
			for i := range parts {
				prefix := strings.TrimSpace(strings.Join(parts[:i+1], "."))
				if prefix == "" || prefixesInRow[prefix] {
					continue
				}
				prefixesInRow[prefix] = true
				counts[prefix] += bookCount
			}
		}
	}

	return sortedMetadataOptions(counts), rows.Err()
}

func getCombinedJSONMetadataOptions(primaryColumn string, legacyColumn string, hierarchical bool, current *AppUser) ([]metadataOption, error) {
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	rows, err := appDB.Query(fmt.Sprintf(`
		SELECT COALESCE(bm.%s, '[]'), COALESCE(bm.%s, '[]')
		FROM book_metadata bm
		JOIN book b ON bm.book_id = b.id
		JOIN library l ON b.library_id = l.id
		WHERE %s
		  AND ((bm.%s IS NOT NULL AND bm.%s != '[]' AND bm.%s != '')
		    OR (bm.%s IS NOT NULL AND bm.%s != '[]' AND bm.%s != ''))
	`, primaryColumn, legacyColumn, ownerClause, primaryColumn, primaryColumn, primaryColumn, legacyColumn, legacyColumn, legacyColumn), ownerArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var primaryJSON string
		var legacyJSON string
		if err := rows.Scan(&primaryJSON, &legacyJSON); err != nil {
			continue
		}

		prefixesInRow := make(map[string]bool)
		for _, value := range mergeMetadataTagLists(parseMetadataJSONList(primaryJSON), parseMetadataJSONList(legacyJSON)) {
			if !hierarchical {
				value = strings.TrimSpace(value)
				if value != "" && !prefixesInRow[value] {
					prefixesInRow[value] = true
					counts[value]++
				}
				continue
			}

			parts := strings.Split(value, ".")
			for i := range parts {
				prefix := strings.TrimSpace(strings.Join(parts[:i+1], "."))
				if prefix == "" || prefixesInRow[prefix] {
					continue
				}
				prefixesInRow[prefix] = true
				counts[prefix]++
			}
		}
	}

	return sortedMetadataOptions(counts), rows.Err()
}

func getScalarMetadataOptions(column string, current *AppUser) ([]metadataOption, error) {
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	rows, err := appDB.Query(fmt.Sprintf(`
		SELECT bm.%s, COUNT(*) as book_count
		FROM book_metadata bm
		JOIN book b ON bm.book_id = b.id
		JOIN library l ON b.library_id = l.id
		WHERE %s AND bm.%s IS NOT NULL AND bm.%s != ''
		GROUP BY bm.%s
	`, column, ownerClause, column, column, column), ownerArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var name string
		var bookCount int64
		if err := rows.Scan(&name, &bookCount); err != nil {
			continue
		}
		name = strings.TrimSpace(name)
		if name != "" {
			counts[name] += bookCount
		}
	}

	return sortedMetadataOptions(counts), rows.Err()
}

func getFormatMetadataOptions(current *AppUser) ([]metadataOption, error) {
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	rows, err := appDB.Query(`
		SELECT LOWER(bf.format), COUNT(DISTINCT bf.book_id) as book_count
		FROM book_file bf
		JOIN book b ON bf.book_id = b.id
		JOIN library l ON b.library_id = l.id
		WHERE `+ownerClause+` AND bf.missing_at IS NULL AND bf.format IS NOT NULL AND bf.format != ''
		GROUP BY LOWER(bf.format)
	`, ownerArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var name string
		var bookCount int64
		if err := rows.Scan(&name, &bookCount); err != nil {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			counts[name] += bookCount
		}
	}

	return sortedMetadataOptions(counts), rows.Err()
}

func buildScopedBookIDSubquery(r *http.Request, current *AppUser) (string, []interface{}) {
	listQuery := buildBookListQuery(r, current)
	return "SELECT DISTINCT b.id " + listQuery.baseQuery, listQuery.args
}

func getJSONMetadataOptionsForScope(column string, hierarchical bool, scopedBookIDsSQL string, scopedArgs []interface{}) ([]metadataOption, error) {
	rows, err := appDB.Query(fmt.Sprintf(`
		SELECT bm.%s, COUNT(*) as book_count
		FROM book_metadata bm
		JOIN (%s) scoped_books ON scoped_books.id = bm.book_id
		WHERE bm.%s IS NOT NULL AND bm.%s != '[]' AND bm.%s != ''
		GROUP BY bm.%s
	`, column, scopedBookIDsSQL, column, column, column, column), scopedArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var jsonStr string
		var bookCount int64
		if err := rows.Scan(&jsonStr, &bookCount); err != nil {
			continue
		}

		var values []string
		if err := json.Unmarshal([]byte(jsonStr), &values); err != nil {
			continue
		}

		prefixesInRow := make(map[string]bool)
		for _, value := range values {
			if !hierarchical {
				value = strings.TrimSpace(value)
				key := normalizedAuthorMatchKey(value)
				if key != "" && !prefixesInRow[key] {
					prefixesInRow[key] = true
					counts[canonicalAuthorOptionName(value)] += bookCount
				}
				continue
			}

			parts := strings.Split(value, ".")
			for i := range parts {
				prefix := strings.TrimSpace(strings.Join(parts[:i+1], "."))
				if prefix == "" || prefixesInRow[prefix] {
					continue
				}
				prefixesInRow[prefix] = true
				counts[prefix] += bookCount
			}
		}
	}

	return sortedMetadataOptions(counts), rows.Err()
}

func getCombinedJSONMetadataOptionsForScope(primaryColumn string, legacyColumn string, hierarchical bool, scopedBookIDsSQL string, scopedArgs []interface{}) ([]metadataOption, error) {
	rows, err := appDB.Query(fmt.Sprintf(`
		SELECT COALESCE(bm.%s, '[]'), COALESCE(bm.%s, '[]')
		FROM book_metadata bm
		JOIN (%s) scoped_books ON scoped_books.id = bm.book_id
		WHERE (bm.%s IS NOT NULL AND bm.%s != '[]' AND bm.%s != '')
		   OR (bm.%s IS NOT NULL AND bm.%s != '[]' AND bm.%s != '')
	`, primaryColumn, legacyColumn, scopedBookIDsSQL, primaryColumn, primaryColumn, primaryColumn, legacyColumn, legacyColumn, legacyColumn), scopedArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var primaryJSON string
		var legacyJSON string
		if err := rows.Scan(&primaryJSON, &legacyJSON); err != nil {
			continue
		}

		prefixesInRow := make(map[string]bool)
		for _, value := range mergeMetadataTagLists(parseMetadataJSONList(primaryJSON), parseMetadataJSONList(legacyJSON)) {
			if !hierarchical {
				value = strings.TrimSpace(value)
				if value != "" && !prefixesInRow[value] {
					prefixesInRow[value] = true
					counts[value]++
				}
				continue
			}

			parts := strings.Split(value, ".")
			for i := range parts {
				prefix := strings.TrimSpace(strings.Join(parts[:i+1], "."))
				if prefix == "" || prefixesInRow[prefix] {
					continue
				}
				prefixesInRow[prefix] = true
				counts[prefix]++
			}
		}
	}

	return sortedMetadataOptions(counts), rows.Err()
}

func getScalarMetadataOptionsForScope(column string, scopedBookIDsSQL string, scopedArgs []interface{}) ([]metadataOption, error) {
	rows, err := appDB.Query(fmt.Sprintf(`
		SELECT bm.%s, COUNT(*) as book_count
		FROM book_metadata bm
		JOIN (%s) scoped_books ON scoped_books.id = bm.book_id
		WHERE bm.%s IS NOT NULL AND bm.%s != ''
		GROUP BY bm.%s
	`, column, scopedBookIDsSQL, column, column, column), scopedArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var name string
		var bookCount int64
		if err := rows.Scan(&name, &bookCount); err != nil {
			continue
		}
		name = strings.TrimSpace(name)
		if name != "" {
			counts[name] += bookCount
		}
	}

	return sortedMetadataOptions(counts), rows.Err()
}

func getFormatMetadataOptionsForScope(scopedBookIDsSQL string, scopedArgs []interface{}) ([]metadataOption, error) {
	rows, err := appDB.Query(`
		SELECT LOWER(bf.format), COUNT(DISTINCT bf.book_id) as book_count
		FROM book_file bf
		JOIN (`+scopedBookIDsSQL+`) scoped_books ON scoped_books.id = bf.book_id
		WHERE bf.missing_at IS NULL AND bf.format IS NOT NULL AND bf.format != ''
		GROUP BY LOWER(bf.format)
	`, scopedArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var name string
		var bookCount int64
		if err := rows.Scan(&name, &bookCount); err != nil {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			counts[name] += bookCount
		}
	}

	return sortedMetadataOptions(counts), rows.Err()
}

func getFilterOptionsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	scopedBookIDsSQL, scopedArgs := buildScopedBookIDSubquery(r, current)

	authors, err := getJSONMetadataOptionsForScope("authors", false, scopedBookIDsSQL, scopedArgs)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch authors")
		return
	}
	series, err := getScalarMetadataOptionsForScope("series", scopedBookIDsSQL, scopedArgs)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch series")
		return
	}
	genres, err := getJSONMetadataOptionsForScope("genres", true, scopedBookIDsSQL, scopedArgs)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch genres")
		return
	}
	tags, err := getCombinedJSONMetadataOptionsForScope("tags", "genres", true, scopedBookIDsSQL, scopedArgs)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch tags")
		return
	}
	publishers, err := getScalarMetadataOptionsForScope("publisher", scopedBookIDsSQL, scopedArgs)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch publishers")
		return
	}
	languages, err := getScalarMetadataOptionsForScope("language", scopedBookIDsSQL, scopedArgs)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch languages")
		return
	}
	formats, err := getFormatMetadataOptionsForScope(scopedBookIDsSQL, scopedArgs)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch formats")
		return
	}

	jsonResponse(w, http.StatusOK, map[string][]metadataOption{
		"authors":    authors,
		"series":     series,
		"genres":     genres,
		"tags":       tags,
		"formats":    formats,
		"publishers": publishers,
		"languages":  languages,
	})
}

// Authors and Series handlers
func getAuthorsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	authors, err := getJSONMetadataOptions("authors", false, current)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch authors")
		return
	}
	jsonResponse(w, http.StatusOK, authors)
}

func getSeriesHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	series, err := getScalarMetadataOptions("series", current)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch series")
		return
	}
	jsonResponse(w, http.StatusOK, series)
}

// getMetadataHandler returns metadata items for a specific type (authors, series, genres, etc.)
func getMetadataHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	metadataType := chi.URLParam(r, "type")
	if metadataType == "tags" {
		tags, err := getCombinedJSONMetadataOptions("tags", "genres", true, current)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to fetch metadata")
			return
		}
		jsonResponse(w, http.StatusOK, tags)
		return
	}
	ownerClause, ownerArgs := userOwnershipClause(current, "l")

	var query string
	var args []interface{}

	switch metadataType {
	case "authors":
		query = `
			SELECT bm.authors, COUNT(*) as book_count
			FROM book_metadata bm
			JOIN book b ON bm.book_id = b.id
			JOIN library l ON b.library_id = l.id
			WHERE ` + ownerClause + ` AND bm.authors IS NOT NULL AND bm.authors != '[]' AND bm.authors != ''
			GROUP BY bm.authors
			ORDER BY book_count DESC, bm.authors`
	case "series":
		query = `
			SELECT bm.series, COUNT(*) as book_count
			FROM book_metadata bm
			JOIN book b ON bm.book_id = b.id
			JOIN library l ON b.library_id = l.id
			WHERE ` + ownerClause + ` AND bm.series IS NOT NULL AND bm.series != ''
			GROUP BY bm.series
			ORDER BY book_count DESC, bm.series`
	case "genres":
		query = `
			SELECT bm.genres, COUNT(*) as book_count
			FROM book_metadata bm
			JOIN book b ON bm.book_id = b.id
			JOIN library l ON b.library_id = l.id
			WHERE ` + ownerClause + ` AND bm.genres IS NOT NULL AND bm.genres != '[]' AND bm.genres != ''
			GROUP BY bm.genres
			ORDER BY book_count DESC, bm.genres`
	case "publishers":
		query = `
			SELECT bm.publisher, COUNT(*) as book_count
			FROM book_metadata bm
			JOIN book b ON bm.book_id = b.id
			JOIN library l ON b.library_id = l.id
			WHERE ` + ownerClause + ` AND bm.publisher IS NOT NULL AND bm.publisher != ''
			GROUP BY bm.publisher
			ORDER BY book_count DESC, bm.publisher`
	case "languages":
		query = `
			SELECT bm.language, COUNT(*) as book_count
			FROM book_metadata bm
			JOIN book b ON bm.book_id = b.id
			JOIN library l ON b.library_id = l.id
			WHERE ` + ownerClause + ` AND bm.language IS NOT NULL AND bm.language != ''
			GROUP BY bm.language
			ORDER BY book_count DESC, bm.language`
	case "tags":
		query = `
			SELECT bm.tags, COUNT(*) as book_count
			FROM book_metadata bm
			JOIN book b ON bm.book_id = b.id
			JOIN library l ON b.library_id = l.id
			WHERE ` + ownerClause + ` AND bm.tags IS NOT NULL AND bm.tags != '[]' AND bm.tags != ''
			GROUP BY bm.tags
			ORDER BY book_count DESC, bm.tags`
	default:
		errorResponse(w, http.StatusBadRequest, "Invalid metadata type")
		return
	}

	rows, err := appDB.Query(query, append(ownerArgs, args...)...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch metadata")
		return
	}
	defer rows.Close()

	type MetadataResponse struct {
		Name      string `json:"name"`
		BookCount int64  `json:"book_count"`
	}

	metadata := []MetadataResponse{}

	for rows.Next() {
		var name string
		var bookCount int64

		if metadataType == "authors" {
			// For authors, we need to parse JSON arrays
			var jsonStr string
			if err := rows.Scan(&jsonStr, &bookCount); err != nil {
				continue
			}

			var authorList []string
			if err := json.Unmarshal([]byte(jsonStr), &authorList); err != nil {
				continue
			}
			for _, author := range authorList {
				key := normalizedAuthorMatchKey(author)
				if key == "" {
					continue
				}
				found := false
				for i, existing := range metadata {
					if normalizedAuthorMatchKey(existing.Name) == key {
						metadata[i].BookCount += bookCount
						found = true
						break
					}
				}
				if !found {
					metadata = append(metadata, MetadataResponse{
						Name:      canonicalAuthorOptionName(author),
						BookCount: bookCount,
					})
				}
			}
		} else if metadataType == "genres" || metadataType == "tags" {
			var jsonStr string
			if err := rows.Scan(&jsonStr, &bookCount); err != nil {
				continue
			}

			var values []string
			if err := json.Unmarshal([]byte(jsonStr), &values); err != nil {
				continue
			}

			prefixesInRow := make(map[string]bool)
			for _, value := range values {
				parts := strings.Split(value, ".")
				for i := range parts {
					prefix := strings.TrimSpace(strings.Join(parts[:i+1], "."))
					if prefix == "" || prefixesInRow[prefix] {
						continue
					}
					prefixesInRow[prefix] = true
					found := false
					for j, existing := range metadata {
						if existing.Name == prefix {
							metadata[j].BookCount += bookCount
							found = true
							break
						}
					}
					if !found {
						metadata = append(metadata, MetadataResponse{
							Name:      prefix,
							BookCount: bookCount,
						})
					}
				}
			}
		} else {
			// For other types (series, publishers, languages), scan directly
			if err := rows.Scan(&name, &bookCount); err != nil {
				continue
			}
			if name != "" {
				metadata = append(metadata, MetadataResponse{
					Name:      name,
					BookCount: bookCount,
				})
			}
		}
	}

	// Sort by book count descending, then by name
	sort.Slice(metadata, func(i, j int) bool {
		if metadata[i].BookCount == metadata[j].BookCount {
			return metadata[i].Name < metadata[j].Name
		}
		return metadata[i].BookCount > metadata[j].BookCount
	})

	jsonResponse(w, http.StatusOK, metadata)
}

// getMetadataSuggestionsHandler returns distinct values for autocomplete
func getMetadataSuggestionsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	field := r.URL.Query().Get("field")
	if field == "" {
		errorResponse(w, http.StatusBadRequest, "field parameter is required")
		return
	}

	var query string
	switch field {
	case "genres":
		query = `SELECT DISTINCT genres FROM book_metadata WHERE genres IS NOT NULL AND genres != '[]' AND genres != ''`
	case "tags":
		query = `SELECT DISTINCT tags FROM book_metadata WHERE tags IS NOT NULL AND tags != '[]' AND tags != ''`
	default:
		errorResponse(w, http.StatusBadRequest, "Invalid field. Use 'genres' or 'tags'")
		return
	}

	rows, err := appDB.Query(query)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch suggestions")
		return
	}
	defer rows.Close()

	suggestions := make(map[string]bool)
	for rows.Next() {
		var jsonStr string
		if err := rows.Scan(&jsonStr); err != nil {
			continue
		}

		var items []string
		if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
			continue
		}

		for _, item := range items {
			if item != "" {
				suggestions[item] = true
			}
		}
	}

	result := make([]string, 0, len(suggestions))
	for item := range suggestions {
		result = append(result, item)
	}
	sort.Strings(result)

	jsonResponse(w, http.StatusOK, result)
}

// Settings handlers
func getSettingsHandler(w http.ResponseWriter, r *http.Request) {
	// Fetch libraries from DB
	rows, err := appDB.Query(`
		SELECT l.id, l.name, COALESCE(l.icon, '') as icon,
		       COALESCE(l.exclude_from_suggestions, 0) as exclude_from_suggestions,
		       COALESCE(l.comic_spread_fallback, 'inherit') as comic_spread_fallback,
		       COUNT(DISTINCT CASE WHEN bf.id IS NOT NULL THEN b.id END) as book_count
		FROM library l
		LEFT JOIN book b ON l.id = b.library_id
		LEFT JOIN book_file bf ON bf.book_id = b.id AND bf.missing_at IS NULL
		GROUP BY l.id, l.name, l.icon, l.exclude_from_suggestions, l.comic_spread_fallback
		ORDER BY l.name
	`)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch libraries")
		return
	}
	defer rows.Close()

	type LibraryResponse struct {
		ID                     int64    `json:"id"`
		Name                   string   `json:"name"`
		Icon                   string   `json:"icon"`
		ExcludeFromSuggestions bool     `json:"exclude_from_suggestions"`
		ComicSpreadFallback    string   `json:"comic_spread_fallback"`
		BookCount              int64    `json:"book_count"`
		Paths                  []string `json:"paths"`
		IsImporting            bool     `json:"is_importing"`
	}

	libraries := []LibraryResponse{}
	for rows.Next() {
		var lib LibraryResponse
		var excludeFromSuggestions int
		if err := rows.Scan(&lib.ID, &lib.Name, &lib.Icon, &excludeFromSuggestions, &lib.ComicSpreadFallback, &lib.BookCount); err != nil {
			continue
		}
		lib.ExcludeFromSuggestions = excludeFromSuggestions == 1

		// Fetch paths
		paths := []string{}
		pathRows, pathErr := appDB.Query("SELECT path FROM library_path WHERE library_id = ?", lib.ID)
		if pathErr != nil {
			slog.Warn("Failed to fetch library paths for settings", "library_id", lib.ID, "error", pathErr)
			lib.IsImporting = isLibraryScanning(lib.ID)
			lib.Paths = paths
			libraries = append(libraries, lib)
			continue
		}
		for pathRows.Next() {
			var path string
			if err := pathRows.Scan(&path); err == nil {
				paths = append(paths, path)
			}
		}
		pathRows.Close()

		lib.IsImporting = isLibraryScanning(lib.ID)
		lib.Paths = paths
		libraries = append(libraries, lib)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("Settings library rows ended with error", "error", err)
	}

	// Default reader settings
	readerSettings := map[string]interface{}{
		"keepScreenOnWhileReading": true,
		"keepScreenOnWhileAppOpen": false,
		"readerTheme":              "catppuccin",
		"showCurrentSection":       true,
		"settingsUpdatedAt":        int64(0),
		"epub": map[string]interface{}{
			"fontFamily":     "serif",
			"fontSize":       16,
			"lineHeight":     1.5,
			"margin":         20,
			"textAlign":      "justify",
			"theme":          "catppuccin",
			"flow":           "scrolled",
			"continuousMode": true,
		},
		"pdf": map[string]interface{}{
			"pageFit":         "auto",
			"zoomLevel":       100,
			"scrollDirection": "vertical",
			"scrollMode":      "continuous-vertical",
		},
		"cbx": map[string]interface{}{
			"readerMode": "single",
			"direction":  "ltr",
		},
		"audio": map[string]interface{}{
			"playbackSpeed": 1.0,
			"autoAdvance":   false,
		},
		"speedReader": map[string]interface{}{
			"theme": "catppuccin",
		},
	}
	var storedReaderSettings string
	if err := appDB.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, "reader_settings").Scan(&storedReaderSettings); err == nil && strings.TrimSpace(storedReaderSettings) != "" {
		var stored map[string]interface{}
		if err := json.Unmarshal([]byte(storedReaderSettings), &stored); err == nil {
			readerSettings = stored
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"libraries":   libraries,
		"bookdrop":    appConfig.Bookdrop,
		"metadata":    appConfig.Metadata,
		"reader":      readerSettings,
		"book_covers": loadBookCoverSettingsResponse(),
	})
}

func updateReaderSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reader map[string]interface{} `json:"reader"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// For now, we'll store settings in memory or we could save to database
	data, err := json.Marshal(req.Reader)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid reader settings")
		return
	}

	if _, err := appDB.Exec(`
		INSERT OR REPLACE INTO app_settings (key, value) VALUES (?, ?)
	`, "reader_settings", string(data)); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to save reader settings")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func updateBookdropHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageLibraries) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	path := strings.TrimSpace(req.Path)
	if path == "" {
		errorResponse(w, http.StatusBadRequest, "Bookdrop path is required")
		return
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		errorResponse(w, http.StatusBadRequest, "Failed to create bookdrop directory")
		return
	}

	appConfig.Bookdrop.Path = path
	if err := config.UpdateBookdropPath(path); err != nil {
		slog.Error("Failed to persist bookdrop path", "path", path, "error", err)
		errorResponse(w, http.StatusInternalServerError, "Failed to save bookdrop path")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"bookdrop": appConfig.Bookdrop,
	})
}

// getCbxPageCountHandler returns the total page count for a CBX archive
func getCbxPageCountHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	requestedFormat := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))

	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}

	filePath, format, err := selectBookFileByFormat(bookIDInt, requestedFormat)
	if err != nil || (requestedFormat == "" && format != "cbz" && format != "cbr" && format != "cb7") {
		err = appDB.QueryRow(`
			SELECT path, format FROM book_file
			WHERE book_id = ? AND format IN ('cbz', 'cbr', 'cb7')
			ORDER BY id
			LIMIT 1
		`, bookIDInt).Scan(&filePath, &format)
	}
	if err != nil {
		errorResponse(w, http.StatusNotFound, "CBX file not found")
		return
	}
	if format != "cbz" && format != "cbr" && format != "cb7" {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf("Format '%s' is not supported for comic reading.", format))
		return
	}

	filePath = translateHostPathToContainerPath(filePath)
	count, err := countCbxPages(filePath, format)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to count pages")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]int{"pages": count})
}

// getPdfPageCountHandler returns the total page count for a PDF using the local file.
func getPdfPageCountHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	requestedFormat := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	current := getUserFromContext(r.Context())

	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	filePath, format, err := selectBookFileByFormat(bookIDInt, requestedFormat)
	if err != nil || (requestedFormat == "" && format != "pdf") {
		err = appDB.QueryRow(`
			SELECT path, format FROM book_file
			WHERE book_id = ? AND format = 'pdf'
			ORDER BY id
			LIMIT 1
		`, bookIDInt).Scan(&filePath, &format)
	}
	if err != nil {
		errorResponse(w, http.StatusNotFound, "PDF file not found")
		return
	}
	if format != "pdf" {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf("Format '%s' is not supported for PDF reading.", format))
		return
	}

	filePath = translateHostPathToContainerPath(filePath)
	count, err := countPdfPages(filePath)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to count PDF pages")
		return
	}

	if count > 0 {
		_, _ = appDB.Exec(`
			UPDATE book_metadata
			SET page_count = ?
			WHERE book_id = ? AND COALESCE(page_count, 0) = 0
		`, count, bookIDInt)
	}

	jsonResponse(w, http.StatusOK, map[string]int{"pages": count})
}

// BookDrop handlers
func getBookdropFilesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := appDB.Query(`
		SELECT id, filename, path, status, COALESCE(error, '') as error, added_at
		FROM bookdrop_file
		WHERE status = 'pending'
		ORDER BY added_at
	`)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch bookdrop files")
		return
	}
	defer rows.Close()

	type BookdropFile struct {
		ID       int64  `json:"id"`
		Filename string `json:"filename"`
		Path     string `json:"path"`
		Status   string `json:"status"`
		Error    string `json:"error"`
		AddedAt  int64  `json:"added_at"`
	}

	files := []BookdropFile{}
	for rows.Next() {
		var f BookdropFile
		if err := rows.Scan(&f.ID, &f.Filename, &f.Path, &f.Status, &f.Error, &f.AddedAt); err != nil {
			continue
		}
		files = append(files, f)
	}

	jsonResponse(w, http.StatusOK, files)
}

func importBookdropFileHandler(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "id")

	var filePath, filename string
	err := appDB.QueryRow("SELECT path, filename FROM bookdrop_file WHERE id = ?", fileID).Scan(&filePath, &filename)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "BookDrop file not found")
		return
	}

	// Run import in background
	go func() {
		for _, lib := range appConfig.Libraries {
			var libraryID int64
			if err := appDB.QueryRow("SELECT id FROM library WHERE name = ? AND owner_user_id = ?", lib.Name, 1).Scan(&libraryID); err != nil {
				continue
			}
			appScanner.ScanLibrary(libraryID, []string{filePath})
			break // import into first library
		}
		appDB.Exec("UPDATE bookdrop_file SET status = 'imported' WHERE id = ?", fileID)
	}()

	jsonResponse(w, http.StatusOK, map[string]string{"status": "importing"})
}

func deleteBookdropFileHandler(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "id")
	appDB.Exec("UPDATE bookdrop_file SET status = 'rejected' WHERE id = ?", fileID)
	w.WriteHeader(http.StatusNoContent)
}

// SSE handler
func handleSSEHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		errorResponse(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	<-r.Context().Done()
}

// OPDS handlers
func handleOPDSRootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;kind=navigation")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2010/catalog">
  <id>urn:cryptorum:root</id>
  <title>Cryptorum Catalog</title>
  <updated>%s</updated>
  <link rel="self" href="/opds/" type="application/atom+xml;profile=opds-catalog;kind=navigation"/>
  <link rel="start" href="/opds/" type="application/atom+xml;profile=opds-catalog;kind=navigation"/>
  <entry>
    <id>urn:cryptorum:catalog</id>
    <title>All Books</title>
    <link rel="subsection" href="/opds/catalog" type="application/atom+xml;profile=opds-catalog;kind=acquisition"/>
  </entry>
</feed>`, time.Now().UTC().Format(time.RFC3339))
}

func handleOPDSCatalogHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;kind=acquisition")

	rows, err := appDB.Query(`
		SELECT b.id, COALESCE(bm.title, 'Unknown') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.description, '') as description,
		       COALESCE(bf.format, '') as format
		FROM book b
		JOIN library l ON b.library_id = l.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		LEFT JOIN (
			SELECT book_id, MIN(format) AS format
			FROM book_file
			GROUP BY book_id
		) bf ON b.id = bf.book_id
		WHERE `+ownerClause+`
		ORDER BY bm.title
		LIMIT 200
	`, ownerArgs...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to generate catalog")
		return
	}
	defer rows.Close()

	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2010/catalog">
  <id>urn:cryptorum:catalog</id>
  <title>All Books</title>
  <updated>%s</updated>
  <link rel="self" href="/opds/catalog" type="application/atom+xml;profile=opds-catalog;kind=acquisition"/>
`, time.Now().UTC().Format(time.RFC3339))

	mimeTypes := map[string]string{
		"epub": "application/epub+zip",
		"pdf":  "application/pdf",
		"cbz":  "application/vnd.comicbook+zip",
		"mp3":  "audio/mpeg",
		"m4b":  "audio/mp4",
	}

	for rows.Next() {
		var id int64
		var title, authors, description, format string
		if err := rows.Scan(&id, &title, &authors, &description, &format); err != nil {
			continue
		}
		mime := mimeTypes[format]
		if mime == "" {
			mime = "application/octet-stream"
		}
		fmt.Fprintf(w, `  <entry>
    <id>urn:cryptorum:book:%d</id>
    <title>%s</title>
    <summary>%s</summary>
    <link rel="http://opds-spec.org/acquisition" href="/opds/%d/download" type="%s"/>
    <link rel="http://opds-spec.org/image/thumbnail" href="/api/covers/%d/thumb" type="image/webp"/>
  </entry>
`, id, xmlEscape(title), xmlEscape(description), id, mime, id)
	}

	fmt.Fprintf(w, `</feed>`)
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func downloadBookHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "id")
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionDownloadBooks) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	bookIDInt, err := strconv.ParseInt(bookID, 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	allowed, err := canAccessBook(current, bookIDInt)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var filePath string
	err = appDB.QueryRow(`
		SELECT bf.path FROM book_file bf WHERE bf.book_id = ? LIMIT 1
	`, bookID).Scan(&filePath)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Book not found")
		return
	}

	http.ServeFile(w, r, filePath)
}

// Kobo handlers
func handleKoboAuthHandler(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"user": map[string]interface{}{
			"userId": "cryptorum-user",
		},
	})
}

func handleKoboSyncHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	// Return books formatted for Kobo consumption
	rows, err := appDB.Query(`
		SELECT b.id, COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       bf.format, bf.path
		FROM book b
		JOIN library l ON b.library_id = l.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		LEFT JOIN book_file bf ON b.id = bf.book_id
		WHERE bf.format IN ('epub', 'pdf')
		  AND `+ownerClause+`
		ORDER BY b.added_at DESC
		LIMIT 100
	`, ownerArgs...)
	if err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"added": []interface{}{}, "changed": []interface{}{},
			"removed": []interface{}{}, "entitlements": []interface{}{},
		})
		return
	}
	defer rows.Close()

	type KoboBook struct {
		BookID       string `json:"BookID"`
		Title        string `json:"Title"`
		Author       string `json:"Author"`
		DownloadURL  string `json:"DownloadUrl"`
		CoverImageID string `json:"CoverImageId"`
	}

	var added []KoboBook
	for rows.Next() {
		var id int64
		var title, authors, format, path string
		if err := rows.Scan(&id, &title, &authors, &format, &path); err != nil {
			continue
		}
		_ = path
		added = append(added, KoboBook{
			BookID:       fmt.Sprintf("cryptorum-%d", id),
			Title:        title,
			Author:       authors,
			DownloadURL:  fmt.Sprintf("/opds/%d/download", id),
			CoverImageID: fmt.Sprintf("cryptorum-cover-%d", id),
		})
	}

	if added == nil {
		added = []KoboBook{}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"added":        added,
		"changed":      []interface{}{},
		"removed":      []interface{}{},
		"entitlements": added,
	})
}

// Global session store
var sessionStore *auth.Store

const sessionCookieName = "cryptorum_session"
const sessionSignatureCookieName = "cryptorum_sig"

func authFailure(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maintenanceMode.Load() {
			errorResponse(w, http.StatusServiceUnavailable, "Maintenance in progress")
			return
		}

		if appConfig.Auth.Mode == "none" {
			if user, err := loadUserByID(1); err == nil {
				ctx := authContextWithUser(r.Context(), user)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		sessionID, _ := r.Cookie(sessionCookieName)

		if sessionStore == nil || sessionID == nil {
			authFailure(w, r)
			return
		}

		session, err := sessionStore.ValidateSession(sessionID.Value)
		if err != nil || session == nil {
			authFailure(w, r)
			return
		}

		user, err := loadUserByID(session.UserID)
		if err != nil {
			authFailure(w, r)
			return
		}

		ctx := r.Context()
		ctx = authContextWithSession(ctx, session)
		ctx = authContextWithUser(ctx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type contextKey string

const sessionContextKey contextKey = "session"
const userContextKey contextKey = "user"

func authContextWithSession(ctx context.Context, session *auth.Session) context.Context {
	return context.WithValue(ctx, sessionContextKey, session)
}

func getSessionFromContext(ctx context.Context) *auth.Session {
	session, _ := ctx.Value(sessionContextKey).(*auth.Session)
	return session
}

func authContextWithUser(ctx context.Context, user *AppUser) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

func getUserFromContext(ctx context.Context) *AppUser {
	user, _ := ctx.Value(userContextKey).(*AppUser)
	return user
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if appConfig.Auth.Mode != "password" {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "auth_disabled"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request")
		return
	}

	var user *AppUser
	if subtle.ConstantTimeCompare([]byte(req.Username), []byte(appConfig.Auth.Username)) == 1 &&
		auth.VerifyPasswordHash(req.Password, appConfig.Auth.PasswordHash) {
		configUser, err := loadUserByUsername(appConfig.Auth.Username)
		if err != nil {
			slog.Error("Login matched config credentials but failed to load user", "username", appConfig.Auth.Username, "error", err)
			errorResponse(w, http.StatusInternalServerError, "Authentication store unavailable")
			return
		}
		user = configUser
	}
	if user == nil {
		dbUser, err := loadUserByUsername(req.Username)
		if err != nil && err != sql.ErrNoRows {
			slog.Error("Login failed to load user", "username", req.Username, "error", err)
			errorResponse(w, http.StatusInternalServerError, "Authentication store unavailable")
			return
		}
		if err == nil && auth.VerifyPasswordHash(req.Password, dbUser.PasswordHash) {
			user = dbUser
		}
	}
	if user == nil {
		errorResponse(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	if sessionStore == nil {
		errorResponse(w, http.StatusInternalServerError, "Session store unavailable")
		return
	}

	session, err := sessionStore.CreateSession(user.ID, user.Username)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	recordAppLog("info", "auth", "User signed in", map[string]any{
		"username": user.Username,
		"user_id":  user.ID,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   shouldUseSecureCookies(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
		MaxAge:   int(time.Until(session.ExpiresAt).Seconds()),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     sessionSignatureCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   shouldUseSecureCookies(r),
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	sessionID, _ := r.Cookie(sessionCookieName)

	if sessionStore != nil && sessionID != nil {
		sessionStore.DeleteSession(sessionID.Value)
	}

	recordAppLog("info", "auth", "User signed out", nil)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   shouldUseSecureCookies(r),
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     sessionSignatureCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   shouldUseSecureCookies(r),
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func authCheckHandler(w http.ResponseWriter, r *http.Request) {
	if appConfig.Auth.Mode == "none" {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"authenticated": true,
			"auth_disabled": true,
		})
		return
	}

	sessionID, _ := r.Cookie(sessionCookieName)

	if sessionStore == nil || sessionID == nil {
		jsonResponse(w, http.StatusOK, map[string]bool{"authenticated": false})
		return
	}

	session, err := sessionStore.ValidateSession(sessionID.Value)
	if err != nil || session == nil {
		jsonResponse(w, http.StatusOK, map[string]bool{"authenticated": false})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"username":      session.Username,
		"user_id":       session.UserID,
	})
}

func shouldUseSecureCookies(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")))
	return proto == "https" || strings.HasPrefix(proto, "https,")
}
