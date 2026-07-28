package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"cryptorum/internal/metaprotection"
)

type metadataRevisionResponse struct {
	ID              int64    `json:"id"`
	ChangedAt       int64    `json:"changed_at"`
	ChangedByUserID *int64   `json:"changed_by_user_id,omitempty"`
	ChangeSource    string   `json:"change_source"`
	ChangedFields   []string `json:"changed_fields"`
}

func ListBookMetadataRevisionsHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	bookID, err := strconv.ParseInt(chi.URLParam(r, "bookID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
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

	rows, err := appDB.Query(`
		SELECT id, changed_at, changed_by_user_id, change_source, changed_fields
		FROM book_metadata_revision
		WHERE book_id = ?
		ORDER BY changed_at DESC, id DESC
		LIMIT 25
	`, bookID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load metadata history")
		return
	}
	defer rows.Close()

	revisions := []metadataRevisionResponse{}
	for rows.Next() {
		var item metadataRevisionResponse
		var changedFieldsJSON string
		if err := rows.Scan(
			&item.ID,
			&item.ChangedAt,
			&item.ChangedByUserID,
			&item.ChangeSource,
			&changedFieldsJSON,
		); err != nil {
			continue
		}
		item.ChangedFields = metaprotection.NormalizeFields(parseMetadataJSONList(changedFieldsJSON))
		revisions = append(revisions, item)
	}
	jsonResponse(w, http.StatusOK, revisions)
}

func RestoreBookMetadataRevisionHandler(w http.ResponseWriter, r *http.Request) {
	currentUser := getUserFromContext(r.Context())
	bookID, err := strconv.ParseInt(chi.URLParam(r, "bookID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid book ID")
		return
	}
	revisionID, err := strconv.ParseInt(chi.URLParam(r, "revisionID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid revision ID")
		return
	}
	allowed, err := canAccessBook(currentUser, bookID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to start metadata restore")
		return
	}
	defer tx.Rollback()

	current, exists, err := metaprotection.LoadSnapshot(tx, bookID)
	if err != nil || !exists {
		errorResponse(w, http.StatusNotFound, "Book metadata not found")
		return
	}
	var previousJSON, changedFieldsJSON string
	if err := tx.QueryRow(`
		SELECT previous_metadata_json, changed_fields
		FROM book_metadata_revision
		WHERE id = ? AND book_id = ?
	`, revisionID, bookID).Scan(&previousJSON, &changedFieldsJSON); err != nil {
		errorResponse(w, http.StatusNotFound, "Metadata revision not found")
		return
	}

	var previous metaprotection.Snapshot
	if err := json.Unmarshal([]byte(previousJSON), &previous); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Metadata revision is invalid")
		return
	}
	fields := metaprotection.NormalizeFields(parseMetadataJSONList(changedFieldsJSON))
	if len(fields) == 0 {
		errorResponse(w, http.StatusBadRequest, "Metadata revision has no restorable fields")
		return
	}
	if err := metaprotection.RecordRevision(tx, current, fields, "revision_restore", currentUser.ID); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to record restore revision")
		return
	}

	restored := current
	previousLocks := metaprotection.ParseLocked(previous.LockedFields)
	restoredLocks := metaprotection.ParseLocked(current.LockedFields)
	for _, field := range fields {
		switch field {
		case metaprotection.FieldTitle:
			restored.Title = previous.Title
		case metaprotection.FieldAuthors:
			restored.Authors = previous.Authors
		case metaprotection.FieldSeries:
			restored.Series = previous.Series
		case metaprotection.FieldSeriesNumber:
			restored.SeriesNumber = previous.SeriesNumber
			restored.SeriesNumberDisplay = previous.SeriesNumberDisplay
		case metaprotection.FieldPublisher:
			restored.Publisher = previous.Publisher
		case metaprotection.FieldPubDate:
			restored.PubDate = previous.PubDate
		case metaprotection.FieldDescription:
			restored.Description = previous.Description
		case metaprotection.FieldRating:
			restored.Rating = previous.Rating
		case metaprotection.FieldTags:
			restored.Genres = previous.Genres
			restored.Tags = previous.Tags
		case metaprotection.FieldISBN:
			restored.ISBN = previous.ISBN
		case metaprotection.FieldASIN:
			restored.ASIN = previous.ASIN
		case metaprotection.FieldLanguage:
			restored.Language = previous.Language
		case metaprotection.FieldPageCount:
			restored.PageCount = previous.PageCount
		case metaprotection.FieldCover:
			if previous.CoverPath == "" {
				restored.CoverPath = ""
				restored.CoverSource = ""
				restored.CoverUpdatedOn = time.Now().Unix()
			} else if _, statErr := os.Stat(previous.CoverPath); statErr == nil {
				restored.CoverPath = previous.CoverPath
				restored.CoverSource = previous.CoverSource
				restored.CoverUpdatedOn = previous.CoverUpdatedOn
			}
		}
		if previousLocks[field] {
			restoredLocks[field] = true
		} else {
			delete(restoredLocks, field)
		}
	}
	restored.LockedFields = metaprotection.EncodeLocked(restoredLocks)
	restored.MetadataUpdatedAt = time.Now().Unix()
	restored.OwnerUserID = currentUser.ID

	_, err = tx.Exec(`
		UPDATE book_metadata SET
			title = ?, authors = ?, series = ?, series_number = ?, series_number_display = ?,
			publisher = ?, pub_date = ?, description = ?, rating = ?, genres = ?, tags = ?,
			isbn = ?, asin = ?, language = ?, page_count = ?, cover_path = ?, cover_source = ?,
			cover_updated_on = ?, locked_fields = ?, metadata_updated_at = ?, owner_user_id = ?
		WHERE book_id = ?
	`, restored.Title, restored.Authors, restored.Series, restored.SeriesNumber, restored.SeriesNumberDisplay,
		restored.Publisher, restored.PubDate, restored.Description, restored.Rating, restored.Genres,
		restored.Tags, restored.ISBN, restored.ASIN, restored.Language, restored.PageCount,
		restored.CoverPath, restored.CoverSource, restored.CoverUpdatedOn, restored.LockedFields,
		restored.MetadataUpdatedAt, restored.OwnerUserID, bookID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to restore metadata revision")
		return
	}
	if err := tx.Commit(); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to commit metadata restore")
		return
	}

	book, err := fetchBookDetail(bookID, currentUser)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Metadata restored but the book could not be reloaded")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"status": "restored",
		"book":   book,
	})
}
