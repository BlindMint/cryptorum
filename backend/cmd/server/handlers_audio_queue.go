package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var supportedAudioFormats = map[string]bool{
	"mp3": true, "m4a": true, "m4b": true,
	"flac": true, "ogg": true, "wav": true,
}

type audioQueueItem struct {
	ID              int64    `json:"id"`
	AudioID         *int64   `json:"audio_id"`
	BookID          int64    `json:"book_id"`
	FileID          int64    `json:"file_id"`
	Title           string   `json:"title"`
	Authors         []string `json:"authors"`
	Filename        string   `json:"filename"`
	Format          string   `json:"format"`
	Category        string   `json:"category"`
	Album           string   `json:"album"`
	ShowTitle       string   `json:"show_title"`
	DurationSeconds float64  `json:"duration_seconds"`
	PlaybackSpeed   *float64 `json:"playback_speed"`
	PositionSeconds float64  `json:"position_seconds"`
	ListeningStatus string   `json:"listening_status"`
	StreamURL       string   `json:"stream_url"`
	ChapterCount    int      `json:"chapter_count"`
	Unavailable     bool     `json:"unavailable"`
}

type audioQueueResponse struct {
	Items         []audioQueueItem `json:"items"`
	CurrentItemID *int64           `json:"current_item_id"`
}

func audioQueueOwnerID(r *http.Request) (int64, bool) {
	current := getUserFromContext(r.Context())
	if current == nil {
		return 0, false
	}
	return userIDForScopedRows(current), true
}

func loadAudioQueue(ownerID int64) (audioQueueResponse, error) {
	response := audioQueueResponse{Items: []audioQueueItem{}}
	rows, err := appDB.Query(`
		SELECT qi.id, ai.id, qi.book_id, qi.file_id,
		       COALESCE(NULLIF(ai.title, ''), NULLIF(bm.title, ''), bf.path),
		       COALESCE(NULLIF(ai.artists, ''), bm.authors, '[]'),
		       bf.path, LOWER(bf.format), COALESCE(ai.category, 'audiobook'),
		       COALESCE(ai.album, ''), COALESCE(ai.show_title, ''), COALESCE(ai.duration_seconds, 0),
		       ai.playback_speed, COALESCE(ls.position_seconds, 0), COALESCE(ls.status, 'unplayed'),
		       (SELECT COUNT(*) FROM audio_chapter ac WHERE ac.audio_item_id = ai.id), bf.missing_at
		FROM audio_queue_item qi
		JOIN book b ON b.id = qi.book_id
		JOIN book_file bf ON bf.id = qi.file_id AND bf.book_id = qi.book_id
		LEFT JOIN book_metadata bm ON bm.book_id = qi.book_id
		LEFT JOIN audio_item ai ON ai.file_id = qi.file_id AND ai.owner_user_id = qi.owner_user_id
		LEFT JOIN audio_listening_state ls ON ls.audio_item_id = ai.id AND ls.owner_user_id = qi.owner_user_id
		WHERE qi.owner_user_id = ?
		ORDER BY qi.position, qi.id`, ownerID)
	if err != nil {
		return response, err
	}
	defer rows.Close()
	for rows.Next() {
		var item audioQueueItem
		var audioID sql.NullInt64
		var playbackSpeed sql.NullFloat64
		var missingAt sql.NullInt64
		var authorsJSON, path string
		if err := rows.Scan(&item.ID, &audioID, &item.BookID, &item.FileID, &item.Title, &authorsJSON, &path, &item.Format, &item.Category, &item.Album, &item.ShowTitle, &item.DurationSeconds, &playbackSpeed, &item.PositionSeconds, &item.ListeningStatus, &item.ChapterCount, &missingAt); err != nil {
			return response, err
		}
		if audioID.Valid {
			item.AudioID = &audioID.Int64
		}
		if playbackSpeed.Valid {
			item.PlaybackSpeed = &playbackSpeed.Float64
		}
		item.Unavailable = missingAt.Valid
		item.Filename = filepath.Base(path)
		item.StreamURL = "/api/books/" + strconv.FormatInt(item.BookID, 10) + "/file?file_id=" + strconv.FormatInt(item.FileID, 10)
		if item.Title == path {
			item.Title = strings.TrimSuffix(item.Filename, filepath.Ext(item.Filename))
		}
		if err := json.Unmarshal([]byte(authorsJSON), &item.Authors); err != nil {
			item.Authors = []string{}
		}
		response.Items = append(response.Items, item)
	}
	if err := rows.Err(); err != nil {
		return response, err
	}
	var currentID sql.NullInt64
	err = appDB.QueryRow(`SELECT current_item_id FROM audio_queue_state WHERE owner_user_id = ?`, ownerID).Scan(&currentID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return response, err
	}
	if currentID.Valid {
		response.CurrentItemID = &currentID.Int64
	}
	return response, nil
}

func writeAudioQueueResponse(w http.ResponseWriter, ownerID int64, status int) {
	response, err := loadAudioQueue(ownerID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load audio queue")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func GetAudioQueueHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := audioQueueOwnerID(r)
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	writeAudioQueueResponse(w, ownerID, http.StatusOK)
}

type addAudioQueueItemRequest struct {
	BookID      int64  `json:"book_id"`
	FileID      *int64 `json:"file_id"`
	Placement   string `json:"placement"`
	MakeCurrent bool   `json:"make_current"`
}

type addAudioQueueItemsBulkRequest struct {
	BookIDs []int64 `json:"book_ids"`
}

type replaceAudioQueueRequest struct {
	AudioIDs []int64 `json:"audio_ids"`
}

func moveAudioQueueItemNext(tx *sql.Tx, ownerID, itemID int64) error {
	var currentID sql.NullInt64
	err := tx.QueryRow(`SELECT current_item_id FROM audio_queue_state WHERE owner_user_id = ?`, ownerID).Scan(&currentID)
	if errors.Is(err, sql.ErrNoRows) || !currentID.Valid || currentID.Int64 == itemID {
		return nil
	}
	if err != nil {
		return err
	}

	rows, err := tx.Query(`SELECT id FROM audio_queue_item WHERE owner_user_id = ? ORDER BY position, id`, ownerID)
	if err != nil {
		return err
	}
	ordered := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if id != itemID {
			ordered = append(ordered, id)
		}
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if err = rows.Err(); err != nil {
		return err
	}

	currentIndex := -1
	for index, id := range ordered {
		if id == currentID.Int64 {
			currentIndex = index
			break
		}
	}
	if currentIndex < 0 {
		return nil
	}
	ordered = append(ordered, 0)
	copy(ordered[currentIndex+2:], ordered[currentIndex+1:])
	ordered[currentIndex+1] = itemID
	for position, id := range ordered {
		if _, err = tx.Exec(`UPDATE audio_queue_item SET position = ? WHERE id = ? AND owner_user_id = ?`, position, id, ownerID); err != nil {
			return err
		}
	}
	return nil
}

func ReplaceAudioQueueHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := audioQueueOwnerID(r)
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	var request replaceAudioQueueRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&request); err != nil || len(request.AudioIDs) == 0 || len(request.AudioIDs) > 5000 {
		errorResponse(w, http.StatusBadRequest, "Select audio items to play")
		return
	}
	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, 500, "Failed to replace audio queue")
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM audio_queue_item WHERE owner_user_id = ?`, ownerID); err != nil {
		errorResponse(w, 500, "Failed to replace audio queue")
		return
	}
	firstID := int64(0)
	position := 0
	seen := map[int64]bool{}
	for _, audioID := range request.AudioIDs {
		if audioID <= 0 || seen[audioID] {
			continue
		}
		seen[audioID] = true
		result, execErr := tx.Exec(`INSERT INTO audio_queue_item (owner_user_id, book_id, file_id, audio_item_id, position, added_at) SELECT ?, book_id, file_id, id, ?, ? FROM audio_item WHERE id = ? AND owner_user_id = ? AND EXISTS(SELECT 1 FROM book_file bf WHERE bf.id = audio_item.file_id AND bf.missing_at IS NULL)`, ownerID, position, time.Now().Unix(), audioID, ownerID)
		if execErr != nil {
			errorResponse(w, 500, "Failed to replace audio queue")
			return
		}
		if count, _ := result.RowsAffected(); count == 0 {
			continue
		}
		itemID, _ := result.LastInsertId()
		if firstID == 0 {
			firstID = itemID
		}
		position++
	}
	if firstID == 0 {
		errorResponse(w, 400, "No playable audio items were selected")
		return
	}
	if _, err = tx.Exec(`INSERT INTO audio_queue_state (owner_user_id, current_item_id, updated_at) VALUES (?, ?, ?) ON CONFLICT(owner_user_id) DO UPDATE SET current_item_id = excluded.current_item_id, updated_at = excluded.updated_at`, ownerID, firstID, time.Now().Unix()); err != nil {
		errorResponse(w, 500, "Failed to replace audio queue")
		return
	}
	if err = tx.Commit(); err != nil {
		errorResponse(w, 500, "Failed to replace audio queue")
		return
	}
	writeAudioQueueResponse(w, ownerID, http.StatusOK)
}

type audioQueueBulkResponse struct {
	audioQueueResponse
	AddedCount   int `json:"added_count"`
	SkippedCount int `json:"skipped_count"`
}

func AddAudioQueueItemsBulkHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := audioQueueOwnerID(r)
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	var request addAudioQueueItemsBulkRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&request); err != nil || len(request.BookIDs) == 0 {
		errorResponse(w, http.StatusBadRequest, "Select at least one book")
		return
	}
	if len(request.BookIDs) > 1000 {
		errorResponse(w, http.StatusBadRequest, "Audio queue updates are limited to 1000 books at a time")
		return
	}

	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
		return
	}
	defer tx.Rollback()

	var nextPosition int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(position), -1) + 1 FROM audio_queue_item WHERE owner_user_id = ?`, ownerID).Scan(&nextPosition); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
		return
	}
	addedCount := 0
	skippedCount := 0
	firstQueueItemID := int64(0)
	seen := make(map[int64]bool, len(request.BookIDs))
	for _, bookID := range request.BookIDs {
		if bookID <= 0 || seen[bookID] {
			skippedCount++
			continue
		}
		seen[bookID] = true
		fileRows, queryErr := tx.Query(`
			SELECT bf.id
			FROM book_file bf JOIN book b ON b.id = bf.book_id
			WHERE b.id = ? AND b.owner_user_id = ? AND bf.missing_at IS NULL
			  AND LOWER(bf.format) IN ('mp3', 'm4a', 'm4b', 'flac', 'ogg', 'wav')
			ORDER BY bf.path COLLATE NOCASE, bf.id`, bookID, ownerID)
		if queryErr != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
			return
		}
		fileIDs := []int64{}
		for fileRows.Next() {
			var fileID int64
			if scanErr := fileRows.Scan(&fileID); scanErr != nil {
				_ = fileRows.Close()
				errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
				return
			}
			fileIDs = append(fileIDs, fileID)
		}
		rowsErr := fileRows.Err()
		_ = fileRows.Close()
		if rowsErr != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
			return
		}
		if len(fileIDs) == 0 {
			skippedCount++
			continue
		}
		for _, fileID := range fileIDs {
			var itemID int64
			err = tx.QueryRow(`SELECT id FROM audio_queue_item WHERE owner_user_id = ? AND file_id = ?`, ownerID, fileID).Scan(&itemID)
			if errors.Is(err, sql.ErrNoRows) {
				result, execErr := tx.Exec(`INSERT INTO audio_queue_item (owner_user_id, book_id, file_id, audio_item_id, position, added_at) VALUES (?, ?, ?, (SELECT id FROM audio_item WHERE owner_user_id = ? AND file_id = ?), ?, ?)`, ownerID, bookID, fileID, ownerID, fileID, nextPosition, time.Now().Unix())
				if execErr != nil {
					errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
					return
				}
				itemID, err = result.LastInsertId()
				if err != nil {
					errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
					return
				}
				nextPosition++
				addedCount++
			} else if err != nil {
				errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
				return
			} else {
				skippedCount++
			}
			if firstQueueItemID == 0 {
				firstQueueItemID = itemID
			}
		}
	}

	var currentID sql.NullInt64
	stateErr := tx.QueryRow(`SELECT current_item_id FROM audio_queue_state WHERE owner_user_id = ?`, ownerID).Scan(&currentID)
	if stateErr != nil && !errors.Is(stateErr, sql.ErrNoRows) {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
		return
	}
	if !currentID.Valid && firstQueueItemID != 0 {
		_, err = tx.Exec(`INSERT INTO audio_queue_state (owner_user_id, current_item_id, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(owner_user_id) DO UPDATE SET current_item_id = excluded.current_item_id, updated_at = excluded.updated_at`, ownerID, firstQueueItemID, time.Now().Unix())
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
		return
	}
	queue, err := loadAudioQueue(ownerID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load audio queue")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(audioQueueBulkResponse{
		audioQueueResponse: queue,
		AddedCount:         addedCount,
		SkippedCount:       skippedCount,
	})
}

func AddAudioQueueItemHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := audioQueueOwnerID(r)
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	var request addAudioQueueItemRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&request); err != nil || request.BookID <= 0 {
		errorResponse(w, http.StatusBadRequest, "Invalid queue item")
		return
	}
	if request.Placement == "" {
		request.Placement = "append"
	}
	if request.Placement != "append" && request.Placement != "next" {
		errorResponse(w, http.StatusBadRequest, "Placement must be append or next")
		return
	}

	query := `SELECT bf.id, LOWER(bf.format)
		FROM book_file bf JOIN book b ON b.id = bf.book_id
		WHERE bf.book_id = ? AND b.owner_user_id = ? AND bf.missing_at IS NULL`
	args := []any{request.BookID, ownerID}
	if request.FileID != nil {
		query += " AND bf.id = ?"
		args = append(args, *request.FileID)
	} else {
		query += " AND LOWER(bf.format) IN ('mp3', 'm4a', 'm4b', 'flac', 'ogg', 'wav')"
	}
	query += " ORDER BY bf.id LIMIT 1"
	var fileID int64
	var format string
	if err := appDB.QueryRow(query, args...).Scan(&fileID, &format); err != nil {
		errorResponse(w, http.StatusNotFound, "Audio file not found")
		return
	}
	if !supportedAudioFormats[format] {
		errorResponse(w, http.StatusBadRequest, "Selected file is not a supported audio format")
		return
	}

	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
		return
	}
	defer tx.Rollback()
	var itemID int64
	err = tx.QueryRow(`SELECT id FROM audio_queue_item WHERE owner_user_id = ? AND file_id = ?`, ownerID, fileID).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		var position int
		if request.Placement == "next" {
			var currentPosition sql.NullInt64
			_ = tx.QueryRow(`SELECT qi.position FROM audio_queue_state qs JOIN audio_queue_item qi ON qi.id = qs.current_item_id WHERE qs.owner_user_id = ?`, ownerID).Scan(&currentPosition)
			if currentPosition.Valid {
				position = int(currentPosition.Int64) + 1
				if _, err = tx.Exec(`UPDATE audio_queue_item SET position = position + 1 WHERE owner_user_id = ? AND position >= ?`, ownerID, position); err != nil {
					errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
					return
				}
			} else {
				request.Placement = "append"
			}
		}
		if request.Placement == "append" {
			if err = tx.QueryRow(`SELECT COALESCE(MAX(position), -1) + 1 FROM audio_queue_item WHERE owner_user_id = ?`, ownerID).Scan(&position); err != nil {
				errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
				return
			}
		}
		result, execErr := tx.Exec(`INSERT INTO audio_queue_item (owner_user_id, book_id, file_id, audio_item_id, position, added_at) VALUES (?, ?, ?, (SELECT id FROM audio_item WHERE owner_user_id = ? AND file_id = ?), ?, ?)`, ownerID, request.BookID, fileID, ownerID, fileID, position, time.Now().Unix())
		if execErr != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
			return
		}
		itemID, err = result.LastInsertId()
	} else if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
		return
	} else if request.Placement == "next" {
		if err = moveAudioQueueItemNext(tx, ownerID, itemID); err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
			return
		}
	}
	var storedCurrent sql.NullInt64
	currentErr := tx.QueryRow(`SELECT current_item_id FROM audio_queue_state WHERE owner_user_id = ?`, ownerID).Scan(&storedCurrent)
	if currentErr != nil && !errors.Is(currentErr, sql.ErrNoRows) {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
		return
	}
	if request.MakeCurrent || !storedCurrent.Valid {
		_, err = tx.Exec(`INSERT INTO audio_queue_state (owner_user_id, current_item_id, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(owner_user_id) DO UPDATE SET current_item_id = excluded.current_item_id, updated_at = excluded.updated_at`, ownerID, itemID, time.Now().Unix())
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
		return
	}
	writeAudioQueueResponse(w, ownerID, http.StatusCreated)
}

type setAudioQueueCurrentRequest struct {
	ItemID *int64 `json:"item_id"`
}

func SetAudioQueueCurrentHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := audioQueueOwnerID(r)
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	var request setAudioQueueCurrentRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&request); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid current queue item")
		return
	}
	if request.ItemID != nil {
		var exists int
		if err := appDB.QueryRow(`SELECT 1 FROM audio_queue_item WHERE id = ? AND owner_user_id = ?`, *request.ItemID, ownerID).Scan(&exists); err != nil {
			errorResponse(w, http.StatusNotFound, "Queue item not found")
			return
		}
	}
	_, err := appDB.Exec(`INSERT INTO audio_queue_state (owner_user_id, current_item_id, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(owner_user_id) DO UPDATE SET current_item_id = excluded.current_item_id, updated_at = excluded.updated_at`, ownerID, request.ItemID, time.Now().Unix())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
		return
	}
	writeAudioQueueResponse(w, ownerID, http.StatusOK)
}

type reorderAudioQueueRequest struct {
	ItemIDs []int64 `json:"item_ids"`
}

func ReorderAudioQueueHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := audioQueueOwnerID(r)
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	var request reorderAudioQueueRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&request); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid queue order")
		return
	}
	var count int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM audio_queue_item WHERE owner_user_id = ?`, ownerID).Scan(&count); err != nil || count != len(request.ItemIDs) {
		errorResponse(w, http.StatusBadRequest, "Queue order must include every item")
		return
	}
	seen := make(map[int64]bool, len(request.ItemIDs))
	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to reorder audio queue")
		return
	}
	defer tx.Rollback()
	for position, itemID := range request.ItemIDs {
		if seen[itemID] {
			errorResponse(w, http.StatusBadRequest, "Queue order contains duplicates")
			return
		}
		seen[itemID] = true
		result, execErr := tx.Exec(`UPDATE audio_queue_item SET position = ? WHERE id = ? AND owner_user_id = ?`, position, itemID, ownerID)
		if execErr != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to reorder audio queue")
			return
		}
		rows, _ := result.RowsAffected()
		if rows != 1 {
			errorResponse(w, http.StatusBadRequest, "Queue order contains an unknown item")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to reorder audio queue")
		return
	}
	writeAudioQueueResponse(w, ownerID, http.StatusOK)
}

func DeleteAudioQueueItemHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := audioQueueOwnerID(r)
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	itemID, err := strconv.ParseInt(chi.URLParam(r, "itemID"), 10, 64)
	if err != nil || itemID <= 0 {
		errorResponse(w, http.StatusBadRequest, "Invalid queue item")
		return
	}
	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
		return
	}
	defer tx.Rollback()
	var wasCurrent bool
	var removedPosition int
	if err := tx.QueryRow(`SELECT position FROM audio_queue_item WHERE id = ? AND owner_user_id = ?`, itemID, ownerID).Scan(&removedPosition); err != nil {
		errorResponse(w, http.StatusNotFound, "Queue item not found")
		return
	}
	_ = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM audio_queue_state WHERE owner_user_id = ? AND current_item_id = ?)`, ownerID, itemID).Scan(&wasCurrent)
	result, err := tx.Exec(`DELETE FROM audio_queue_item WHERE id = ? AND owner_user_id = ?`, itemID, ownerID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
		return
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		errorResponse(w, http.StatusNotFound, "Queue item not found")
		return
	}
	if wasCurrent {
		var nextID sql.NullInt64
		_ = tx.QueryRow(`SELECT id FROM audio_queue_item WHERE owner_user_id = ? AND position >= ? ORDER BY position, id LIMIT 1`, ownerID, removedPosition).Scan(&nextID)
		_, err = tx.Exec(`UPDATE audio_queue_state SET current_item_id = ?, updated_at = ? WHERE owner_user_id = ?`, nextID, time.Now().Unix(), ownerID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update audio queue")
		return
	}
	writeAudioQueueResponse(w, ownerID, http.StatusOK)
}

func ClearAudioQueueHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := audioQueueOwnerID(r)
	if !ok {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	tx, err := appDB.Begin()
	if err == nil {
		_, err = tx.Exec(`DELETE FROM audio_queue_state WHERE owner_user_id = ?`, ownerID)
	}
	if err == nil {
		_, err = tx.Exec(`DELETE FROM audio_queue_item WHERE owner_user_id = ?`, ownerID)
	}
	if err == nil {
		err = tx.Commit()
	} else if tx != nil {
		_ = tx.Rollback()
	}
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to clear audio queue")
		return
	}
	writeAudioQueueResponse(w, ownerID, http.StatusOK)
}
