package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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

type AdminJob struct {
	ID             int64           `json:"id"`
	JobType        string          `json:"job_type"`
	Title          string          `json:"title"`
	Status         string          `json:"status"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	Result         json.RawMessage `json:"result,omitempty"`
	TotalItems     int             `json:"total_items"`
	CompletedItems int             `json:"completed_items"`
	FailedItems    int             `json:"failed_items"`
	Error          string          `json:"error,omitempty"`
	CreatedAt      int64           `json:"created_at"`
	StartedAt      *int64          `json:"started_at,omitempty"`
	CompletedAt    *int64          `json:"completed_at,omitempty"`
}

type AdminNotification struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Message   string `json:"message,omitempty"`
	URL       string `json:"url,omitempty"`
	ReadAt    *int64 `json:"read_at,omitempty"`
	CreatedAt int64  `json:"created_at"`
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
	var startedAt, completedAt sql.NullInt64
	err := appDB.QueryRow(`
		SELECT id, job_type, title, status, payload_json, result_json,
		       total_items, completed_items, failed_items, COALESCE(error, ''),
		       created_at, started_at, completed_at
		FROM metadata_job
		WHERE id = ?
	`, jobID).Scan(
		&job.ID, &job.JobType, &job.Title, &job.Status, &payload, &result,
		&job.TotalItems, &job.CompletedItems, &job.FailedItems, &job.Error,
		&job.CreatedAt, &startedAt, &completedAt,
	)
	if err != nil {
		return AdminJob{}, err
	}

	job.Payload = rawMessageOrNil(payload)
	job.Result = rawMessageOrNil(result)
	job.StartedAt = scanOptionalInt(startedAt)
	job.CompletedAt = scanOptionalInt(completedAt)
	return job, nil
}

func applyMetadataCandidateToBook(bookID int64, candidate MetadataCandidate, includeCover bool) error {
	var exists bool
	if err := appDB.QueryRow("SELECT EXISTS(SELECT 1 FROM book WHERE id = ?)", bookID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("book not found")
	}

	authorsJSON, _ := json.Marshal(candidate.Authors)
	genresJSON, _ := json.Marshal(candidate.Genres)

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
		ISBN         string
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
		       COALESCE(isbn, ''),
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
		&current.ISBN,
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
				pub_date, description, rating, genres, isbn, cover_path,
				cover_updated_on, page_count, language, locked_fields
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, bookID, candidate.Title, nullString(authorsJSON), candidate.Series,
			0, candidate.Publisher, candidate.PubDate, candidate.Description,
			candidate.Rating, nullString(genresJSON), candidate.ISBN, "",
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

		finalGenres := current.Genres
		if len(candidate.Genres) > 0 && !contains(locked, "genres") {
			finalGenres = nullString(genresJSON).(string)
		}

		finalISBN := current.ISBN
		if candidate.ISBN != "" && !contains(locked, "isbn") {
			finalISBN = candidate.ISBN
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
				genres = ?,
				isbn = ?,
				page_count = ?,
				language = ?
			WHERE book_id = ?
		`, finalTitle, finalAuthors, finalSeries, finalSeriesNumber, finalPublisher,
			finalPubDate, finalDescription, finalRating, finalGenres, finalISBN,
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
		"/settings?tab=admin",
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
		"/settings?tab=admin",
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

	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := 50
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	query := `
		SELECT id, job_type, title, status, payload_json, result_json,
		       total_items, completed_items, failed_items, COALESCE(error, ''),
		       created_at, started_at, completed_at
		FROM metadata_job
	`
	args := []any{}
	if statusFilter != "" {
		query += " WHERE status = ?"
		args = append(args, statusFilter)
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
		var startedAt, completedAt sql.NullInt64
		if err := rows.Scan(
			&job.ID, &job.JobType, &job.Title, &job.Status, &payload, &result,
			&job.TotalItems, &job.CompletedItems, &job.FailedItems, &job.Error,
			&job.CreatedAt, &startedAt, &completedAt,
		); err != nil {
			continue
		}
		job.Payload = rawMessageOrNil(payload)
		job.Result = rawMessageOrNil(result)
		job.StartedAt = scanOptionalInt(startedAt)
		job.CompletedAt = scanOptionalInt(completedAt)
		jobs = append(jobs, job)
	}

	jsonResponse(w, http.StatusOK, jobs)
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

	query := `
		SELECT id, kind, title, COALESCE(message, ''), COALESCE(url, ''),
		       read_at, created_at
		FROM app_notification
	`
	args := []any{}
	if includeUnreadOnly {
		query += " WHERE read_at IS NULL"
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := appDB.Query(query, args...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load notifications")
		return
	}
	defer rows.Close()

	notifications := []AdminNotification{}
	for rows.Next() {
		var item AdminNotification
		var readAt sql.NullInt64
		if err := rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Message, &item.URL, &readAt, &item.CreatedAt); err != nil {
			continue
		}
		item.ReadAt = scanOptionalInt(readAt)
		notifications = append(notifications, item)
	}

	var unreadCount int64
	_ = appDB.QueryRow(`SELECT COUNT(*) FROM app_notification WHERE read_at IS NULL`).Scan(&unreadCount)

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
