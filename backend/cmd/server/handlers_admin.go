package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type MetadataApplyJobItem struct {
	BookID   int64             `json:"book_id"`
	Metadata MetadataCandidate `json:"metadata"`
}

type MetadataApplyJobRequest struct {
	Items        []MetadataApplyJobItem `json:"items"`
	IncludeCover bool                   `json:"include_cover"`
}

type MetadataLookupJobRequest struct {
	BookIDs      []int64 `json:"book_ids"`
	Provider     string  `json:"provider,omitempty"`
	TopMatchOnly bool    `json:"top_match_only,omitempty"`
}

type AdminJob struct {
	ID                int64           `json:"id"`
	JobType           string          `json:"job_type"`
	Title             string          `json:"title"`
	Status            string          `json:"status"`
	Payload           json.RawMessage `json:"payload,omitempty"`
	Result            json.RawMessage `json:"result,omitempty"`
	TotalItems        int             `json:"total_items"`
	CompletedItems    int             `json:"completed_items"`
	FailedItems       int             `json:"failed_items"`
	Error             string          `json:"error,omitempty"`
	CreatedAt         int64           `json:"created_at"`
	StartedAt         *int64          `json:"started_at,omitempty"`
	CompletedAt       *int64          `json:"completed_at,omitempty"`
	CancelRequestedAt *int64          `json:"cancel_requested_at,omitempty"`
}

type AdminNotification struct {
	ID        int64     `json:"id"`
	Source    string    `json:"source"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Message   string    `json:"message,omitempty"`
	URL       string    `json:"url,omitempty"`
	ReadAt    *int64    `json:"read_at,omitempty"`
	CreatedAt int64     `json:"created_at"`
	Job       *AdminJob `json:"job,omitempty"`
}

type AdminLogEntry struct {
	ID        int64           `json:"id"`
	Level     string          `json:"level"`
	Category  string          `json:"category"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt int64           `json:"created_at"`
}

type MetadataApplyJobResultItem struct {
	BookID   int64  `json:"book_id"`
	Title    string `json:"title,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	Provider string `json:"provider,omitempty"`
}

type MetadataLookupBookSnapshot struct {
	BookID      int64    `json:"book_id"`
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	Series      string   `json:"series,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	PubDate     string   `json:"pub_date,omitempty"`
	Description string   `json:"description,omitempty"`
	ISBN        string   `json:"isbn,omitempty"`
	ASIN        string   `json:"asin,omitempty"`
	CoverPath   string   `json:"cover_path,omitempty"`
	PageCount   int      `json:"page_count,omitempty"`
	Language    string   `json:"language,omitempty"`
}

type MetadataLookupJobResultItem struct {
	BookID      int64                        `json:"book_id"`
	Current     MetadataLookupBookSnapshot   `json:"current"`
	Match       *MetadataCandidate           `json:"match,omitempty"`
	Matches     []MetadataCandidate          `json:"matches,omitempty"`
	Status      string                       `json:"status"`
	Error       string                       `json:"error,omitempty"`
	Query       string                       `json:"query,omitempty"`
	Diagnostics []MetadataProviderDiagnostic `json:"diagnostics,omitempty"`
}

const metadataBulkLookupCandidateLimit = 6

func recordAppLog(level, category, message string, data any) {
	if appDB == nil {
		return
	}

	var dataJSON []byte
	if data != nil {
		dataJSON, _ = json.Marshal(data)
	}

	_, err := appDB.Exec(`
		INSERT INTO app_log (level, category, message, data_json, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, level, category, message, nullString(dataJSON), time.Now().Unix())
	if err != nil {
		slog.Warn("Failed to record app log", "category", category, "error", err)
	}
}

func createAdminNotification(kind, title, message, url string) {
	if appDB == nil {
		return
	}

	_, err := appDB.Exec(`
		INSERT INTO app_notification (kind, title, message, url, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, kind, title, message, url, time.Now().Unix())
	if err != nil {
		slog.Warn("Failed to create admin notification", "kind", kind, "error", err)
	}
}

func nullString(data []byte) any {
	if len(data) == 0 {
		return nil
	}
	return string(data)
}

func rawMessageOrNil(value sql.NullString) json.RawMessage {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	return json.RawMessage(value.String)
}

func scanOptionalInt(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func loadAdminJob(jobID int64) (AdminJob, error) {
	var job AdminJob
	var payload, result sql.NullString
	var startedAt, completedAt, cancelRequestedAt sql.NullInt64
	err := appDB.QueryRow(`
		SELECT id, job_type, title, status, payload_json, result_json,
		       total_items, completed_items, failed_items, COALESCE(error, ''),
		       created_at, started_at, completed_at, cancel_requested_at
		FROM metadata_job
		WHERE id = ?
	`, jobID).Scan(
		&job.ID, &job.JobType, &job.Title, &job.Status, &payload, &result,
		&job.TotalItems, &job.CompletedItems, &job.FailedItems, &job.Error,
		&job.CreatedAt, &startedAt, &completedAt, &cancelRequestedAt,
	)
	if err != nil {
		return AdminJob{}, err
	}

	job.Payload = rawMessageOrNil(payload)
	job.Result = rawMessageOrNil(result)
	job.StartedAt = scanOptionalInt(startedAt)
	job.CompletedAt = scanOptionalInt(completedAt)
	job.CancelRequestedAt = scanOptionalInt(cancelRequestedAt)
	return job, nil
}

func isJobNotificationKind(kind string) bool {
	return strings.HasPrefix(kind, "job_") ||
		strings.HasPrefix(kind, "library_scan_") ||
		strings.HasPrefix(kind, "backup_") ||
		strings.HasPrefix(kind, "cover_regeneration_")
}

func isActiveJobStatus(status string) bool {
	return status == "queued" || status == "running" || status == "cancelling"
}

func jobNotificationMessage(job AdminJob) string {
	progress := ""
	if job.TotalItems > 0 {
		progress = fmt.Sprintf(" %d/%d completed, %d failed.", job.CompletedItems, job.TotalItems, job.FailedItems)
	}
	if job.Error != "" {
		return fmt.Sprintf("%s.%s Error: %s", job.Status, progress, job.Error)
	}
	return fmt.Sprintf("%s.%s", job.Status, progress)
}

func jobNotificationReadAt(job AdminJob) *int64 {
	if isActiveJobStatus(job.Status) {
		return nil
	}
	if job.CompletedAt != nil {
		return job.CompletedAt
	}
	readAt := job.CreatedAt
	return &readAt
}

func humanizeKey(key string) string {
	parts := strings.FieldsFunc(key, func(r rune) bool {
		return r == '_' || r == '-'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func humanizeLogData(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}

	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil || len(values) == 0 {
		return ""
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		switch typed := value.(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(typed) == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s: %s", humanizeKey(key), typed))
		case float64:
			if typed == float64(int64(typed)) {
				parts = append(parts, fmt.Sprintf("%s: %d", humanizeKey(key), int64(typed)))
			} else {
				parts = append(parts, fmt.Sprintf("%s: %.2f", humanizeKey(key), typed))
			}
		case bool:
			parts = append(parts, fmt.Sprintf("%s: %t", humanizeKey(key), typed))
		default:
			encoded, err := json.Marshal(typed)
			if err != nil {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s: %s", humanizeKey(key), string(encoded)))
		}
	}

	return strings.Join(parts, " · ")
}

func notificationTextLine(item AdminNotification) string {
	when := time.Unix(item.CreatedAt, 0).Format(time.RFC3339)
	source := item.Kind
	if item.Source != "" {
		source = item.Source + " · " + item.Kind
	}
	line := fmt.Sprintf("%s [%s] %s", when, source, item.Title)
	if strings.TrimSpace(item.Message) != "" {
		line += " - " + item.Message
	}
	return line
}

func splitCSVFilter(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func firstQueryValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	values := make([]string, count)
	for i := range values {
		values[i] = "?"
	}
	return strings.Join(values, ", ")
}

func loadMetadataLookupSnapshot(bookID int64) (MetadataLookupBookSnapshot, error) {
	var snapshot MetadataLookupBookSnapshot
	var authorsJSON string
	err := appDB.QueryRow(`
		SELECT b.id,
		       COALESCE(bm.title, ''),
		       COALESCE(bm.authors, '[]'),
		       COALESCE(bm.series, ''),
		       COALESCE(bm.publisher, ''),
		       COALESCE(bm.pub_date, ''),
		       COALESCE(bm.description, ''),
		       COALESCE(bm.isbn, ''),
		       COALESCE(bm.asin, ''),
		       COALESCE(bm.cover_path, ''),
		       COALESCE(bm.page_count, 0),
		       COALESCE(bm.language, '')
		FROM book b
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		WHERE b.id = ?
	`, bookID).Scan(
		&snapshot.BookID,
		&snapshot.Title,
		&authorsJSON,
		&snapshot.Series,
		&snapshot.Publisher,
		&snapshot.PubDate,
		&snapshot.Description,
		&snapshot.ISBN,
		&snapshot.ASIN,
		&snapshot.CoverPath,
		&snapshot.PageCount,
		&snapshot.Language,
	)
	if err != nil {
		return MetadataLookupBookSnapshot{}, err
	}

	if err := json.Unmarshal([]byte(authorsJSON), &snapshot.Authors); err != nil {
		snapshot.Authors = strings.Split(authorsJSON, ",")
		for i := range snapshot.Authors {
			snapshot.Authors[i] = strings.TrimSpace(snapshot.Authors[i])
		}
	}

	return snapshot, nil
}

func metadataLookupQuery(snapshot MetadataLookupBookSnapshot) string {
	parts := []string{
		snapshot.Title,
		strings.Join(snapshot.Authors, " "),
		snapshot.ISBN,
		snapshot.ASIN,
		snapshot.Series,
		snapshot.Publisher,
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func applyMetadataCandidateToBook(bookID int64, candidate MetadataCandidate, includeCover bool) error {
	var exists bool
	if err := appDB.QueryRow("SELECT EXISTS(SELECT 1 FROM book WHERE id = ?)", bookID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("book not found")
	}

	candidate.Authors = normalizeMetadataStringList(candidate.Authors)
	candidateTags := metadataCandidateTags(candidate)

	authorsJSON, _ := json.Marshal(candidate.Authors)
	emptyJSON, _ := json.Marshal([]string{})
	candidateTagsJSON, _ := json.Marshal(candidateTags)

	type currentMetadata struct {
		Title        string
		Authors      string
		Series       string
		SeriesNumber float64
		Publisher    string
		PubDate      string
		Description  string
		Rating       float64
		Genres       string
		Tags         string
		ISBN         string
		ASIN         string
		CoverPath    string
		CoverUpdated int64
		PageCount    int64
		Language     string
		LockedFields string
	}

	var current currentMetadata
	err := appDB.QueryRow(`
		SELECT COALESCE(title, ''),
		       COALESCE(authors, '[]'),
		       COALESCE(series, ''),
		       COALESCE(series_number, 0),
		       COALESCE(publisher, ''),
		       COALESCE(pub_date, ''),
		       COALESCE(description, ''),
		       COALESCE(rating, 0),
		       COALESCE(genres, '[]'),
		       COALESCE(tags, '[]'),
		       COALESCE(isbn, ''),
		       COALESCE(asin, ''),
		       COALESCE(cover_path, ''),
		       COALESCE(cover_updated_on, 0),
		       COALESCE(page_count, 0),
		       COALESCE(language, ''),
		       COALESCE(locked_fields, '[]')
		FROM book_metadata
		WHERE book_id = ?
	`, bookID).Scan(
		&current.Title,
		&current.Authors,
		&current.Series,
		&current.SeriesNumber,
		&current.Publisher,
		&current.PubDate,
		&current.Description,
		&current.Rating,
		&current.Genres,
		&current.Tags,
		&current.ISBN,
		&current.ASIN,
		&current.CoverPath,
		&current.CoverUpdated,
		&current.PageCount,
		&current.Language,
		&current.LockedFields,
	)

	if err == sql.ErrNoRows {
		_, err = appDB.Exec(`
			INSERT INTO book_metadata (
				book_id, title, authors, series, series_number, publisher,
				pub_date, description, rating, genres, tags, isbn, asin, cover_path,
				cover_updated_on, page_count, language, locked_fields
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, bookID, candidate.Title, nullString(authorsJSON), candidate.Series,
			0, candidate.Publisher, candidate.PubDate, candidate.Description,
			candidate.Rating, nullString(emptyJSON), nullString(candidateTagsJSON), candidate.ISBN, candidate.ASIN, "",
			0, candidate.PageCount, candidate.Language, "[]")
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		var locked []string
		if err := json.Unmarshal([]byte(current.LockedFields), &locked); err != nil {
			locked = nil
		}

		finalTitle := current.Title
		if candidate.Title != "" && !contains(locked, "title") {
			finalTitle = candidate.Title
		}

		finalAuthors := current.Authors
		if len(candidate.Authors) > 0 && !contains(locked, "authors") {
			finalAuthors = nullString(authorsJSON).(string)
		}

		finalSeries := current.Series
		if candidate.Series != "" && !contains(locked, "series") {
			finalSeries = candidate.Series
		}

		finalSeriesNumber := current.SeriesNumber

		finalPublisher := current.Publisher
		if candidate.Publisher != "" && !contains(locked, "publisher") {
			finalPublisher = candidate.Publisher
		}

		finalPubDate := current.PubDate
		if candidate.PubDate != "" && !contains(locked, "pub_date") {
			finalPubDate = candidate.PubDate
		}

		finalDescription := current.Description
		if candidate.Description != "" && !contains(locked, "description") {
			finalDescription = candidate.Description
		}

		finalRating := current.Rating
		if candidate.Rating > 0 && !contains(locked, "rating") {
			finalRating = candidate.Rating
		}

		finalTags := current.Tags
		if len(candidateTags) > 0 && !contains(locked, "tags") && !contains(locked, "genres") {
			mergedTagsJSON, _ := json.Marshal(mergeMetadataTagLists(parseMetadataJSONList(current.Tags), candidateTags))
			finalTags = string(mergedTagsJSON)
		}

		finalISBN := current.ISBN
		if candidate.ISBN != "" && !contains(locked, "isbn") {
			finalISBN = candidate.ISBN
		}

		finalASIN := current.ASIN
		if candidate.ASIN != "" && !contains(locked, "asin") {
			finalASIN = candidate.ASIN
		}

		finalPageCount := current.PageCount
		if candidate.PageCount > 0 && !contains(locked, "page_count") {
			finalPageCount = int64(candidate.PageCount)
		}

		finalLanguage := current.Language
		if candidate.Language != "" && !contains(locked, "language") {
			finalLanguage = candidate.Language
		}

		_, err = appDB.Exec(`
			UPDATE book_metadata SET
				title = ?,
				authors = ?,
				series = ?,
				series_number = ?,
				publisher = ?,
				pub_date = ?,
				description = ?,
				rating = ?,
				tags = ?,
				isbn = ?,
				asin = ?,
				page_count = ?,
				language = ?
			WHERE book_id = ?
		`, finalTitle, finalAuthors, finalSeries, finalSeriesNumber, finalPublisher,
			finalPubDate, finalDescription, finalRating, finalTags, finalISBN, finalASIN,
			finalPageCount, finalLanguage, bookID)
		if err != nil {
			return err
		}
	}

	if includeCover && candidate.CoverURL != "" {
		downloadCover(bookID, candidate.CoverURL)
	}

	recordAppLog("info", "metadata", "Applied metadata to book", map[string]any{
		"book_id":  bookID,
		"title":    candidate.Title,
		"provider": candidate.Provider,
	})
	return nil
}

func QueueMetadataApplyJobHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req MetadataApplyJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(req.Items) == 0 {
		errorResponse(w, http.StatusBadRequest, "No metadata items provided")
		return
	}

	title := fmt.Sprintf("Bulk metadata update (%d books)", len(req.Items))
	payload, _ := json.Marshal(req)
	now := time.Now().Unix()

	res, err := appDB.Exec(`
		INSERT INTO metadata_job (
			job_type, title, status, payload_json,
			total_items, completed_items, failed_items,
			created_at
		) VALUES (?, ?, ?, ?, ?, 0, 0, ?)
	`, "metadata_apply", title, "queued", nullString(payload), len(req.Items), now)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to queue job")
		return
	}

	jobID, _ := res.LastInsertId()
	createAdminNotification(
		"job_queued",
		title,
		"Queued a background metadata job.",
		"/settings?tab=jobs",
	)
	recordAppLog("info", "jobs", "Queued metadata apply job", map[string]any{
		"job_id": jobID,
		"count":  len(req.Items),
	})

	go processMetadataApplyJob(jobID, req, title)

	job, err := loadAdminJob(jobID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load queued job")
		return
	}

	jsonResponse(w, http.StatusAccepted, job)
}

func QueueMetadataLookupJobHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req MetadataLookupJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(req.BookIDs) == 0 {
		errorResponse(w, http.StatusBadRequest, "No books provided")
		return
	}

	seen := map[int64]bool{}
	bookIDs := make([]int64, 0, len(req.BookIDs))
	for _, bookID := range req.BookIDs {
		if bookID <= 0 || seen[bookID] {
			continue
		}
		allowed, err := canAccessBook(current, bookID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
			return
		}
		if !allowed {
			errorResponse(w, http.StatusForbidden, "Permission denied")
			return
		}
		seen[bookID] = true
		bookIDs = append(bookIDs, bookID)
	}

	if len(bookIDs) == 0 {
		errorResponse(w, http.StatusBadRequest, "No valid books provided")
		return
	}

	req.BookIDs = bookIDs
	req.Provider = strings.TrimSpace(req.Provider)

	title := fmt.Sprintf("Bulk metadata lookup (%d books)", len(req.BookIDs))
	payload, _ := json.Marshal(req)
	now := time.Now().Unix()

	res, err := appDB.Exec(`
		INSERT INTO metadata_job (
			job_type, title, status, payload_json,
			total_items, completed_items, failed_items,
			created_at
		) VALUES (?, ?, ?, ?, ?, 0, 0, ?)
	`, "metadata_lookup", title, "queued", nullString(payload), len(req.BookIDs), now)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to queue job")
		return
	}

	jobID, _ := res.LastInsertId()
	initialResults, initialFailed, firstErr := initializeMetadataLookupJobResults(req.BookIDs)
	initialPayload, _ := json.Marshal(map[string]any{
		"items":     initialResults,
		"completed": 0,
		"failed":    initialFailed,
		"total":     len(req.BookIDs),
	})
	_, _ = appDB.Exec(`
		UPDATE metadata_job
		SET failed_items = ?, error = ?, result_json = ?
		WHERE id = ?
	`, initialFailed, firstErr, nullString(initialPayload), jobID)

	createAdminNotification(
		"job_queued",
		title,
		"Queued a background metadata lookup job.",
		"/settings?tab=jobs",
	)
	recordAppLog("info", "jobs", "Queued metadata lookup job", map[string]any{
		"job_id": jobID,
		"count":  len(req.BookIDs),
	})

	go processMetadataLookupJob(jobID, req, title, initialResults, initialFailed, firstErr)

	job, err := loadAdminJob(jobID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load queued job")
		return
	}

	jsonResponse(w, http.StatusAccepted, job)
}

func GetJobHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionViewAdmin) && !requirePermission(current, PermissionManageJobs) && !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	jobID, err := strconv.ParseInt(chi.URLParam(r, "jobID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid job ID")
		return
	}

	job, err := loadAdminJob(jobID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Job not found")
		return
	}

	jsonResponse(w, http.StatusOK, job)
}

func initializeMetadataLookupJobResults(bookIDs []int64) ([]MetadataLookupJobResultItem, int, string) {
	results := make([]MetadataLookupJobResultItem, 0, len(bookIDs))
	failed := 0
	var firstErr string

	for _, bookID := range bookIDs {
		item := MetadataLookupJobResultItem{
			BookID: bookID,
			Status: "pending",
		}
		snapshot, err := loadMetadataLookupSnapshot(bookID)
		if err != nil {
			item.Status = "failed"
			item.Error = err.Error()
			failed++
		} else {
			item.Current = snapshot
			item.Query = metadataLookupQuery(snapshot)
			if item.Query == "" {
				item.Status = "failed"
				item.Error = "No searchable metadata"
				failed++
			}
		}
		if firstErr == "" && item.Error != "" {
			firstErr = item.Error
		}
		results = append(results, item)
	}

	return results, failed, firstErr
}

func metadataLookupCandidatesForJob(candidates []MetadataCandidate, topMatchOnly bool) []MetadataCandidate {
	if len(candidates) == 0 {
		return nil
	}
	limit := metadataBulkLookupCandidateLimit
	if topMatchOnly {
		limit = 1
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return append([]MetadataCandidate(nil), candidates...)
}

func processMetadataLookupJob(jobID int64, req MetadataLookupJobRequest, title string, results []MetadataLookupJobResultItem, failed int, firstErr string) {
	startedAt := time.Now().Unix()
	res, _ := appDB.Exec(`
		UPDATE metadata_job
		SET status = ?, started_at = ?
		WHERE id = ? AND status = 'queued'
	`, "running", startedAt, jobID)
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return
	}

	completed := 0

	for index := range results {
		if isJobCancelRequested(jobID) {
			finalizeMetadataLookupJobCancelled(jobID, title, req.BookIDs, results, completed, failed)
			return
		}

		item := &results[index]
		if item.Status == "failed" {
			continue
		}

		item.Status = "searching"
		partialJSON, _ := json.Marshal(map[string]any{
			"items":     results,
			"completed": completed,
			"failed":    failed,
			"total":     len(req.BookIDs),
		})
		_, _ = appDB.Exec(`
			UPDATE metadata_job
			SET completed_items = ?, failed_items = ?, result_json = ?
			WHERE id = ?
		`, completed, failed, nullString(partialJSON), jobID)

		snapshot := item.Current
		candidates, diagnostics := searchMetadataCandidatesWithDiagnostics(MetadataSearchFields{
			Title:     snapshot.Title,
			Author:    strings.Join(snapshot.Authors, " "),
			ISBN:      snapshot.ISBN,
			ASIN:      snapshot.ASIN,
			Series:    snapshot.Series,
			Publisher: snapshot.Publisher,
			Provider:  req.Provider,
		})

		if isJobCancelRequested(jobID) {
			item.Status = "cancelled"
			finalizeMetadataLookupJobCancelled(jobID, title, req.BookIDs, results, completed, failed)
			return
		}

		item.Diagnostics = diagnostics
		if len(candidates) > 0 {
			item.Matches = metadataLookupCandidatesForJob(candidates, req.TopMatchOnly)
			item.Match = &item.Matches[0]
			item.Status = "matched"
			item.Error = ""
			completed++
		} else {
			item.Status = "no_match"
			item.Error = metadataDiagnosticsSummary(diagnostics)
			if metadataDiagnosticsAllFailed(diagnostics) {
				recordAppLog("warn", "metadata", "Metadata lookup providers failed for book", map[string]any{
					"job_id":      jobID,
					"book_id":     item.BookID,
					"query":       item.Query,
					"provider":    req.Provider,
					"diagnostics": diagnostics,
				})
			}
			completed++
		}

		partialJSON, _ = json.Marshal(map[string]any{
			"items":     results,
			"completed": completed,
			"failed":    failed,
			"total":     len(req.BookIDs),
		})
		_, _ = appDB.Exec(`
			UPDATE metadata_job
			SET completed_items = ?, failed_items = ?, result_json = ?
			WHERE id = ?
		`, completed, failed, nullString(partialJSON), jobID)
	}

	status := "completed"
	if completed == 0 && failed > 0 {
		status = "failed"
	}

	resultPayload := map[string]any{
		"items":     results,
		"completed": completed,
		"failed":    failed,
		"total":     len(req.BookIDs),
	}
	resultJSON, _ := json.Marshal(resultPayload)
	completedAt := time.Now().Unix()

	_, _ = appDB.Exec(`
		UPDATE metadata_job
		SET status = ?, result_json = ?, error = ?, completed_at = ?
		WHERE id = ?
	`, status, nullString(resultJSON), firstErr, completedAt, jobID)

	matched := 0
	for _, item := range results {
		if item.Match != nil {
			matched++
		}
	}

	createAdminNotification(
		"job_completed",
		title,
		fmt.Sprintf("Metadata lookup finished: %d matches, %d failed.", matched, failed),
		"/settings?tab=jobs",
	)
	recordAppLog("info", "jobs", "Completed metadata lookup job", map[string]any{
		"job_id":  jobID,
		"status":  status,
		"matched": matched,
		"failed":  failed,
		"error":   firstErr,
	})
}

func finalizeMetadataLookupJobCancelled(jobID int64, title string, bookIDs []int64, results []MetadataLookupJobResultItem, completed int, failed int) {
	for index := range results {
		if results[index].Status == "pending" || results[index].Status == "searching" {
			results[index].Status = "cancelled"
		}
	}

	resultPayload := map[string]any{
		"items":     results,
		"completed": completed,
		"failed":    failed,
		"total":     len(bookIDs),
	}
	resultJSON, _ := json.Marshal(resultPayload)
	completedAt := time.Now().Unix()

	_, _ = appDB.Exec(`
		UPDATE metadata_job
		SET status = ?, result_json = ?, completed_items = ?, failed_items = ?, error = ?, completed_at = ?
		WHERE id = ?
	`, "cancelled", nullString(resultJSON), completed, failed, "Cancelled", completedAt, jobID)

	createAdminNotification(
		"job_cancelled",
		title,
		fmt.Sprintf("Metadata lookup cancelled: %d completed, %d failed.", completed, failed),
		"/settings?tab=jobs",
	)
	recordAppLog("info", "jobs", "Cancelled metadata lookup job", map[string]any{
		"job_id":    jobID,
		"completed": completed,
		"failed":    failed,
	})
}

func processMetadataApplyJob(jobID int64, req MetadataApplyJobRequest, title string) {
	startedAt := time.Now().Unix()
	_, _ = appDB.Exec(`
		UPDATE metadata_job
		SET status = ?, started_at = ?
		WHERE id = ?
	`, "running", startedAt, jobID)

	results := make([]MetadataApplyJobResultItem, 0, len(req.Items))
	completed := 0
	failed := 0
	var firstErr string

	for _, item := range req.Items {
		result := MetadataApplyJobResultItem{
			BookID: item.BookID,
			Title:  item.Metadata.Title,
			Status: "applied",
		}

		if err := applyMetadataCandidateToBook(item.BookID, item.Metadata, req.IncludeCover); err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			failed++
			if firstErr == "" {
				firstErr = err.Error()
			}
		} else {
			completed++
		}

		results = append(results, result)
		_, _ = appDB.Exec(`
			UPDATE metadata_job
			SET completed_items = ?, failed_items = ?
			WHERE id = ?
		`, completed, failed, jobID)
	}

	status := "completed"
	if completed == 0 && failed > 0 {
		status = "failed"
	}

	resultPayload := map[string]any{
		"items":     results,
		"completed": completed,
		"failed":    failed,
		"total":     len(req.Items),
	}
	resultJSON, _ := json.Marshal(resultPayload)
	completedAt := time.Now().Unix()

	_, _ = appDB.Exec(`
		UPDATE metadata_job
		SET status = ?, result_json = ?, error = ?, completed_at = ?
		WHERE id = ?
	`, status, nullString(resultJSON), firstErr, completedAt, jobID)

	createAdminNotification(
		"job_completed",
		title,
		fmt.Sprintf("Metadata job finished: %d applied, %d failed.", completed, failed),
		"/settings?tab=jobs",
	)
	recordAppLog("info", "jobs", "Completed metadata apply job", map[string]any{
		"job_id":  jobID,
		"status":  status,
		"applied": completed,
		"failed":  failed,
		"error":   firstErr,
	})
}

func ListJobsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionViewAdmin) && !requirePermission(current, PermissionManageJobs) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	statusFilters := splitCSVFilter(r.URL.Query().Get("status"))
	typeFilters := splitCSVFilter(firstQueryValue(r.URL.Query().Get("type"), r.URL.Query().Get("job_type")))
	limit := 50
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	query := `
		SELECT id, job_type, title, status, payload_json, result_json,
		       total_items, completed_items, failed_items, COALESCE(error, ''),
		       created_at, started_at, completed_at, cancel_requested_at
		FROM metadata_job
	`
	args := []any{}
	conditions := []string{}
	if len(statusFilters) > 0 {
		conditions = append(conditions, "status IN ("+placeholders(len(statusFilters))+")")
		for _, status := range statusFilters {
			args = append(args, status)
		}
	}
	if len(typeFilters) > 0 {
		conditions = append(conditions, "job_type IN ("+placeholders(len(typeFilters))+")")
		for _, jobType := range typeFilters {
			args = append(args, jobType)
		}
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := appDB.Query(query, args...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load jobs")
		return
	}
	defer rows.Close()

	jobs := []AdminJob{}
	for rows.Next() {
		var job AdminJob
		var payload, result sql.NullString
		var startedAt, completedAt, cancelRequestedAt sql.NullInt64
		if err := rows.Scan(
			&job.ID, &job.JobType, &job.Title, &job.Status, &payload, &result,
			&job.TotalItems, &job.CompletedItems, &job.FailedItems, &job.Error,
			&job.CreatedAt, &startedAt, &completedAt, &cancelRequestedAt,
		); err != nil {
			continue
		}
		job.Payload = rawMessageOrNil(payload)
		job.Result = rawMessageOrNil(result)
		job.StartedAt = scanOptionalInt(startedAt)
		job.CompletedAt = scanOptionalInt(completedAt)
		job.CancelRequestedAt = scanOptionalInt(cancelRequestedAt)
		jobs = append(jobs, job)
	}

	jsonResponse(w, http.StatusOK, jobs)
}

func CancelJobHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	jobID, err := strconv.ParseInt(chi.URLParam(r, "jobID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid job ID")
		return
	}

	job, err := loadAdminJob(jobID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Job not found")
		return
	}
	if !requirePermission(current, PermissionManageJobs) && !(job.JobType == "metadata_lookup" && requirePermission(current, PermissionManageMetadata)) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	if !isActiveJobStatus(job.Status) {
		errorResponse(w, http.StatusConflict, "Job is not active")
		return
	}
	if job.Status != "queued" && job.JobType != "library_scan" && job.JobType != "metadata_lookup" {
		errorResponse(w, http.StatusConflict, "This running job type cannot be cancelled yet")
		return
	}

	now := time.Now().Unix()
	if job.Status == "queued" {
		_, err = appDB.Exec(`
			UPDATE metadata_job
			SET status = 'cancelled', cancel_requested_at = ?, completed_at = ?
			WHERE id = ? AND status = 'queued'
		`, now, now, jobID)
	} else {
		_, err = appDB.Exec(`
			UPDATE metadata_job
			SET status = 'cancelling', cancel_requested_at = ?
			WHERE id = ? AND status IN ('running', 'cancelling')
		`, now, jobID)
	}
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to cancel job")
		return
	}

	recordAppLog("info", "jobs", "Requested job cancellation", map[string]any{"job_id": jobID})
	jsonResponse(w, http.StatusAccepted, map[string]string{"status": "cancelling"})
}

func isJobCancelRequested(jobID int64) bool {
	var exists bool
	if err := appDB.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM metadata_job
			WHERE id = ?
			  AND (cancel_requested_at IS NOT NULL OR status = 'cancelling')
		)
	`, jobID).Scan(&exists); err != nil {
		return false
	}
	return exists
}

func DeleteJobHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageJobs) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	jobID, err := strconv.ParseInt(chi.URLParam(r, "jobID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid job ID")
		return
	}

	job, err := loadAdminJob(jobID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Job not found")
		return
	}
	if isActiveJobStatus(job.Status) {
		errorResponse(w, http.StatusConflict, "Cancel active jobs before deleting them")
		return
	}

	_, err = appDB.Exec(`DELETE FROM metadata_job WHERE id = ?`, jobID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to delete job")
		return
	}

	recordAppLog("info", "jobs", "Deleted metadata job", map[string]any{"job_id": jobID})
	jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func ListNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionViewAdmin) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	limit := 20
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	includeUnreadOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("unread")), "true")
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	typeFilters := splitCSVFilter(firstQueryValue(r.URL.Query().Get("type"), r.URL.Query().Get("job_type")))
	searchFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	var startAt, endAt int64
	if value := strings.TrimSpace(r.URL.Query().Get("start")); value != "" {
		startAt, _ = strconv.ParseInt(value, 10, 64)
	}
	if value := strings.TrimSpace(r.URL.Query().Get("end")); value != "" {
		endAt, _ = strconv.ParseInt(value, 10, 64)
	}

	notifications := []AdminNotification{}
	if statusFilter == "" {
		query := `
			SELECT id, kind, title, COALESCE(message, ''), COALESCE(url, ''),
			       read_at, created_at
			FROM app_notification
		`
		args := []any{}
		conditions := []string{}
		if includeUnreadOnly {
			conditions = append(conditions, "read_at IS NULL")
		}
		if startAt > 0 {
			conditions = append(conditions, "created_at >= ?")
			args = append(args, startAt)
		}
		if endAt > 0 {
			conditions = append(conditions, "created_at <= ?")
			args = append(args, endAt)
		}
		if searchFilter != "" {
			conditions = append(conditions, "(LOWER(kind) LIKE ? OR LOWER(title) LIKE ? OR LOWER(COALESCE(message, '')) LIKE ?)")
			like := "%" + searchFilter + "%"
			args = append(args, like, like, like)
		}
		for _, typeFilter := range typeFilters {
			switch typeFilter {
			case "notification":
				// no-op; keep app notifications
			case "auth", "login", "logout", "user":
				conditions = append(conditions, "1 = 0")
			default:
				conditions = append(conditions, "kind = ?")
				args = append(args, typeFilter)
			}
		}
		conditions = append(conditions, `
			kind NOT LIKE 'job_%'
			AND kind NOT LIKE 'library_scan_%'
			AND kind NOT LIKE 'backup_%'
			AND kind NOT LIKE 'cover_regeneration_%'
		`)
		query += " WHERE " + strings.Join(conditions, " AND ")
		query += " ORDER BY created_at DESC LIMIT ?"
		args = append(args, limit)

		rows, err := appDB.Query(query, args...)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to load notifications")
			return
		}
		defer rows.Close()

		for rows.Next() {
			var item AdminNotification
			var readAt sql.NullInt64
			if err := rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Message, &item.URL, &readAt, &item.CreatedAt); err != nil {
				continue
			}
			item.Source = "notification"
			item.ReadAt = scanOptionalInt(readAt)
			notifications = append(notifications, item)
		}

		includeLogs := !includeUnreadOnly
		logCategoryFilters := []string{}
		if len(typeFilters) > 0 {
			includeLogs = false
			for _, typeFilter := range typeFilters {
				switch typeFilter {
				case "auth", "login", "logout", "user":
					includeLogs = true
					logCategoryFilters = append(logCategoryFilters, "auth")
				case "logs", "events":
					includeLogs = true
				}
			}
		}
		if includeLogs {
			logQuery := `
				SELECT id, level, category, message, COALESCE(data_json, ''), created_at
				FROM app_log
			`
			logArgs := []any{}
			logConditions := []string{}
			if len(logCategoryFilters) > 0 {
				logConditions = append(logConditions, "category IN ("+placeholders(len(logCategoryFilters))+")")
				for _, category := range logCategoryFilters {
					logArgs = append(logArgs, category)
				}
			}
			if startAt > 0 {
				logConditions = append(logConditions, "created_at >= ?")
				logArgs = append(logArgs, startAt)
			}
			if endAt > 0 {
				logConditions = append(logConditions, "created_at <= ?")
				logArgs = append(logArgs, endAt)
			}
			if searchFilter != "" {
				logConditions = append(logConditions, "(LOWER(level) LIKE ? OR LOWER(category) LIKE ? OR LOWER(message) LIKE ? OR LOWER(COALESCE(data_json, '')) LIKE ?)")
				like := "%" + searchFilter + "%"
				logArgs = append(logArgs, like, like, like, like)
			}
			if len(logConditions) > 0 {
				logQuery += " WHERE " + strings.Join(logConditions, " AND ")
			}
			logQuery += " ORDER BY created_at DESC LIMIT ?"
			logArgs = append(logArgs, limit)
			logRows, err := appDB.Query(logQuery, logArgs...)
			if err != nil {
				errorResponse(w, http.StatusInternalServerError, "Failed to load log notifications")
				return
			}
			defer logRows.Close()

			for logRows.Next() {
				var logEntry AdminLogEntry
				var data sql.NullString
				if err := logRows.Scan(
					&logEntry.ID, &logEntry.Level, &logEntry.Category, &logEntry.Message,
					&data, &logEntry.CreatedAt,
				); err != nil {
					continue
				}
				logEntry.Data = rawMessageOrNil(data)
				readAt := logEntry.CreatedAt
				notifications = append(notifications, AdminNotification{
					ID:        -1000000000 - logEntry.ID,
					Source:    "log",
					Kind:      logEntry.Level + " · " + logEntry.Category,
					Title:     logEntry.Message,
					Message:   humanizeLogData(logEntry.Data),
					URL:       "",
					ReadAt:    &readAt,
					CreatedAt: logEntry.CreatedAt,
				})
			}
		}
	}

	jobQuery := `
		SELECT id, job_type, title, status, payload_json, result_json,
		       total_items, completed_items, failed_items, COALESCE(error, ''),
		       created_at, started_at, completed_at, cancel_requested_at
		FROM metadata_job
	`
	jobArgs := []any{}
	jobConditions := []string{}
	if statusFilter != "" {
		jobConditions = append(jobConditions, "status = ?")
		jobArgs = append(jobArgs, statusFilter)
	}
	if startAt > 0 {
		jobConditions = append(jobConditions, "created_at >= ?")
		jobArgs = append(jobArgs, startAt)
	}
	if endAt > 0 {
		jobConditions = append(jobConditions, "created_at <= ?")
		jobArgs = append(jobArgs, endAt)
	}
	if searchFilter != "" {
		jobConditions = append(jobConditions, "(LOWER(job_type) LIKE ? OR LOWER(title) LIKE ? OR LOWER(status) LIKE ? OR LOWER(COALESCE(error, '')) LIKE ? OR LOWER(COALESCE(payload_json, '')) LIKE ? OR LOWER(COALESCE(result_json, '')) LIKE ?)")
		like := "%" + searchFilter + "%"
		jobArgs = append(jobArgs, like, like, like, like, like, like)
	}
	if len(typeFilters) > 0 {
		jobConditions = append(jobConditions, "job_type IN ("+placeholders(len(typeFilters))+")")
		for _, jobType := range typeFilters {
			jobArgs = append(jobArgs, jobType)
		}
	}
	if includeUnreadOnly {
		jobConditions = append(jobConditions, "status IN ('queued', 'running', 'cancelling')")
	}
	if len(jobConditions) > 0 {
		jobQuery += " WHERE " + strings.Join(jobConditions, " AND ")
	}
	jobQuery += " ORDER BY created_at DESC LIMIT ?"
	jobArgs = append(jobArgs, limit)

	jobRows, err := appDB.Query(jobQuery, jobArgs...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load job notifications")
		return
	}
	defer jobRows.Close()

	for jobRows.Next() {
		var job AdminJob
		var payload, result sql.NullString
		var startedAt, completedAt, cancelRequestedAt sql.NullInt64
		if err := jobRows.Scan(
			&job.ID, &job.JobType, &job.Title, &job.Status, &payload, &result,
			&job.TotalItems, &job.CompletedItems, &job.FailedItems, &job.Error,
			&job.CreatedAt, &startedAt, &completedAt, &cancelRequestedAt,
		); err != nil {
			continue
		}
		job.Payload = rawMessageOrNil(payload)
		job.Result = rawMessageOrNil(result)
		job.StartedAt = scanOptionalInt(startedAt)
		job.CompletedAt = scanOptionalInt(completedAt)
		job.CancelRequestedAt = scanOptionalInt(cancelRequestedAt)
		jobCopy := job
		notifications = append(notifications, AdminNotification{
			ID:        -job.ID,
			Source:    "job",
			Kind:      job.JobType,
			Title:     job.Title,
			Message:   jobNotificationMessage(job),
			URL:       "/settings?tab=jobs",
			ReadAt:    jobNotificationReadAt(job),
			CreatedAt: job.CreatedAt,
			Job:       &jobCopy,
		})
	}

	sort.SliceStable(notifications, func(i, j int) bool {
		return notifications[i].CreatedAt > notifications[j].CreatedAt
	})
	if len(notifications) > limit {
		notifications = notifications[:limit]
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, item := range notifications {
			_, _ = w.Write([]byte(notificationTextLine(item) + "\n"))
		}
		return
	}

	var unreadCount int64
	_ = appDB.QueryRow(`
		SELECT COUNT(*)
		FROM app_notification
		WHERE read_at IS NULL
		  AND kind NOT LIKE 'job_%'
		  AND kind NOT LIKE 'library_scan_%'
		  AND kind NOT LIKE 'backup_%'
		  AND kind NOT LIKE 'cover_regeneration_%'
	`).Scan(&unreadCount)
	var activeJobCount int64
	_ = appDB.QueryRow(`SELECT COUNT(*) FROM metadata_job WHERE status IN ('queued', 'running', 'cancelling')`).Scan(&activeJobCount)
	unreadCount += activeJobCount

	jsonResponse(w, http.StatusOK, map[string]any{
		"items":        notifications,
		"unread_count": unreadCount,
	})
}

func MarkNotificationReadHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionViewAdmin) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	notificationID, err := strconv.ParseInt(chi.URLParam(r, "notificationID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid notification ID")
		return
	}

	_, err = appDB.Exec(`UPDATE app_notification SET read_at = ? WHERE id = ? AND read_at IS NULL`, time.Now().Unix(), notificationID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to mark notification read")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func DeleteNotificationHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionViewAdmin) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	notificationID, err := strconv.ParseInt(chi.URLParam(r, "notificationID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid notification ID")
		return
	}

	_, err = appDB.Exec(`DELETE FROM app_notification WHERE id = ?`, notificationID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to delete notification")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func DeleteAllNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionViewAdmin) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	result, err := appDB.Exec(`DELETE FROM app_notification`)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to delete notifications")
		return
	}

	deleted, _ := result.RowsAffected()
	jsonResponse(w, http.StatusOK, map[string]any{
		"status":  "deleted",
		"deleted": deleted,
	})
}

func ListLogsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionViewLogs) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	limit := 100
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	levelFilter := strings.TrimSpace(r.URL.Query().Get("level"))
	categoryFilter := strings.TrimSpace(r.URL.Query().Get("category"))
	queryFilter := strings.TrimSpace(r.URL.Query().Get("q"))
	fromFilter := parseAdminTimeFilter(strings.TrimSpace(r.URL.Query().Get("from")))
	toFilter := parseAdminTimeFilter(strings.TrimSpace(r.URL.Query().Get("to")))

	query := `
		SELECT id, level, category, message, COALESCE(data_json, ''), created_at
		FROM app_log
		WHERE 1 = 1
	`
	args := []any{}
	if levelFilter != "" {
		query += " AND level = ?"
		args = append(args, levelFilter)
	}
	if categoryFilter != "" {
		query += " AND category = ?"
		args = append(args, categoryFilter)
	}
	if queryFilter != "" {
		query += " AND (message LIKE ? OR COALESCE(data_json, '') LIKE ?)"
		search := "%" + queryFilter + "%"
		args = append(args, search, search)
	}
	if fromFilter > 0 {
		query += " AND created_at >= ?"
		args = append(args, fromFilter)
	}
	if toFilter > 0 {
		query += " AND created_at <= ?"
		args = append(args, toFilter)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := appDB.Query(query, args...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load logs")
		return
	}
	defer rows.Close()

	logs := []AdminLogEntry{}
	for rows.Next() {
		var item AdminLogEntry
		var data sql.NullString
		if err := rows.Scan(&item.ID, &item.Level, &item.Category, &item.Message, &data, &item.CreatedAt); err != nil {
			continue
		}
		item.Data = rawMessageOrNil(data)
		logs = append(logs, item)
	}

	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("format")), "text") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, item := range logs {
			line := fmt.Sprintf("%s [%s] [%s] %s", time.Unix(item.CreatedAt, 0).Format(time.RFC3339), item.Level, item.Category, item.Message)
			if len(item.Data) > 0 {
				line += " " + string(item.Data)
			}
			_, _ = w.Write([]byte(line + "\n"))
		}
		return
	}

	jsonResponse(w, http.StatusOK, logs)
}

func parseAdminTimeFilter(value string) int64 {
	if value == "" {
		return 0
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed.Unix()
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.Unix()
	}
	return 0
}
