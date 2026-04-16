package main

import (
	"context"
	"crypto/subtle"
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
	"cryptorum/internal/db"
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
	LibraryID  string     `json:"library_id"`
	Author     filterList `json:"author"`
	Series     filterList `json:"series"`
	Genre      filterList `json:"genre"`
	Tags       filterList `json:"tags"`
	Status     filterList `json:"status"`
	FilterMode string     `json:"filter_mode"`
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
	addFilterCondition(
		fmt.Sprintf(
			`EXISTS (SELECT 1 FROM json_each(COALESCE(%s, '[]')) WHERE value = ? OR value LIKE ?)`,
			column,
		),
		value,
		value+".%",
	)
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
	var filterConditions []string
	var filterArgs []interface{}

	addFilterCondition := func(condition string, values ...interface{}) {
		filterConditions = append(filterConditions, condition)
		filterArgs = append(filterArgs, values...)
	}

	if req.LibraryID != "" {
		conditions = append(conditions, "b.library_id = ?")
		args = append(args, req.LibraryID)
	}
	if user != nil && !userCanAccessAllData(user) {
		conditions = append(conditions, "l.owner_user_id = ?")
		args = append(args, user.ID)
	}
	for _, value := range req.Author {
		addFilterCondition(`EXISTS (SELECT 1 FROM json_each(COALESCE(bm.authors, '[]')) WHERE value = ?)`, value)
	}
	for _, value := range req.Series {
		addFilterCondition("COALESCE(bm.series, '') = ?", value)
	}
	for _, value := range cleanFilterValues(req.Genre, true) {
		addHierarchicalJSONFilterCondition(addFilterCondition, "bm.genres", value)
	}
	for _, value := range cleanFilterValues(req.Tags, true) {
		addHierarchicalJSONFilterCondition(addFilterCondition, "bm.tags", value)
	}
	for _, value := range req.Status {
		addFilterCondition("COALESCE(rp.status, 'unread') = ?", value)
	}

	filterMode := strings.ToUpper(req.FilterMode)
	if filterMode != "OR" && filterMode != "NOT" {
		filterMode = "AND"
	}
	if len(filterConditions) > 0 {
		switch filterMode {
		case "OR":
			conditions = append(conditions, "("+strings.Join(filterConditions, " OR ")+")")
			args = append(args, filterArgs...)
		case "NOT":
			conditions = append(conditions, "NOT ("+strings.Join(filterConditions, " OR ")+")")
			args = append(args, filterArgs...)
		default:
			conditions = append(conditions, filterConditions...)
			args = append(args, filterArgs...)
		}
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

	// PDF.js worker
	r.Get("/pdf.worker.min.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		http.ServeFile(w, r, "./static/pdf.worker.min.js")
	})

	// PDF.js worker (.mjs version for legacy builds)
	r.Get("/pdf.worker.min.mjs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		http.ServeFile(w, r, "./static/pdf.worker.min.mjs")
	})

	// API routes - protected
	r.Route("/api", func(r chi.Router) {
		r.Use(authMiddleware)

		// Books
		r.Route("/books", func(r chi.Router) {
			r.Get("/", getBooksHandler)
			r.Post("/bulk-delete", bulkDeleteBooksHandler)
			r.Post("/bulk-delete-by-filter", bulkDeleteByFilterHandler)
			r.Route("/{bookID}", func(r chi.Router) {
				r.Get("/", getBookHandler)
				r.Put("/", updateBookHandler)
				r.Delete("/", deleteBookHandler)
				r.Get("/files", getBookFilesHandler)
				r.Get("/files/{fileID}/download", ServeBookFileByIDHandler)
				r.Get("/files/{fileID}/convert", ConvertBookFileHandler)
				r.Get("/progress", GetReadingProgressHandler)
				r.Put("/progress", UpdateReadingProgressHandler)
				r.Put("/speed-reader", UpdateSpeedReaderProgressHandler)
				r.Post("/cover/regenerate", RegenerateBookCoverHandler)
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
				r.Delete("/books/{bookID}", removeBookFromShelfHandler)
			})
		})

		// Search
		r.Get("/search", searchBooksHandler)

		// Authors and Series
		r.Get("/authors", getAuthorsHandler)
		r.Get("/series", getSeriesHandler)

		// Metadata management
		r.Get("/metadata/{type}", getMetadataHandler)
		r.Get("/metadata/suggestions", getMetadataSuggestionsHandler)

		// Statistics
		r.Get("/stats", GetStatsHandler)

		// Reading history
		r.Get("/history", GetReadingHistoryHandler)

		// Admin workflow
		r.Route("/jobs", func(r chi.Router) {
			r.Get("/", ListJobsHandler)
			r.Get("/{jobID}", GetJobHandler)
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
	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs := http.StripPrefix(path[:len(path)-1], http.FileServer(root))
		fs.ServeHTTP(w, r)
	}).ServeHTTP)
}

// serveSPAHandler serves the frontend SPA
func serveSPAHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./static/index.html")
}

// getBooksHandler lists books with pagination, including reading status
func getBooksHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	libraryID := r.URL.Query().Get("library_id")
	status := r.URL.Query().Get("status")
	author := r.URL.Query().Get("author")
	series := r.URL.Query().Get("series")
	genre := r.URL.Query().Get("genre")
	tags := r.URL.Query().Get("tags")
	publisher := r.URL.Query().Get("publisher")
	language := r.URL.Query().Get("language")
	pubDate := r.URL.Query().Get("pub_date")
	filterMode := strings.ToUpper(r.URL.Query().Get("filter_mode"))
	sort := r.URL.Query().Get("sort")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	if filterMode != "OR" && filterMode != "NOT" {
		filterMode = "AND"
	}

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
			GROUP BY book_id
		) bf ON b.id = bf.book_id`

	var args []interface{}
	var conditions []string
	var filterConditions []string
	var filterArgs []interface{}

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

	addFilterCondition := func(condition string, values ...interface{}) {
		filterConditions = append(filterConditions, condition)
		filterArgs = append(filterArgs, values...)
	}

	if libraryID != "" {
		conditions = append(conditions, "b.library_id = ?")
		args = append(args, libraryID)
	}

	if current != nil && !userCanAccessAllData(current) {
		conditions = append(conditions, "l.owner_user_id = ?")
		args = append(args, current.ID)
	}

	if status != "" {
		for _, value := range queryValues("status", false) {
			addFilterCondition("COALESCE(rp.status, 'unread') = ?", value)
		}
	}

	// Author filter - searches in JSON authors array
	if author != "" {
		for _, value := range queryValues("author", false) {
			addFilterCondition(`EXISTS (SELECT 1 FROM json_each(COALESCE(bm.authors, '[]')) WHERE value = ?)`, value)
		}
	}

	// Series filter
	if series != "" {
		for _, value := range queryValues("series", false) {
			addFilterCondition("COALESCE(bm.series, '') = ?", value)
		}
	}

	// Genre filter
	if genre != "" {
		for _, value := range queryValues("genre", true) {
			addHierarchicalJSONFilterCondition(addFilterCondition, "bm.genres", value)
		}
	}

	// Tags filter
	if tags != "" {
		for _, value := range queryValues("tags", true) {
			addHierarchicalJSONFilterCondition(addFilterCondition, "bm.tags", value)
		}
	}

	// Publisher filter
	if publisher != "" {
		addFilterCondition("COALESCE(bm.publisher, '') = ?", publisher)
	}

	// Language filter
	if language != "" {
		addFilterCondition("COALESCE(bm.language, '') = ?", language)
	}

	// Publication date filter (exact match on pub_date field)
	if pubDate != "" {
		addFilterCondition("COALESCE(bm.pub_date, '') = ?", pubDate)
	}

	if len(filterConditions) > 0 {
		switch filterMode {
		case "OR":
			conditions = append(conditions, "("+strings.Join(filterConditions, " OR ")+")")
			args = append(args, filterArgs...)
		case "NOT":
			conditions = append(conditions, "NOT ("+strings.Join(filterConditions, " OR ")+")")
			args = append(args, filterArgs...)
		default:
			conditions = append(conditions, filterConditions...)
			args = append(args, filterArgs...)
		}
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	query := `
		SELECT b.id, b.library_id, b.added_at,
		       COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(bm.cover_updated_on, 0) as cover_updated_on,
		       COALESCE(rp.status, 'unread') as status,
		       COALESCE(rp.percent, 0) as percent,
		       CASE WHEN rp.book_id IS NOT NULL THEN 1 ELSE 0 END as opened,
		       COALESCE(bf.format, '') as format`
	query += baseQuery

	// Count total before applying limit/offset
	countQuery := "SELECT COUNT(*) " + baseQuery
	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	err := appDB.QueryRow(countQuery, countArgs...).Scan(&total)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to count books")
		return
	}

	// Handle sorting
	orderBy := "b.added_at DESC"
	if sort == "random" {
		orderBy = "RANDOM()"
	} else if sort == "last_read" {
		orderBy = "COALESCE(rp.updated_at, b.added_at) DESC"
	}

	query += " ORDER BY " + orderBy + " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := appDB.Query(query, args...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch books")
		return
	}
	defer rows.Close()

	type BookResponse struct {
		ID             int64   `json:"id"`
		LibraryID      int64   `json:"library_id"`
		AddedAt        int64   `json:"added_at"`
		Title          string  `json:"title"`
		Authors        string  `json:"authors"`
		CoverPath      string  `json:"cover_path"`
		CoverUpdatedOn int64   `json:"cover_updated_on"`
		Status         string  `json:"status"`
		Percent        float64 `json:"percent"`
		Opened         bool    `json:"opened"`
		Format         string  `json:"format"`
	}

	type BooksResponse struct {
		Books  []BookResponse `json:"books"`
		Total  int            `json:"total"`
		Offset int            `json:"offset"`
		Limit  int            `json:"limit"`
	}

	books := []BookResponse{}
	for rows.Next() {
		var b BookResponse
		var opened int
		if err := rows.Scan(&b.ID, &b.LibraryID, &b.AddedAt, &b.Title, &b.Authors, &b.CoverPath, &b.CoverUpdatedOn, &b.Status, &b.Percent, &opened, &b.Format); err != nil {
			continue
		}
		b.Opened = opened == 1
		books = append(books, b)
	}

	jsonResponse(w, http.StatusOK, BooksResponse{
		Books:  books,
		Total:  total,
		Offset: offset,
		Limit:  limit,
	})
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

	type BookDetail struct {
		ID                 int64   `json:"id"`
		LibraryID          int64   `json:"library_id"`
		LibraryName        string  `json:"library_name"`
		AddedAt            int64   `json:"added_at"`
		Title              string  `json:"title"`
		Authors            string  `json:"authors"`
		Series             string  `json:"series"`
		SeriesNumber       float64 `json:"series_number"`
		Publisher          string  `json:"publisher"`
		PubDate            string  `json:"pub_date"`
		Description        string  `json:"description"`
		CoverPath          string  `json:"cover_path"`
		Rating             float64 `json:"rating"`
		Genres             string  `json:"genres"`
		Tags               string  `json:"tags"`
		ISBN               string  `json:"isbn"`
		ASIN               string  `json:"asin"`
		Language           string  `json:"language"`
		PageCount          int     `json:"page_count"`
		Status             string  `json:"status"`
		Percent            float64 `json:"percent"`
		SpeedReaderPercent float64 `json:"speed_reader_percent"`
		Opened             bool    `json:"opened"`
	}

	var book BookDetail
	var opened int
	err = appDB.QueryRow(`
		SELECT b.id, b.library_id, b.added_at,
		       COALESCE(l.name, '') as library_name,
		       COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.series, '') as series,
		       COALESCE(bm.series_number, 0) as series_number,
		       COALESCE(bm.publisher, '') as publisher,
		       COALESCE(bm.pub_date, '') as pub_date,
		       COALESCE(bm.description, '') as description,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(bm.rating, 0) as rating,
		       COALESCE(bm.genres, '[]') as genres,
		       COALESCE(bm.tags, '[]') as tags,
		       COALESCE(bm.isbn, '') as isbn,
		       COALESCE(bm.asin, '') as asin,
		       COALESCE(bm.language, '') as language,
		       COALESCE(bm.page_count, 0) as page_count,
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
		&book.Publisher, &book.PubDate, &book.Description, &book.CoverPath,
		&book.Rating, &book.Genres, &book.Tags, &book.ISBN, &book.ASIN, &book.Language, &book.PageCount,
		&book.Status, &book.Percent, &book.SpeedReaderPercent, &opened,
	)

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
		Title        string   `json:"title"`
		Authors      []string `json:"authors"`
		Series       string   `json:"series"`
		SeriesNumber float64  `json:"series_number"`
		Publisher    string   `json:"publisher"`
		PubDate      string   `json:"pub_date"`
		Description  string   `json:"description"`
		Rating       float64  `json:"rating"`
		Status       string   `json:"status"`
		Genres       []string `json:"genres"`
		Tags         []string `json:"tags"`
		ISBN         string   `json:"isbn"`
		ASIN         string   `json:"asin"`
		Language     string   `json:"language"`
		PageCount    int      `json:"page_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	authorsJSON, _ := json.Marshal(req.Authors)
	genresJSON, _ := json.Marshal(req.Genres)
	tagsJSON, _ := json.Marshal(req.Tags)

	// Upsert metadata
	_, err = appDB.Exec(`
		INSERT INTO book_metadata (book_id, title, authors, series, series_number, publisher, pub_date,
		                           description, rating, genres, tags, isbn, asin, language, page_count, owner_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(book_id) DO UPDATE SET
		    title = excluded.title,
		    authors = excluded.authors,
		    series = excluded.series,
		    series_number = excluded.series_number,
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
		    owner_user_id = excluded.owner_user_id
	`, bookID, req.Title, string(authorsJSON), req.Series, req.SeriesNumber, req.Publisher, req.PubDate,
		req.Description, req.Rating, string(genresJSON), string(tagsJSON), req.ISBN, req.ASIN, req.Language, req.PageCount, current.ID)

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

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
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
		ID        int64
		Title     string
		Authors   string
		CoverPath string
		Format    string
		Score     int
		MatchType string
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
		       COALESCE(bm.genres, '[]'), COALESCE(bm.tags, '[]'), COALESCE(bm.publisher, ''),
		       (SELECT bf.format FROM book_file bf WHERE bf.book_id = b.id ORDER BY bf.format ASC LIMIT 1) as format
		FROM book b
		JOIN library l ON b.library_id = l.id
		JOIN book_metadata bm ON b.id = bm.book_id
		WHERE b.id != ? AND `+func() string { clause, _ := userOwnershipClause(current, "l"); return clause }()+`
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
		if err := rows.Scan(&id, &title, &authors, &coverPath, &genres, &tags, &publisher, &format); err != nil {
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
				if strings.EqualFold(strings.TrimSpace(author), strings.TrimSpace(targetAuthor)) {
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
			ID:        id,
			Title:     title,
			Authors:   authors,
			CoverPath: coverPath,
			Format:    format,
			Score:     score,
			MatchType: matchType,
		})

		if score >= 30 {
			tagMatches = append(tagMatches, scoredBook{
				ID: id, Title: title, Authors: authors, CoverPath: coverPath, Format: format, Score: score, MatchType: matchType,
			})
		}
		if score >= 20 {
			genreMatches = append(genreMatches, scoredBook{
				ID: id, Title: title, Authors: authors, CoverPath: coverPath, Format: format, Score: score, MatchType: matchType,
			})
		}
		if score >= 50 {
			authorMatches = append(authorMatches, scoredBook{
				ID: id, Title: title, Authors: authors, CoverPath: coverPath, Format: format, Score: score, MatchType: matchType,
			})
		}
		if score >= 10 {
			publisherMatches = append(publisherMatches, scoredBook{
				ID: id, Title: title, Authors: authors, CoverPath: coverPath, Format: format, Score: score, MatchType: matchType,
			})
		}
		if score >= 15 {
			similarNameMatches = append(similarNameMatches, scoredBook{
				ID: id, Title: title, Authors: authors, CoverPath: coverPath, Format: format, Score: score, MatchType: matchType,
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
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		Authors   string `json:"authors"`
		CoverPath string `json:"cover_path"`
		Format    string `json:"format"`
		Score     int    `json:"score"`
		MatchType string `json:"match_type"`
	}

	similarResult := make([]SimilarBook, len(result))
	for i, c := range result {
		similarResult[i] = SimilarBook{
			ID:        c.ID,
			Title:     c.Title,
			Authors:   c.Authors,
			CoverPath: c.CoverPath,
			Format:    c.Format,
			Score:     c.Score,
			MatchType: c.MatchType,
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
		       COUNT(DISTINCT b.id) as book_count
		FROM library l
		LEFT JOIN book b ON l.id = b.library_id
		WHERE `+ownerClause+`
		GROUP BY l.id, l.name, l.icon
		ORDER BY l.name
	`, ownerArgs...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch libraries")
		return
	}
	defer rows.Close()

	type LibraryResponse struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Icon        string `json:"icon"`
		BookCount   int64  `json:"book_count"`
		IsImporting bool   `json:"is_importing"`
	}

	libraries := []LibraryResponse{}
	for rows.Next() {
		var lib LibraryResponse
		if err := rows.Scan(&lib.ID, &lib.Name, &lib.Icon, &lib.BookCount); err != nil {
			continue
		}
		libraries = append(libraries, lib)
	}

	jsonResponse(w, http.StatusOK, libraries)
}

func getLibraryHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	libraryID := chi.URLParam(r, "libraryID")
	ownerClause, ownerArgs := userOwnershipClause(current, "l")

	type LibraryResponse struct {
		ID        int64    `json:"id"`
		Name      string   `json:"name"`
		Icon      string   `json:"icon"`
		BookCount int64    `json:"book_count"`
		Paths     []string `json:"paths"`
	}

	var lib LibraryResponse
	query := `
		SELECT l.id, l.name, COALESCE(l.icon, '') as icon,
		       COUNT(DISTINCT b.id) as book_count
		FROM library l
		LEFT JOIN book b ON l.id = b.library_id
		WHERE l.id = ? AND ` + ownerClause + `
		GROUP BY l.id
	`
	err := appDB.QueryRow(query, append([]interface{}{libraryID}, ownerArgs...)...).Scan(&lib.ID, &lib.Name, &lib.Icon, &lib.BookCount)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Library not found")
		return
	}

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
		Name  string   `json:"name"`
		Icon  string   `json:"icon"`
		Paths []string `json:"paths"`
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

	result, err := tx.Exec(`INSERT INTO library (id, name, icon, owner_user_id) VALUES (?, ?, ?, ?)`, libraryID, req.Name, req.Icon, current.ID)
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

	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"id":    libraryID,
		"name":  req.Name,
		"icon":  req.Icon,
		"paths": req.Paths,
	})
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
		Name  string   `json:"name"`
		Icon  string   `json:"icon"`
		Paths []string `json:"paths"`
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

	_, err = tx.Exec(`UPDATE library SET name = ?, icon = ? WHERE id = ?`, req.Name, req.Icon, libraryID)
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

	// Set importing status
	scanningLibraries[libraryID] = true

	go func() {
		appScanner.ScanLibrary(libraryID, paths)
		scanningLibraries[libraryID] = false
	}()

	jsonResponse(w, http.StatusOK, map[string]string{"status": "scanning"})
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
		entryType := "file"
		if entry.IsDir() {
			entryType = "directory"
		}

		fullPath := filepath.Join(path, entry.Name())
		result = append(result, DirectoryEntry{
			Name: entry.Name(),
			Type: entryType,
			Path: fullPath,
		})
	}

	jsonResponse(w, http.StatusOK, result)
}

// searchBooksHandler uses FTS5 for full-text search with LIKE fallback
func searchBooksHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	query := r.URL.Query().Get("q")
	if query == "" {
		jsonResponse(w, http.StatusOK, []interface{}{})
		return
	}

	type SearchResult struct {
		ID          int64  `json:"id"`
		Title       string `json:"title"`
		Authors     string `json:"authors"`
		Description string `json:"description"`
		CoverPath   string `json:"cover_path"`
		Status      string `json:"status"`
	}

	results := []SearchResult{}

	// Use LIKE search for fuzzy matching (FTS will be enabled after database rebuild)
	likePattern := "%" + query + "%"
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	rows, err := appDB.Query(`
		SELECT b.id, COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.description, '') as description,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(rp.status, 'unread') as status
		FROM book_metadata bm
		JOIN book b ON bm.book_id = b.id
		JOIN library l ON b.library_id = l.id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		WHERE (`+ownerClause+`) AND (bm.title LIKE ? OR bm.authors LIKE ? OR bm.description LIKE ? OR COALESCE(bm.series, '') LIKE ? OR COALESCE(bm.asin, '') LIKE ?)
		LIMIT 50
	`, append(ownerArgs, likePattern, likePattern, likePattern, likePattern, likePattern)...)

	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Search failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var res SearchResult
		if err := rows.Scan(&res.ID, &res.Title, &res.Authors, &res.Description, &res.CoverPath, &res.Status); err != nil {
			continue
		}
		results = append(results, res)
	}

	jsonResponse(w, http.StatusOK, results)
}

// Authors and Series handlers
func getAuthorsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	rows, err := appDB.Query(`
		SELECT bm.authors, COUNT(*) as book_count
		FROM book_metadata bm
		JOIN book b ON bm.book_id = b.id
		JOIN library l ON b.library_id = l.id
		WHERE `+ownerClause+` AND bm.authors IS NOT NULL AND bm.authors != '[]' AND bm.authors != ''
		GROUP BY bm.authors
		ORDER BY book_count DESC, bm.authors
	`, ownerArgs...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch authors")
		return
	}
	defer rows.Close()

	type AuthorResponse struct {
		Name      string `json:"name"`
		BookCount int64  `json:"book_count"`
	}

	authors := []AuthorResponse{}
	for rows.Next() {
		var authorsJson string
		var bookCount int64
		if err := rows.Scan(&authorsJson, &bookCount); err != nil {
			continue
		}

		// Parse JSON array of authors
		var authorList []string
		if err := json.Unmarshal([]byte(authorsJson), &authorList); err != nil {
			continue
		}

		// Add each author individually
		for _, author := range authorList {
			if author == "" {
				continue
			}
			// Check if author already exists
			found := false
			for i, existing := range authors {
				if existing.Name == author {
					authors[i].BookCount += bookCount
					found = true
					break
				}
			}
			if !found {
				authors = append(authors, AuthorResponse{
					Name:      author,
					BookCount: bookCount,
				})
			}
		}
	}

	// Sort by book count descending, then by name
	sort.Slice(authors, func(i, j int) bool {
		if authors[i].BookCount == authors[j].BookCount {
			return authors[i].Name < authors[j].Name
		}
		return authors[i].BookCount > authors[j].BookCount
	})

	jsonResponse(w, http.StatusOK, authors)
}

func getSeriesHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	rows, err := appDB.Query(`
		SELECT bm.series, COUNT(*) as book_count
		FROM book_metadata bm
		JOIN book b ON bm.book_id = b.id
		JOIN library l ON b.library_id = l.id
		WHERE `+ownerClause+` AND bm.series IS NOT NULL AND bm.series != ''
		GROUP BY bm.series
		ORDER BY book_count DESC, bm.series
	`, ownerArgs...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch series")
		return
	}
	defer rows.Close()

	type SeriesResponse struct {
		Name      string `json:"name"`
		BookCount int64  `json:"book_count"`
	}

	series := []SeriesResponse{}
	for rows.Next() {
		var name string
		var bookCount int64
		if err := rows.Scan(&name, &bookCount); err != nil {
			continue
		}
		series = append(series, SeriesResponse{
			Name:      name,
			BookCount: bookCount,
		})
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
				if author == "" {
					continue
				}
				found := false
				for i, existing := range metadata {
					if existing.Name == author {
						metadata[i].BookCount += bookCount
						found = true
						break
					}
				}
				if !found {
					metadata = append(metadata, MetadataResponse{
						Name:      author,
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
		       COUNT(DISTINCT b.id) as book_count
		FROM library l
		LEFT JOIN book b ON l.id = b.library_id
		GROUP BY l.id, l.name, l.icon
		ORDER BY l.name
	`)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch libraries")
		return
	}
	defer rows.Close()

	type LibraryResponse struct {
		ID          int64    `json:"id"`
		Name        string   `json:"name"`
		Icon        string   `json:"icon"`
		BookCount   int64    `json:"book_count"`
		Paths       []string `json:"paths"`
		IsImporting bool     `json:"is_importing"`
	}

	libraries := []LibraryResponse{}
	for rows.Next() {
		var lib LibraryResponse
		if err := rows.Scan(&lib.ID, &lib.Name, &lib.Icon, &lib.BookCount); err != nil {
			continue
		}

		// Fetch paths
		pathRows, _ := appDB.Query("SELECT path FROM library_path WHERE library_id = ?", lib.ID)
		paths := []string{}
		for pathRows.Next() {
			var path string
			pathRows.Scan(&path)
			paths = append(paths, path)
		}
		pathRows.Close()

		lib.IsImporting = len(scanningLibraries) > 0 // Set to true if any library is being scanned
		lib.Paths = paths
		libraries = append(libraries, lib)
	}

	// Default reader settings
	readerSettings := map[string]interface{}{
		"epub": map[string]interface{}{
			"fontFamily": "serif",
			"fontSize":   16,
			"lineHeight": 1.5,
			"margin":     20,
			"textAlign":  "justify",
			"theme":      "light",
		},
		"pdf": map[string]interface{}{
			"pageFit":         "auto",
			"zoomLevel":       100,
			"scrollDirection": "vertical",
		},
		"cbx": map[string]interface{}{
			"readerMode": "single",
			"direction":  "ltr",
		},
		"audio": map[string]interface{}{
			"playbackSpeed": 1.0,
			"autoAdvance":   false,
		},
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
	// In a full implementation, you'd want to persist these settings
	// For demonstration, we'll just acknowledge the request

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

	count, err := countCbzPages(filePath)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to count pages")
		return
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

func init() {
	sessionStore = auth.NewStore("cryptorum-secret-key-change-in-production", 720*time.Hour)
}

const sessionCookieName = "cryptorum_session"
const sessionSignatureCookieName = "cryptorum_sig"

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
		signature, _ := r.Cookie(sessionSignatureCookieName)

		if sessionID == nil || signature == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		if !sessionStore.VerifySignature(sessionID.Value, signature.Value) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		session, err := sessionStore.ValidateSession(sessionID.Value)
		if err != nil || session == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		user, err := loadUserByID(session.UserID)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
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
		user, _ = loadUserByUsername(appConfig.Auth.Username)
	}
	if user == nil {
		dbUser, err := loadUserByUsername(req.Username)
		if err == nil && auth.VerifyPasswordHash(req.Password, dbUser.PasswordHash) {
			user = dbUser
		}
	}
	if user == nil {
		errorResponse(w, http.StatusUnauthorized, "Invalid credentials")
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
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     sessionSignatureCookieName,
		Value:    sessionStore.SignSession(session.ID),
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	sessionID, _ := r.Cookie(sessionCookieName)

	if sessionID != nil {
		sessionStore.DeleteSession(sessionID.Value)
	}

	recordAppLog("info", "auth", "User signed out", nil)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     sessionSignatureCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
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
	signature, _ := r.Cookie(sessionSignatureCookieName)

	if sessionID == nil || signature == nil {
		jsonResponse(w, http.StatusOK, map[string]bool{"authenticated": false})
		return
	}

	if !sessionStore.VerifySignature(sessionID.Value, signature.Value) {
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
