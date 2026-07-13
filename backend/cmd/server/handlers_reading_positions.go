package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	readingChannelStandard = "standard"
	readingChannelSpeed    = "speed"
)

type storedReadingLocator struct {
	Revision int64           `json:"revision"`
	Locator  json.RawMessage `json:"locator"`
}

type readingPosition struct {
	ID               int64                           `json:"id,omitempty"`
	BookID           int64                           `json:"book_id"`
	FileID           int64                           `json:"file_id"`
	Channel          string                          `json:"channel"`
	Percent          float64                         `json:"percent"`
	ActiveReaderMode string                          `json:"active_reader_mode"`
	Locators         map[string]storedReadingLocator `json:"locators"`
	SourceHash       string                          `json:"source_hash"`
	Revision         int64                           `json:"revision"`
	UpdatedAtMS      int64                           `json:"updated_at_ms"`
}

type startReadingPositionSessionRequest struct {
	FileID     int64  `json:"file_id"`
	Channel    string `json:"channel"`
	ReaderMode string `json:"reader_mode"`
}

type saveReadingPositionRequest struct {
	ClientSequence int64           `json:"client_sequence"`
	BaseRevision   int64           `json:"base_revision"`
	ReaderMode     string          `json:"reader_mode"`
	Percent        float64         `json:"percent"`
	Locator        json.RawMessage `json:"locator,omitempty"`
	SourceHash     string          `json:"source_hash"`
	ReachedEnd     bool            `json:"reached_end"`
}

type readingPositionQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

func normalizeReadingChannel(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), readingChannelSpeed) {
		return readingChannelSpeed
	}
	return readingChannelStandard
}

func normalizeReaderMode(value string, channel string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if channel == readingChannelSpeed {
		return "speed"
	}
	switch value {
	case "epub_paginated", "continuous_text", "pdf", "comic", "audio":
		return value
	default:
		return ""
	}
}

func defaultReaderMode(format, channel string) string {
	if channel == readingChannelSpeed {
		return "speed"
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "epub":
		return "epub_paginated"
	case "pdf":
		return "pdf"
	case "cbz", "cbr", "cb7", "cbt":
		return "comic"
	case "mp3", "m4a", "m4b", "flac", "ogg", "opus", "wav", "aac":
		return "audio"
	default:
		return "continuous_text"
	}
}

func readerModeSupportsFormat(mode, format, channel string) bool {
	format = strings.ToLower(strings.TrimSpace(format))
	if channel == readingChannelSpeed {
		return format == "pdf" || isSupportedTextBookFormat(format) || format == "txt" || format == "text"
	}
	switch mode {
	case "epub_paginated":
		return format == "epub"
	case "continuous_text":
		return isSupportedTextBookFormat(format) || format == "txt" || format == "text"
	case "pdf":
		return format == "pdf"
	case "comic":
		return format == "cbz" || format == "cbr" || format == "cb7" || format == "cbt"
	case "audio":
		return format == "mp3" || format == "m4a" || format == "m4b" || format == "flac" || format == "ogg" || format == "opus" || format == "wav" || format == "aac"
	default:
		return false
	}
}

func sessionReaderType(mode string) string {
	switch mode {
	case "epub_paginated", "continuous_text":
		return "epub"
	case "comic":
		return "comic"
	case "audio":
		return "audio"
	case "speed":
		return "speed"
	case "pdf":
		return "pdf"
	default:
		return "normal"
	}
}

func loadReadingPosition(q readingPositionQueryer, ownerUserID, fileID int64, channel string) (readingPosition, error) {
	var position readingPosition
	var locatorsJSON string
	err := q.QueryRow(`
		SELECT id, book_id, file_id, channel, percent, active_reader_mode,
		       locators_json, source_hash, revision, updated_at_ms
		FROM reading_position
		WHERE owner_user_id = ? AND file_id = ? AND channel = ?
	`, ownerUserID, fileID, channel).Scan(
		&position.ID, &position.BookID, &position.FileID, &position.Channel,
		&position.Percent, &position.ActiveReaderMode, &locatorsJSON,
		&position.SourceHash, &position.Revision, &position.UpdatedAtMS,
	)
	if err != nil {
		return readingPosition{}, err
	}
	position.Locators = make(map[string]storedReadingLocator)
	if strings.TrimSpace(locatorsJSON) != "" {
		_ = json.Unmarshal([]byte(locatorsJSON), &position.Locators)
	}
	return position, nil
}

func emptyReadingPosition(bookID, fileID int64, channel, mode, sourceHash string) readingPosition {
	return readingPosition{
		BookID:           bookID,
		FileID:           fileID,
		Channel:          channel,
		ActiveReaderMode: mode,
		Locators:         make(map[string]storedReadingLocator),
		SourceHash:       sourceHash,
	}
}

func legacyLocator(format, channel, cfi string, page, wordIndex int64) json.RawMessage {
	var value any
	if channel == readingChannelSpeed && wordIndex >= 0 {
		value = map[string]any{"type": "word_index", "word_index": wordIndex}
	} else {
		switch strings.ToLower(format) {
		case "epub":
			if strings.TrimSpace(cfi) != "" {
				value = map[string]any{"type": "epub_cfi", "cfi": cfi}
			}
		case "pdf":
			if page > 0 {
				value = map[string]any{"type": "pdf_page", "page": page}
			}
		case "cbz", "cbr", "cb7", "cbt":
			if page > 0 {
				value = map[string]any{"type": "comic_page", "page": page}
			}
		}
	}
	if value == nil {
		return nil
	}
	encoded, _ := json.Marshal(value)
	return encoded
}

func adoptLegacyReadingPosition(tx *sql.Tx, bookID, fileID, ownerUserID int64, format, sourceHash, channel, mode string) (readingPosition, error) {
	var percent, speedPercent sql.NullFloat64
	var cfi sql.NullString
	var page, wordIndex sql.NullInt64
	var standardAdopted, speedAdopted int
	err := tx.QueryRow(`
		SELECT percent, cfi, page, speed_reader_word_index, speed_reader_percent,
		       legacy_standard_adopted, legacy_speed_adopted
		FROM reading_progress
		WHERE book_id = ? AND owner_user_id = ?
	`, bookID, ownerUserID).Scan(
		&percent, &cfi, &page, &wordIndex, &speedPercent, &standardAdopted, &speedAdopted,
	)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(`
			INSERT INTO reading_progress (book_id, status, percent, speed_reader_percent, updated_at, owner_user_id)
			VALUES (?, 'unread', 0, 0, 0, ?)
		`, bookID, ownerUserID)
		return emptyReadingPosition(bookID, fileID, channel, mode, sourceHash), err
	}
	if err != nil {
		return readingPosition{}, err
	}

	adopted := standardAdopted != 0
	legacyPercent := percent.Float64
	if channel == readingChannelSpeed {
		adopted = speedAdopted != 0
		legacyPercent = speedPercent.Float64
	}
	flagColumn := "legacy_standard_adopted"
	if channel == readingChannelSpeed {
		flagColumn = "legacy_speed_adopted"
	}
	if !adopted {
		if _, err := tx.Exec(`UPDATE reading_progress SET `+flagColumn+` = 1 WHERE book_id = ? AND owner_user_id = ?`, bookID, ownerUserID); err != nil {
			return readingPosition{}, err
		}
	}
	if adopted || legacyPercent <= 0 {
		return emptyReadingPosition(bookID, fileID, channel, mode, sourceHash), nil
	}

	legacyPercent = math.Max(0, math.Min(100, legacyPercent))
	locators := make(map[string]storedReadingLocator)
	locator := legacyLocator(format, channel, cfi.String, page.Int64, wordIndex.Int64)
	if len(locator) > 0 {
		locators[mode] = storedReadingLocator{Revision: 1, Locator: locator}
	}
	locatorsJSON, _ := json.Marshal(locators)
	result, err := tx.Exec(`
		INSERT INTO reading_position (
			book_id, file_id, owner_user_id, channel, percent, active_reader_mode,
			locators_json, source_hash, revision, updated_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
	`, bookID, fileID, ownerUserID, channel, legacyPercent, mode, string(locatorsJSON), sourceHash, time.Now().UnixMilli())
	if err != nil {
		return readingPosition{}, err
	}
	id, _ := result.LastInsertId()
	return readingPosition{
		ID: id, BookID: bookID, FileID: fileID, Channel: channel,
		Percent: legacyPercent, ActiveReaderMode: mode, Locators: locators,
		SourceHash: sourceHash, Revision: 1, UpdatedAtMS: time.Now().UnixMilli(),
	}, nil
}

// StartReadingPositionSessionHandler starts the authoritative session for a
// book/channel and returns the exact per-file position in the same round trip.
func StartReadingPositionSessionHandler(w http.ResponseWriter, r *http.Request) {
	bookID, err := strconv.ParseInt(chi.URLParam(r, "bookID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	current := getUserFromContext(r.Context())
	allowed, err := canAccessBook(current, bookID)
	if err != nil || !allowed {
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		} else {
			errorResponse(w, http.StatusForbidden, "Permission denied")
		}
		return
	}

	var req startReadingPositionSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileID <= 0 {
		errorResponse(w, http.StatusBadRequest, "A valid file_id is required")
		return
	}
	channel := normalizeReadingChannel(req.Channel)

	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to start reading session")
		return
	}
	defer tx.Rollback()

	var format, sourceHash string
	err = tx.QueryRow(`
		SELECT LOWER(format), COALESCE(hash, '')
		FROM book_file
		WHERE id = ? AND book_id = ? AND missing_at IS NULL
	`, req.FileID, bookID).Scan(&format, &sourceHash)
	if errors.Is(err, sql.ErrNoRows) {
		errorResponse(w, http.StatusNotFound, "Book file not found")
		return
	}
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load book file")
		return
	}
	mode := normalizeReaderMode(req.ReaderMode, channel)
	if mode == "" {
		mode = defaultReaderMode(format, channel)
	}
	if !readerModeSupportsFormat(mode, format, channel) {
		errorResponse(w, http.StatusBadRequest, "Reader mode is not compatible with this file")
		return
	}

	now := time.Now()
	_, err = tx.Exec(`
		UPDATE reading_session
		SET superseded_at = COALESCE(superseded_at, ?), ended_at = COALESCE(ended_at, ?)
		WHERE book_id = ? AND owner_user_id = ? AND channel = ? AND superseded_at IS NULL
	`, now.Unix(), now.Unix(), bookID, current.ID, channel)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to supersede previous reading session")
		return
	}

	result, err := tx.Exec(`
		INSERT INTO reading_session (
			book_id, reader_type, started_at, owner_user_id, file_id, channel, reader_mode
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, bookID, sessionReaderType(mode), now.Unix(), current.ID, req.FileID, channel, mode)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to start reading session")
		return
	}
	sessionID, _ := result.LastInsertId()

	position, err := loadReadingPosition(tx, current.ID, req.FileID, channel)
	if errors.Is(err, sql.ErrNoRows) {
		position, err = adoptLegacyReadingPosition(tx, bookID, req.FileID, current.ID, format, sourceHash, channel, mode)
	}
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load reading position")
		return
	}
	if position.Locators == nil {
		position.Locators = make(map[string]storedReadingLocator)
	}
	if position.SourceHash != "" && position.SourceHash != sourceHash {
		// The logical percentage remains useful, but exact locators refer to the
		// previous contents and must not be applied to the replacement file.
		position.Locators = make(map[string]storedReadingLocator)
		position.SourceHash = sourceHash
	}

	if channel == readingChannelStandard {
		_, err = tx.Exec(`
			INSERT INTO reading_progress (book_id, file_id, percent, status, updated_at, owner_user_id)
			VALUES (?, ?, ?, 'unread', 0, ?)
			ON CONFLICT(book_id, owner_user_id) DO UPDATE SET
				file_id = excluded.file_id,
				percent = excluded.percent
		`, bookID, req.FileID, position.Percent, current.ID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to update resume target")
			return
		}
	} else {
		_, err = tx.Exec(`
			INSERT INTO reading_progress (book_id, speed_file_id, speed_reader_percent, status, updated_at, owner_user_id)
			VALUES (?, ?, ?, 'unread', 0, ?)
			ON CONFLICT(book_id, owner_user_id) DO UPDATE SET
				speed_file_id = excluded.speed_file_id,
				speed_reader_percent = excluded.speed_reader_percent
		`, bookID, req.FileID, position.Percent, current.ID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to update speed reader target")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to start reading session")
		return
	}
	jsonResponse(w, http.StatusCreated, map[string]any{
		"session": map[string]any{
			"id": sessionID, "book_id": bookID, "file_id": req.FileID,
			"channel": channel, "reader_mode": mode, "started_at": now.Unix(),
		},
		"position": position,
	})
}

func validLocator(locator json.RawMessage) bool {
	if len(locator) == 0 || string(locator) == "null" {
		return true
	}
	var value map[string]any
	if json.Unmarshal(locator, &value) != nil {
		return false
	}
	typeValue, ok := value["type"].(string)
	return ok && strings.TrimSpace(typeValue) != ""
}

func readingPositionConflict(w http.ResponseWriter, reason string, position readingPosition) {
	jsonResponse(w, http.StatusConflict, map[string]any{
		"error": reason, "reason": reason, "position": position,
	})
}

// GetReadingPositionSessionHandler lets a resumed/focused tab verify that it is
// still authoritative without writing a synthetic progress checkpoint.
func GetReadingPositionSessionHandler(w http.ResponseWriter, r *http.Request) {
	bookID, err := strconv.ParseInt(chi.URLParam(r, "bookID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	sessionID, err := strconv.ParseInt(chi.URLParam(r, "sessionID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid session ID")
		return
	}
	current := getUserFromContext(r.Context())
	var sessionBookID, fileID int64
	var channel string
	var supersededAt sql.NullInt64
	err = appDB.QueryRow(`
		SELECT book_id, file_id, channel, superseded_at
		FROM reading_session
		WHERE id = ? AND owner_user_id = ?
	`, sessionID, current.ID).Scan(&sessionBookID, &fileID, &channel, &supersededAt)
	if errors.Is(err, sql.ErrNoRows) || sessionBookID != bookID {
		errorResponse(w, http.StatusNotFound, "Reading session not found")
		return
	}
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load reading session")
		return
	}
	position, err := loadReadingPosition(appDB, current.ID, fileID, channel)
	if errors.Is(err, sql.ErrNoRows) {
		position = emptyReadingPosition(bookID, fileID, channel, "", "")
	} else if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load reading position")
		return
	}
	if supersededAt.Valid {
		readingPositionConflict(w, "session_superseded", position)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"status": "ok", "position": position})
}

func legacySummaryLocator(locator json.RawMessage) (string, int64, int64) {
	var value struct {
		Type      string `json:"type"`
		CFI       string `json:"cfi"`
		Page      int64  `json:"page"`
		WordIndex int64  `json:"word_index"`
	}
	if len(locator) > 0 {
		_ = json.Unmarshal(locator, &value)
	}
	return value.CFI, value.Page, value.WordIndex
}

// SaveReadingPositionHandler accepts an ordered, revision-checked checkpoint.
func SaveReadingPositionHandler(w http.ResponseWriter, r *http.Request) {
	bookID, err := strconv.ParseInt(chi.URLParam(r, "bookID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	sessionID, err := strconv.ParseInt(chi.URLParam(r, "sessionID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid session ID")
		return
	}
	var req saveReadingPositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.ClientSequence <= 0 || req.BaseRevision < 0 || math.IsNaN(req.Percent) || math.IsInf(req.Percent, 0) || req.Percent < 0 || req.Percent > 100 || !validLocator(req.Locator) {
		errorResponse(w, http.StatusBadRequest, "Invalid reading checkpoint")
		return
	}

	current := getUserFromContext(r.Context())
	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to save reading position")
		return
	}
	defer tx.Rollback()

	var sessionBookID, fileID, lastSequence int64
	var channel, sessionMode, sourceHash, format string
	var supersededAt sql.NullInt64
	err = tx.QueryRow(`
		SELECT rs.book_id, rs.file_id, rs.channel, rs.reader_mode,
		       rs.superseded_at, rs.last_client_sequence, COALESCE(bf.hash, ''), LOWER(bf.format)
		FROM reading_session rs
		JOIN book_file bf ON bf.id = rs.file_id AND bf.book_id = rs.book_id AND bf.missing_at IS NULL
		WHERE rs.id = ? AND rs.owner_user_id = ?
	`, sessionID, current.ID).Scan(
		&sessionBookID, &fileID, &channel, &sessionMode,
		&supersededAt, &lastSequence, &sourceHash, &format,
	)
	if errors.Is(err, sql.ErrNoRows) || sessionBookID != bookID {
		errorResponse(w, http.StatusNotFound, "Reading session not found")
		return
	}
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load reading session")
		return
	}

	position, loadErr := loadReadingPosition(tx, current.ID, fileID, channel)
	if errors.Is(loadErr, sql.ErrNoRows) {
		position = emptyReadingPosition(bookID, fileID, channel, sessionMode, sourceHash)
	} else if loadErr != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load reading position")
		return
	}
	if supersededAt.Valid {
		readingPositionConflict(w, "session_superseded", position)
		return
	}
	if strings.TrimSpace(req.SourceHash) != "" && req.SourceHash != sourceHash {
		readingPositionConflict(w, "source_changed", position)
		return
	}
	if req.ClientSequence < lastSequence {
		readingPositionConflict(w, "out_of_order", position)
		return
	}
	if req.ClientSequence == lastSequence && lastSequence > 0 {
		jsonResponse(w, http.StatusOK, map[string]any{"status": "ok", "position": position})
		return
	}
	if position.Revision != req.BaseRevision {
		readingPositionConflict(w, "revision_conflict", position)
		return
	}

	mode := normalizeReaderMode(req.ReaderMode, channel)
	if mode == "" {
		mode = sessionMode
	}
	if !readerModeSupportsFormat(mode, format, channel) {
		errorResponse(w, http.StatusBadRequest, "Reader mode is not compatible with this file")
		return
	}
	newRevision := position.Revision + 1
	if position.Locators == nil {
		position.Locators = make(map[string]storedReadingLocator)
	}
	if len(req.Locator) > 0 && string(req.Locator) != "null" {
		position.Locators[mode] = storedReadingLocator{Revision: newRevision, Locator: req.Locator}
	}
	locatorsJSON, _ := json.Marshal(position.Locators)
	now := time.Now()

	var result sql.Result
	if position.ID == 0 {
		result, err = tx.Exec(`
			INSERT INTO reading_position (
				book_id, file_id, owner_user_id, channel, percent, active_reader_mode,
				locators_json, source_hash, revision, updated_at_ms
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, bookID, fileID, current.ID, channel, req.Percent, mode, string(locatorsJSON), sourceHash, newRevision, now.UnixMilli())
	} else {
		result, err = tx.Exec(`
			UPDATE reading_position
			SET percent = ?, active_reader_mode = ?, locators_json = ?, source_hash = ?,
			    revision = ?, updated_at_ms = ?
			WHERE id = ? AND revision = ?
		`, req.Percent, mode, string(locatorsJSON), sourceHash, newRevision, now.UnixMilli(), position.ID, position.Revision)
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			latest, _ := loadReadingPosition(tx, current.ID, fileID, channel)
			readingPositionConflict(w, "revision_conflict", latest)
			return
		}
		errorResponse(w, http.StatusInternalServerError, "Failed to save reading position")
		return
	}
	if position.ID != 0 {
		rows, _ := result.RowsAffected()
		if rows == 0 {
			latest, _ := loadReadingPosition(tx, current.ID, fileID, channel)
			readingPositionConflict(w, "revision_conflict", latest)
			return
		}
	} else {
		position.ID, _ = result.LastInsertId()
	}

	_, err = tx.Exec(`
		UPDATE reading_session SET last_client_sequence = ?
		WHERE id = ? AND owner_user_id = ? AND superseded_at IS NULL
	`, req.ClientSequence, sessionID, current.ID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to acknowledge reading checkpoint")
		return
	}

	cfi, page, wordIndex := legacySummaryLocator(req.Locator)
	statusExpr := "CASE WHEN reading_progress.status = 'unread' THEN 'reading' ELSE reading_progress.status END"
	if req.ReachedEnd {
		statusExpr = "'finished'"
	}
	if channel == readingChannelStandard {
		_, err = tx.Exec(`
			INSERT INTO reading_progress (
				book_id, file_id, percent, cfi, page, status, updated_at,
				standard_updated_at_ms, legacy_standard_adopted, owner_user_id
			) VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, 0), ?, ?, ?, 1, ?)
			ON CONFLICT(book_id, owner_user_id) DO UPDATE SET
				file_id = excluded.file_id,
				percent = excluded.percent,
				cfi = excluded.cfi,
				page = excluded.page,
				status = `+statusExpr+`,
				updated_at = excluded.updated_at,
				standard_updated_at_ms = excluded.standard_updated_at_ms,
				legacy_standard_adopted = 1
		`, bookID, fileID, req.Percent, cfi, page, func() string {
			if req.ReachedEnd {
				return "finished"
			}
			return "reading"
		}(), now.Unix(), now.UnixMilli(), current.ID)
	} else {
		_, err = tx.Exec(`
			INSERT INTO reading_progress (
				book_id, speed_file_id, status, percent, speed_reader_word_index, speed_reader_percent,
				updated_at, speed_updated_at_ms, legacy_speed_adopted, owner_user_id
			) VALUES (?, ?, ?, 0, NULLIF(?, 0), ?, 0, ?, 1, ?)
			ON CONFLICT(book_id, owner_user_id) DO UPDATE SET
				speed_file_id = excluded.speed_file_id,
				speed_reader_word_index = excluded.speed_reader_word_index,
				speed_reader_percent = excluded.speed_reader_percent,
				status = `+statusExpr+`,
				speed_updated_at_ms = excluded.speed_updated_at_ms,
				legacy_speed_adopted = 1
		`, bookID, fileID, func() string {
			if req.ReachedEnd {
				return "finished"
			}
			return "reading"
		}(), wordIndex, req.Percent, now.UnixMilli(), current.ID)
	}
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update reading summary")
		return
	}

	if err := tx.Commit(); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to save reading position")
		return
	}
	position.Percent = req.Percent
	position.ActiveReaderMode = mode
	position.SourceHash = sourceHash
	position.Revision = newRevision
	position.UpdatedAtMS = now.UnixMilli()
	jsonResponse(w, http.StatusOK, map[string]any{"status": "ok", "position": position})
}

// EndReadingPositionSessionHandler records analytics duration without touching
// progress ordering or book status.
func EndReadingPositionSessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID, err := strconv.ParseInt(chi.URLParam(r, "sessionID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid session ID")
		return
	}
	current := getUserFromContext(r.Context())
	_, err = appDB.Exec(`
		UPDATE reading_session SET ended_at = COALESCE(ended_at, ?)
		WHERE id = ? AND owner_user_id = ?
	`, time.Now().Unix(), sessionID, current.ID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to end reading session")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}
