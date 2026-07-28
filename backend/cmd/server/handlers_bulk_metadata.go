package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cryptorum/internal/metaprotection"
	"cryptorum/internal/seriesnum"
)

type bulkMetadataRequest struct {
	BookIDs      []int64            `json:"book_ids"`
	Filter       *bulkFilterRequest `json:"filter,omitempty"`
	Authors      *[]string          `json:"authors,omitempty"`
	Publisher    *string            `json:"publisher,omitempty"`
	Language     *string            `json:"language,omitempty"`
	Series       *string            `json:"series,omitempty"`
	SeriesNumber json.RawMessage    `json:"series_number,omitempty"`
	Status       *string            `json:"status,omitempty"`
	Rating       *float64           `json:"rating,omitempty"`
	AddGenres    []string           `json:"add_genres,omitempty"`
	RemoveGenres []string           `json:"remove_genres,omitempty"`
	AddTags      []string           `json:"add_tags,omitempty"`
	RemoveTags   []string           `json:"remove_tags,omitempty"`
	ClearFields  []string           `json:"clear_fields,omitempty"`
}

type bulkMetadataJobResultItem struct {
	BookID int64  `json:"book_id"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func bulkUpdateMetadataHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req bulkMetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	bookIDs, err := resolveBulkMetadataBookIDs(current, req)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to resolve selected books")
		return
	}
	if len(bookIDs) == 0 {
		errorResponse(w, http.StatusBadRequest, "No books selected")
		return
	}
	req.BookIDs = bookIDs
	req.Filter = nil

	if err := validateBulkMetadataRequest(req); err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	title := fmt.Sprintf("Bulk metadata update (%d books)", len(req.BookIDs))
	payload, _ := json.Marshal(req)
	now := time.Now().Unix()
	res, err := appDB.Exec(`
		INSERT INTO metadata_job (
			job_type, title, status, payload_json,
			total_items, completed_items, failed_items,
			created_at
		) VALUES (?, ?, ?, ?, ?, 0, 0, ?)
	`, "bulk_metadata_update", title, "queued", nullString(payload), len(req.BookIDs), now)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to queue bulk metadata update")
		return
	}

	jobID, _ := res.LastInsertId()
	createAdminNotification("job_queued", title, "Queued a background bulk metadata update job.", "/settings?tab=jobs")
	recordAppLog("info", "jobs", "Queued bulk metadata update job", map[string]any{
		"job_id": jobID,
		"count":  len(req.BookIDs),
	})

	go processBulkMetadataUpdateJob(jobID, req, current.ID, title)

	job, err := loadAdminJob(jobID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load queued job")
		return
	}
	jsonResponse(w, http.StatusAccepted, job)
}

func resolveBulkMetadataBookIDs(current *AppUser, req bulkMetadataRequest) ([]int64, error) {
	if req.Filter != nil {
		query, args := buildBulkFilterQuery(current, *req.Filter)
		rows, err := appDB.Query(query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		bookIDs := []int64{}
		for rows.Next() {
			var bookID int64
			if err := rows.Scan(&bookID); err != nil {
				return nil, err
			}
			bookIDs = append(bookIDs, bookID)
		}
		return bookIDs, rows.Err()
	}

	seen := map[int64]bool{}
	bookIDs := make([]int64, 0, len(req.BookIDs))
	for _, bookID := range req.BookIDs {
		if bookID <= 0 || seen[bookID] {
			continue
		}
		allowed, err := canAccessBook(current, bookID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		seen[bookID] = true
		bookIDs = append(bookIDs, bookID)
	}
	return bookIDs, nil
}

func validateBulkMetadataRequest(req bulkMetadataRequest) error {
	updateSeriesNumber := len(req.SeriesNumber) > 0 && string(req.SeriesNumber) != "null" && strings.TrimSpace(string(req.SeriesNumber)) != `""`
	if updateSeriesNumber {
		if _, _, err := seriesnum.ParseJSON(req.SeriesNumber); err != nil {
			return err
		}
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		if status != "" && status != "unread" && status != "reading" && status != "finished" {
			return fmt.Errorf("invalid reading status")
		}
	}
	return nil
}

func processBulkMetadataUpdateJob(jobID int64, req bulkMetadataRequest, ownerUserID int64, title string) {
	startedAt := time.Now().Unix()
	_, _ = appDB.Exec(`
		UPDATE metadata_job
		SET status = ?, started_at = ?
		WHERE id = ?
	`, "running", startedAt, jobID)

	results := make([]bulkMetadataJobResultItem, 0, len(req.BookIDs))
	completed := 0
	failed := 0
	var firstErr string

	for _, bookID := range req.BookIDs {
		result := bulkMetadataJobResultItem{
			BookID: bookID,
			Title:  bulkMetadataBookTitle(bookID),
			Status: "updated",
		}

		if err := applyBulkMetadataUpdate(bookID, ownerUserID, req); err != nil {
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

	createAdminNotification(
		"job_completed",
		title,
		fmt.Sprintf("Bulk metadata update finished: %d updated, %d failed.", completed, failed),
		"/settings?tab=jobs",
	)
	recordAppLog("info", "jobs", "Completed bulk metadata update job", map[string]any{
		"job_id":  jobID,
		"status":  status,
		"updated": completed,
		"failed":  failed,
		"error":   firstErr,
	})
}

func bulkMetadataBookTitle(bookID int64) string {
	var title string
	_ = appDB.QueryRow(`SELECT COALESCE(title, '') FROM book_metadata WHERE book_id = ?`, bookID).Scan(&title)
	return title
}

func applyBulkMetadataUpdate(bookID int64, ownerUserID int64, req bulkMetadataRequest) error {
	clear := make(map[string]bool)
	for _, field := range req.ClearFields {
		clear[strings.ToLower(strings.TrimSpace(field))] = true
	}

	var parsedSeriesNumber float64
	var parsedSeriesNumberDisplay string
	updateSeriesNumber := len(req.SeriesNumber) > 0 && string(req.SeriesNumber) != "null" && strings.TrimSpace(string(req.SeriesNumber)) != `""`
	if updateSeriesNumber {
		value, display, err := seriesnum.ParseJSON(req.SeriesNumber)
		if err != nil {
			return err
		}
		parsedSeriesNumber = value
		parsedSeriesNumberDisplay = display
	}

	tx, err := appDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	current, metadataExists, err := metaprotection.LoadSnapshot(tx, bookID)
	if err != nil {
		return err
	}
	touchedFields := []string{}
	if req.Authors != nil || clear["authors"] {
		touchedFields = append(touchedFields, metaprotection.FieldAuthors)
	}
	if req.Publisher != nil || clear["publisher"] {
		touchedFields = append(touchedFields, metaprotection.FieldPublisher)
	}
	if req.Language != nil || clear["language"] {
		touchedFields = append(touchedFields, metaprotection.FieldLanguage)
	}
	if req.Series != nil || clear["series"] {
		touchedFields = append(touchedFields, metaprotection.FieldSeries)
	}
	if updateSeriesNumber || clear["series_number"] || clear["series"] {
		touchedFields = append(touchedFields, metaprotection.FieldSeriesNumber)
	}
	if req.Rating != nil || clear["rating"] {
		touchedFields = append(touchedFields, metaprotection.FieldRating)
	}
	if len(req.AddGenres) > 0 || len(req.RemoveGenres) > 0 ||
		len(req.AddTags) > 0 || len(req.RemoveTags) > 0 ||
		clear["genres"] || clear["tags"] {
		touchedFields = append(touchedFields, metaprotection.FieldTags)
	}
	touchedFields = metaprotection.NormalizeFields(touchedFields)
	if metadataExists && len(touchedFields) > 0 {
		if err := metaprotection.RecordRevision(tx, current, touchedFields, "bulk_edit", ownerUserID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO book_metadata (book_id, authors, genres, tags, locked_fields, owner_user_id)
		VALUES (?, '[]', '[]', '[]', '[]', ?)
		ON CONFLICT(book_id) DO NOTHING
	`, bookID, ownerUserID); err != nil {
		return err
	}

	if req.Authors != nil || clear["authors"] {
		authors := []string{}
		if req.Authors != nil && !clear["authors"] {
			authors = normalizeMetadataStringList(*req.Authors)
		}
		authorsJSON, _ := json.Marshal(authors)
		if _, err := tx.Exec(`UPDATE book_metadata SET authors = ? WHERE book_id = ?`, string(authorsJSON), bookID); err != nil {
			return err
		}
	}
	if req.Publisher != nil || clear["publisher"] {
		value := ""
		if req.Publisher != nil && !clear["publisher"] {
			value = strings.TrimSpace(*req.Publisher)
		}
		if _, err := tx.Exec(`UPDATE book_metadata SET publisher = ? WHERE book_id = ?`, value, bookID); err != nil {
			return err
		}
	}
	if req.Language != nil || clear["language"] {
		value := ""
		if req.Language != nil && !clear["language"] {
			value = strings.TrimSpace(*req.Language)
		}
		if _, err := tx.Exec(`UPDATE book_metadata SET language = ? WHERE book_id = ?`, value, bookID); err != nil {
			return err
		}
	}
	if req.Series != nil || clear["series"] {
		value := ""
		if req.Series != nil && !clear["series"] {
			value = strings.TrimSpace(*req.Series)
		}
		if _, err := tx.Exec(`UPDATE book_metadata SET series = ? WHERE book_id = ?`, value, bookID); err != nil {
			return err
		}
	}
	if updateSeriesNumber || clear["series_number"] || clear["series"] {
		value := parsedSeriesNumber
		display := parsedSeriesNumberDisplay
		if clear["series_number"] || clear["series"] {
			value = 0
			display = ""
		}
		if _, err := tx.Exec(`UPDATE book_metadata SET series_number = ?, series_number_display = ? WHERE book_id = ?`, value, display, bookID); err != nil {
			return err
		}
	}
	if req.Rating != nil || clear["rating"] {
		value := 0.0
		if req.Rating != nil && !clear["rating"] {
			value = *req.Rating
		}
		if _, err := tx.Exec(`UPDATE book_metadata SET rating = ? WHERE book_id = ?`, value, bookID); err != nil {
			return err
		}
	}
	if req.Status != nil || clear["status"] {
		status := "unread"
		if req.Status != nil && !clear["status"] {
			status = strings.TrimSpace(*req.Status)
		}
		if status != "unread" && status != "reading" && status != "finished" {
			return fmt.Errorf("invalid reading status")
		}
		if _, err := tx.Exec(`
				INSERT INTO reading_progress (book_id, status, percent, updated_at, owner_user_id)
				VALUES (?, ?, 0, 0, ?)
				ON CONFLICT(book_id, owner_user_id) DO UPDATE SET
					status = excluded.status
			`, bookID, status, ownerUserID); err != nil {
			return err
		}
	}
	if len(req.AddGenres) > 0 || len(req.RemoveGenres) > 0 || clear["genres"] {
		if err := bulkUpdateBookJSONList(tx, bookID, "genres", req.AddGenres, req.RemoveGenres, clear["genres"]); err != nil {
			return err
		}
	}
	addTags := append([]string{}, req.AddGenres...)
	addTags = append(addTags, req.AddTags...)
	removeTags := append([]string{}, req.RemoveGenres...)
	removeTags = append(removeTags, req.RemoveTags...)
	if len(addTags) > 0 || len(removeTags) > 0 || clear["tags"] {
		if err := bulkUpdateBookJSONList(tx, bookID, "tags", addTags, removeTags, clear["tags"]); err != nil {
			return err
		}
	}
	if len(touchedFields) > 0 {
		lockedFields := metaprotection.MergeLocked(current.LockedFields, touchedFields...)
		if _, err := tx.Exec(`
			UPDATE book_metadata
			SET locked_fields = ?, metadata_updated_at = ?
			WHERE book_id = ?
		`, lockedFields, time.Now().Unix(), bookID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

func bulkUpdateBookJSONList(tx sqlExecer, bookID int64, column string, addValues []string, removeValues []string, clear bool) error {
	if column != "genres" && column != "tags" {
		return nil
	}

	values := []string{}
	if !clear {
		var raw string
		_ = tx.QueryRow(`SELECT COALESCE(`+column+`, '[]') FROM book_metadata WHERE book_id = ?`, bookID).Scan(&raw)
		_ = json.Unmarshal([]byte(raw), &values)
	}

	removeSet := make(map[string]bool)
	for _, value := range cleanedStringList(removeValues) {
		removeSet[strings.ToLower(value)] = true
	}

	next := []string{}
	for _, value := range values {
		if !removeSet[strings.ToLower(value)] {
			next = append(next, value)
		}
	}
	next = append(next, cleanedStringList(addValues)...)
	if column == "tags" {
		next = normalizeMetadataTagList(next)
	} else {
		next = normalizeMetadataStringList(next)
	}
	valuesJSON, _ := json.Marshal(next)
	_, err := tx.Exec(`UPDATE book_metadata SET `+column+` = ? WHERE book_id = ?`, string(valuesJSON), bookID)
	return err
}
