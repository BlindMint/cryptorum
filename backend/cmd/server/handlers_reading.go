package main

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// ReadingProgress represents reading progress data
type ReadingProgress struct {
	ID                   int64   `json:"id"`
	BookID               int64   `json:"book_id"`
	FileID               *int64  `json:"file_id,omitempty"`
	Percent              float64 `json:"percent"`
	CFI                  string  `json:"cfi,omitempty"`
	Page                 int     `json:"page,omitempty"`
	Status               string  `json:"status"`
	SpeedReaderWordIndex int     `json:"speed_reader_word_index,omitempty"`
	SpeedReaderPercent   float64 `json:"speed_reader_percent,omitempty"`
	UpdatedAt            int64   `json:"updated_at"`
}

// UpdateReadingProgressRequest represents a request to update reading progress
type UpdateReadingProgressRequest struct {
	FileID  *int64  `json:"file_id,omitempty"`
	Percent float64 `json:"percent"`
	CFI     string  `json:"cfi,omitempty"`
	Page    int     `json:"page,omitempty"`
	Status  string  `json:"status"`
}

// Annotation represents a book annotation
type Annotation struct {
	ID           int64  `json:"id"`
	BookID       int64  `json:"book_id"`
	FileID       *int64 `json:"file_id,omitempty"`
	CFIStart     string `json:"cfi_start,omitempty"`
	CFIEnd       string `json:"cfi_end,omitempty"`
	SelectedText string `json:"selected_text,omitempty"`
	Note         string `json:"note,omitempty"`
	Color        string `json:"color,omitempty"`
	CreatedAt    int64  `json:"created_at"`
}

// Bookmark represents a book bookmark
type Bookmark struct {
	ID        int64  `json:"id"`
	BookID    int64  `json:"book_id"`
	FileID    *int64 `json:"file_id,omitempty"`
	CFI       string `json:"cfi,omitempty"`
	Label     string `json:"label,omitempty"`
	Color     string `json:"color,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// CreateAnnotationRequest represents a request to create an annotation
type CreateAnnotationRequest struct {
	FileID       *int64 `json:"file_id,omitempty"`
	CFIStart     string `json:"cfi_start,omitempty"`
	CFIEnd       string `json:"cfi_end,omitempty"`
	SelectedText string `json:"selected_text,omitempty"`
	Note         string `json:"note,omitempty"`
	Color        string `json:"color,omitempty"`
}

// CreateBookmarkRequest represents a request to create a bookmark
type CreateBookmarkRequest struct {
	FileID *int64 `json:"file_id,omitempty"`
	CFI    string `json:"cfi,omitempty"`
	Label  string `json:"label,omitempty"`
	Color  string `json:"color,omitempty"`
}

// ReadingSession represents a reading session
type ReadingSession struct {
	ID         int64  `json:"id"`
	BookID     int64  `json:"book_id"`
	ReaderType string `json:"reader_type"`
	StartedAt  int64  `json:"started_at"`
	EndedAt    *int64 `json:"ended_at,omitempty"`
}

func closeStaleReadingSessions(cutoff int64) (int64, error) {
	if cutoff <= 0 {
		return 0, nil
	}

	result, err := appDB.Exec(`
		UPDATE reading_session
		SET ended_at = ?
		WHERE ended_at IS NULL AND started_at <= ?
	`, cutoff, cutoff)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return rowsAffected, nil
}

// GetReadingProgressHandler gets reading progress for a book
func GetReadingProgressHandler(w http.ResponseWriter, r *http.Request) {
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

	progress, err := loadReadingProgress(bookID, current.ID)
	if err != nil {
		slog.Error("GetReadingProgressHandler failed", "book_id", bookID, "error", err)
		errorResponse(w, http.StatusInternalServerError, "Failed to get reading progress")
		return
	}

	jsonResponse(w, http.StatusOK, progress)
}

func loadReadingProgress(bookID string, ownerUserID int64) (ReadingProgress, error) {
	const maxAttempts = 5

	for attempt := 0; attempt < maxAttempts; attempt++ {
		var progress ReadingProgress
		var fileID sql.NullInt64
		var percent sql.NullFloat64
		var cfi sql.NullString
		var page sql.NullInt64
		var status sql.NullString
		var speedReaderWordIndex sql.NullInt64
		var speedReaderPercent sql.NullFloat64
		var updatedAt sql.NullInt64

		err := appDB.QueryRow(`
			SELECT id, book_id, file_id, percent, cfi, page, status,
			       speed_reader_word_index, speed_reader_percent, updated_at
			FROM reading_progress
			WHERE book_id = ? AND owner_user_id = ?
		`, bookID, ownerUserID).Scan(
			&progress.ID, &progress.BookID, &fileID, &percent,
			&cfi, &page, &status, &speedReaderWordIndex,
			&speedReaderPercent, &updatedAt,
		)

		if err == sql.ErrNoRows {
			id, parseErr := strconv.ParseInt(bookID, 10, 64)
			if parseErr != nil {
				return ReadingProgress{}, parseErr
			}
			return ReadingProgress{
				BookID: id,
				Status: "unread",
			}, nil
		}

		if err != nil {
			if isSQLiteBusyError(err) && attempt < maxAttempts-1 {
				time.Sleep(time.Duration(50*(1<<attempt)) * time.Millisecond)
				continue
			}
			return ReadingProgress{}, err
		}

		if fileID.Valid {
			progress.FileID = &fileID.Int64
		}
		if percent.Valid {
			progress.Percent = percent.Float64
		}
		if cfi.Valid {
			progress.CFI = cfi.String
		}
		if page.Valid {
			progress.Page = int(page.Int64)
		}
		if status.Valid && status.String != "" {
			progress.Status = status.String
		} else {
			progress.Status = "unread"
		}
		if speedReaderWordIndex.Valid {
			progress.SpeedReaderWordIndex = int(speedReaderWordIndex.Int64)
		}
		if speedReaderPercent.Valid {
			progress.SpeedReaderPercent = speedReaderPercent.Float64
		}
		if updatedAt.Valid {
			progress.UpdatedAt = updatedAt.Int64
		}

		return progress, nil
	}

	return ReadingProgress{}, sql.ErrConnDone
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "database table is locked")
}

// UpdateReadingProgressHandler updates reading progress for a book
func UpdateReadingProgressHandler(w http.ResponseWriter, r *http.Request) {
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

	var req UpdateReadingProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	now := time.Now().Unix()

	// Check if progress exists
	var exists bool
	err = appDB.QueryRow("SELECT EXISTS(SELECT 1 FROM reading_progress WHERE book_id = ? AND owner_user_id = ?)", bookID, current.ID).Scan(&exists)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to check reading progress")
		return
	}

	if exists {
		// Update existing progress
		_, err = appDB.Exec(`
			UPDATE reading_progress 
			SET file_id = ?, percent = ?, cfi = ?, page = ?, status = ?, updated_at = ?
			WHERE book_id = ? AND owner_user_id = ?
		`, req.FileID, req.Percent, req.CFI, req.Page, req.Status, now, bookID, current.ID)
	} else {
		// Create new progress
		bookIDInt, _ := strconv.ParseInt(bookID, 10, 64)
		_, err = appDB.Exec(`
			INSERT INTO reading_progress (book_id, file_id, percent, cfi, page, status, updated_at, owner_user_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, bookIDInt, req.FileID, req.Percent, req.CFI, req.Page, req.Status, now, current.ID)
	}

	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update reading progress")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateSpeedReaderProgressHandler updates speed reader progress
func UpdateSpeedReaderProgressHandler(w http.ResponseWriter, r *http.Request) {
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
		WordIndex int     `json:"word_index"`
		Percent   float64 `json:"percent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	now := time.Now().Unix()
	_, err = appDB.Exec(`
		INSERT INTO reading_progress (
			book_id, status, percent, speed_reader_word_index, speed_reader_percent, updated_at, owner_user_id
		)
		VALUES (?, 'reading', 0, ?, ?, ?, ?)
		ON CONFLICT(book_id) DO UPDATE SET
			speed_reader_word_index = excluded.speed_reader_word_index,
			speed_reader_percent = excluded.speed_reader_percent,
			updated_at = excluded.updated_at
	`, bookIDInt, req.WordIndex, req.Percent, now, current.ID)

	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update speed reader progress")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetAnnotationsHandler gets all annotations for a book
func GetAnnotationsHandler(w http.ResponseWriter, r *http.Request) {
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
		SELECT id, book_id, file_id, cfi_start, cfi_end, selected_text, note, color, created_at
		FROM annotation
		WHERE book_id = ? AND owner_user_id = ?
		ORDER BY created_at DESC
	`, bookID, current.ID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to get annotations")
		return
	}
	defer rows.Close()

	annotations := []Annotation{}
	for rows.Next() {
		var a Annotation
		var fileID sql.NullInt64
		var cfiStart, cfiEnd, selectedText, note, color sql.NullString
		var createdAt int64

		if err := rows.Scan(&a.ID, &a.BookID, &fileID, &cfiStart, &cfiEnd,
			&selectedText, &note, &color, &createdAt); err != nil {
			continue
		}

		if fileID.Valid {
			a.FileID = &fileID.Int64
		}
		if cfiStart.Valid {
			a.CFIStart = cfiStart.String
		}
		if cfiEnd.Valid {
			a.CFIEnd = cfiEnd.String
		}
		if selectedText.Valid {
			a.SelectedText = selectedText.String
		}
		if note.Valid {
			a.Note = note.String
		}
		if color.Valid {
			a.Color = color.String
		}
		a.CreatedAt = createdAt

		annotations = append(annotations, a)
	}

	jsonResponse(w, http.StatusOK, annotations)
}

// CreateAnnotationHandler creates a new annotation
func CreateAnnotationHandler(w http.ResponseWriter, r *http.Request) {
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

	var req CreateAnnotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	now := time.Now().Unix()

	result, err := appDB.Exec(`
		INSERT INTO annotation (book_id, file_id, cfi_start, cfi_end, selected_text, note, color, created_at, owner_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, bookIDInt, req.FileID, req.CFIStart, req.CFIEnd, req.SelectedText, req.Note, req.Color, now, current.ID)

	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to create annotation")
		return
	}

	id, _ := result.LastInsertId()
	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"id":     id,
		"status": "created",
	})
}

// DeleteAnnotationHandler deletes an annotation
func DeleteAnnotationHandler(w http.ResponseWriter, r *http.Request) {
	annotationID := chi.URLParam(r, "id")
	current := getUserFromContext(r.Context())

	_, err := appDB.Exec("DELETE FROM annotation WHERE id = ? AND owner_user_id = ?", annotationID, current.ID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to delete annotation")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetBookmarksHandler gets all bookmarks for a book
func GetBookmarksHandler(w http.ResponseWriter, r *http.Request) {
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
		SELECT id, book_id, file_id, cfi, label, color, created_at
		FROM bookmark
		WHERE book_id = ? AND owner_user_id = ?
		ORDER BY created_at DESC
	`, bookID, current.ID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to get bookmarks")
		return
	}
	defer rows.Close()

	bookmarks := []Bookmark{}
	for rows.Next() {
		var b Bookmark
		var fileID sql.NullInt64
		var cfi, label, color sql.NullString
		var createdAt int64

		if err := rows.Scan(&b.ID, &b.BookID, &fileID, &cfi, &label, &color, &createdAt); err != nil {
			continue
		}

		if fileID.Valid {
			b.FileID = &fileID.Int64
		}
		if cfi.Valid {
			b.CFI = cfi.String
		}
		if label.Valid {
			b.Label = label.String
		}
		if color.Valid {
			b.Color = color.String
		}
		b.CreatedAt = createdAt

		bookmarks = append(bookmarks, b)
	}

	jsonResponse(w, http.StatusOK, bookmarks)
}

// CreateBookmarkHandler creates a new bookmark
func CreateBookmarkHandler(w http.ResponseWriter, r *http.Request) {
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

	var req CreateBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	now := time.Now().Unix()

	result, err := appDB.Exec(`
		INSERT INTO bookmark (book_id, file_id, cfi, label, color, created_at, owner_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, bookIDInt, req.FileID, req.CFI, req.Label, req.Color, now, current.ID)

	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to create bookmark")
		return
	}

	id, _ := result.LastInsertId()
	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"id":     id,
		"status": "created",
	})
}

// DeleteBookmarkHandler deletes a bookmark
func DeleteBookmarkHandler(w http.ResponseWriter, r *http.Request) {
	bookmarkID := chi.URLParam(r, "id")
	current := getUserFromContext(r.Context())

	_, err := appDB.Exec("DELETE FROM bookmark WHERE id = ? AND owner_user_id = ?", bookmarkID, current.ID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to delete bookmark")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// StartReadingSessionHandler starts a new reading session
func StartReadingSessionHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	current := getUserFromContext(r.Context())
	bookIDInt, _ := strconv.ParseInt(bookID, 10, 64)
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
		ReaderType string `json:"reader_type"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	readerType := strings.TrimSpace(req.ReaderType)
	if readerType == "" {
		readerType = "normal"
	}

	now := time.Now().Unix()

	_, _ = appDB.Exec(`
		UPDATE reading_session
		SET ended_at = ?
		WHERE book_id = ? AND ended_at IS NULL AND owner_user_id = ?
	`, now, bookIDInt, current.ID)

	result, err := appDB.Exec(`
		INSERT INTO reading_session (book_id, reader_type, started_at, owner_user_id)
		VALUES (?, ?, ?, ?)
	`, bookIDInt, readerType, now, current.ID)

	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to start reading session")
		return
	}

	sessionID, _ := result.LastInsertId()
	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"id":         sessionID,
		"started_at": now,
	})
}

// EndReadingSessionHandler ends a reading session
func EndReadingSessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	current := getUserFromContext(r.Context())

	now := time.Now().Unix()

	_, err := appDB.Exec(`
		UPDATE reading_session SET ended_at = ? WHERE id = ? AND ended_at IS NULL AND owner_user_id = ?
	`, now, sessionID, current.ID)

	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to end reading session")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteReadingSessionHandler deletes a reading session
func DeleteReadingSessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	current := getUserFromContext(r.Context())

	_, err := appDB.Exec(`DELETE FROM reading_session WHERE id = ? AND owner_user_id = ?`, sessionID, current.ID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to delete reading session")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetReadingHistoryHandler gets reading history
func GetReadingHistoryHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	// Get sessions from last 30 days
	since := time.Now().AddDate(0, 0, -30).Unix()

	rows, err := appDB.Query(`
		SELECT rs.id, rs.book_id, rs.started_at, rs.ended_at,
		       COALESCE(rs.reader_type, 'normal') as reader_type,
		       COALESCE(bm.title, 'Unknown') as title,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(rp.percent, 0) as percent,
		       COALESCE(rp.status, 'unread') as status
		FROM reading_session rs
		JOIN book b ON rs.book_id = b.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		WHERE rs.started_at > ? AND rs.owner_user_id = ?
		ORDER BY rs.started_at DESC
		LIMIT 50
	`, since, current.ID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to get reading history")
		return
	}
	defer rows.Close()

	type HistoryItem struct {
		SessionID  int64   `json:"session_id"`
		BookID     int64   `json:"book_id"`
		Title      string  `json:"title"`
		CoverPath  string  `json:"cover_path"`
		Percent    float64 `json:"percent"`
		Status     string  `json:"status"`
		ReaderType string  `json:"reader_type"`
		StartedAt  int64   `json:"started_at"`
		EndedAt    *int64  `json:"ended_at,omitempty"`
	}

	history := []HistoryItem{}
	for rows.Next() {
		var h HistoryItem
		var endedAt sql.NullInt64
		var coverPath, status, readerType sql.NullString
		var percent sql.NullFloat64
		var startedAt int64

		if err := rows.Scan(&h.SessionID, &h.BookID, &startedAt, &endedAt, &readerType,
			&h.Title, &coverPath, &percent, &status); err != nil {
			continue
		}

		h.StartedAt = startedAt
		if endedAt.Valid {
			h.EndedAt = &endedAt.Int64
		}
		if coverPath.Valid {
			h.CoverPath = coverPath.String
		}
		if percent.Valid {
			h.Percent = percent.Float64
		}
		if status.Valid {
			h.Status = status.String
		}
		if readerType.Valid {
			h.ReaderType = readerType.String
		}

		history = append(history, h)
	}

	jsonResponse(w, http.StatusOK, history)
}

// GetBookSessionsHandler gets reading sessions for a specific book
func GetBookSessionsHandler(w http.ResponseWriter, r *http.Request) {
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
		SELECT id, book_id, COALESCE(reader_type, 'normal'), started_at, ended_at
		FROM reading_session
		WHERE book_id = ? AND owner_user_id = ?
		ORDER BY started_at DESC
		LIMIT 100
	`, bookID, current.ID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to get reading sessions")
		return
	}
	defer rows.Close()

	sessions := []ReadingSession{}
	for rows.Next() {
		var s ReadingSession
		var endedAt sql.NullInt64
		var startedAt int64
		var readerType sql.NullString

		if err := rows.Scan(&s.ID, &s.BookID, &readerType, &startedAt, &endedAt); err != nil {
			continue
		}

		if readerType.Valid {
			s.ReaderType = readerType.String
		}
		s.StartedAt = startedAt
		if endedAt.Valid {
			s.EndedAt = &endedAt.Int64
		}

		sessions = append(sessions, s)
	}

	jsonResponse(w, http.StatusOK, sessions)
}
