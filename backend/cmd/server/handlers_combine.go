package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

const maxCombineBooks = 25

type combineBooksRequest struct {
	PrimaryBookID int64   `json:"primary_book_id"`
	BookIDs       []int64 `json:"book_ids"`
}

type combineBooksResponse struct {
	PrimaryBookID int64   `json:"primary_book_id"`
	MergedBookIDs []int64 `json:"merged_book_ids"`
	FileCount     int     `json:"file_count"`
}

func combineBooksHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	var req combineBooksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request")
		return
	}

	bookIDs := uniquePositiveIDs(req.BookIDs)
	if len(bookIDs) < 2 {
		errorResponse(w, http.StatusBadRequest, "Select at least two books to combine")
		return
	}
	if len(bookIDs) > maxCombineBooks {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf("Cannot combine more than %d books at once", maxCombineBooks))
		return
	}
	if !containsInt64(bookIDs, req.PrimaryBookID) {
		errorResponse(w, http.StatusBadRequest, "Primary book must be one of the selected books")
		return
	}

	for _, bookID := range bookIDs {
		allowed, err := canAccessBook(current, bookID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
			return
		}
		if !allowed {
			errorResponse(w, http.StatusForbidden, "Permission denied")
			return
		}
	}

	libraryID, err := sharedLibraryID(bookIDs)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = libraryID

	secondaryIDs := make([]int64, 0, len(bookIDs)-1)
	for _, bookID := range bookIDs {
		if bookID != req.PrimaryBookID {
			secondaryIDs = append(secondaryIDs, bookID)
		}
	}

	fileCount, coverPaths, err := combineBooks(req.PrimaryBookID, secondaryIDs)
	if err != nil {
		slog.Error("Failed to combine books", "error", err, "primaryBookID", req.PrimaryBookID)
		errorResponse(w, http.StatusInternalServerError, "Failed to combine books")
		return
	}
	for _, coverPath := range coverPaths {
		if strings.TrimSpace(coverPath) != "" {
			_ = os.Remove(coverPath)
		}
	}

	jsonResponse(w, http.StatusOK, combineBooksResponse{
		PrimaryBookID: req.PrimaryBookID,
		MergedBookIDs: secondaryIDs,
		FileCount:     fileCount,
	})
}

func combineBooks(primaryID int64, secondaryIDs []int64) (int, []string, error) {
	tx, err := appDB.Begin()
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()

	secondaryArgs := int64Args(secondaryIDs)
	inClause := placeholders(len(secondaryIDs))

	reassign := []string{
		"UPDATE book_file SET book_id = ? WHERE book_id IN (" + inClause + ")",
		"UPDATE reading_position SET book_id = ? WHERE book_id IN (" + inClause + ")",
		"UPDATE reading_session SET book_id = ? WHERE book_id IN (" + inClause + ")",
		"UPDATE audio_item SET book_id = ? WHERE book_id IN (" + inClause + ")",
		"UPDATE audio_queue_item SET book_id = ? WHERE book_id IN (" + inClause + ")",
		"UPDATE bookmark SET book_id = ? WHERE book_id IN (" + inClause + ")",
		"UPDATE annotation SET book_id = ? WHERE book_id IN (" + inClause + ")",
		"UPDATE notebook_entry SET book_id = ? WHERE book_id IN (" + inClause + ")",
	}
	for _, stmt := range reassign {
		args := append([]any{primaryID}, secondaryArgs...)
		if _, err := tx.Exec(stmt, args...); err != nil {
			return 0, nil, fmt.Errorf("reassign %s: %w", stmt, err)
		}
	}

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO book_shelf (book_id, shelf_id)
		SELECT ?, shelf_id FROM book_shelf WHERE book_id IN (`+inClause+`)
	`, append([]any{primaryID}, secondaryArgs...)...); err != nil {
		return 0, nil, fmt.Errorf("merge shelves: %w", err)
	}

	coverPaths := []string{}
	metaIDs := []int64{}
	rows, err := tx.Query(`
		SELECT id, COALESCE(cover_path, '')
		FROM book_metadata
		WHERE book_id IN (`+inClause+`)
	`, secondaryArgs...)
	if err != nil {
		return 0, nil, err
	}
	for rows.Next() {
		var metaID int64
		var coverPath string
		if err := rows.Scan(&metaID, &coverPath); err != nil {
			rows.Close()
			return 0, nil, err
		}
		metaIDs = append(metaIDs, metaID)
		coverPaths = append(coverPaths, coverPath)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, nil, err
	}
	rows.Close()

	for _, metaID := range metaIDs {
		var ftsExists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM book_fts_docsize WHERE id = ?`, metaID).Scan(&ftsExists); err != nil {
			return 0, nil, fmt.Errorf("lookup fts: %w", err)
		}
		if ftsExists == 0 {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM book_fts WHERE rowid = ?`, metaID); err != nil {
			return 0, nil, fmt.Errorf("delete fts: %w", err)
		}
	}

	cleanup := []string{
		"DELETE FROM book_shelf WHERE book_id IN (" + inClause + ")",
		"DELETE FROM reading_progress WHERE book_id IN (" + inClause + ")",
		"DELETE FROM book_metadata_revision WHERE book_id IN (" + inClause + ")",
		"DELETE FROM book_metadata WHERE book_id IN (" + inClause + ")",
		"DELETE FROM book WHERE id IN (" + inClause + ")",
	}
	for _, stmt := range cleanup {
		if _, err := tx.Exec(stmt, secondaryArgs...); err != nil {
			return 0, nil, fmt.Errorf("cleanup %s: %w", stmt, err)
		}
	}

	var fileCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM book_file WHERE book_id = ? AND missing_at IS NULL`, primaryID).Scan(&fileCount); err != nil {
		return 0, nil, err
	}

	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}
	return fileCount, coverPaths, nil
}

func sharedLibraryID(bookIDs []int64) (int64, error) {
	args := int64Args(bookIDs)
	rows, err := appDB.Query(`SELECT DISTINCT library_id FROM book WHERE id IN (`+placeholders(len(bookIDs))+`)`, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var libraryID int64
	count := 0
	for rows.Next() {
		if err := rows.Scan(&libraryID); err != nil {
			return 0, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, fmt.Errorf("books not found")
	}
	if count != 1 {
		return 0, fmt.Errorf("Books must belong to the same library")
	}

	var found int
	if err := appDB.QueryRow(`SELECT COUNT(*) FROM book WHERE id IN (`+placeholders(len(bookIDs))+`)`, args...).Scan(&found); err != nil {
		return 0, err
	}
	if found != len(bookIDs) {
		return 0, fmt.Errorf("books not found")
	}
	return libraryID, nil
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func containsInt64(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func int64Args(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}
