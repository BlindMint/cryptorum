package metaprotection

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const (
	FieldTitle        = "title"
	FieldAuthors      = "authors"
	FieldSeries       = "series"
	FieldSeriesNumber = "series_number"
	FieldPublisher    = "publisher"
	FieldPubDate      = "pub_date"
	FieldDescription  = "description"
	FieldRating       = "rating"
	FieldTags         = "tags"
	FieldISBN         = "isbn"
	FieldASIN         = "asin"
	FieldLanguage     = "language"
	FieldPageCount    = "page_count"
	FieldCover        = "cover"
)

var AllFields = []string{
	FieldTitle,
	FieldAuthors,
	FieldSeries,
	FieldSeriesNumber,
	FieldPublisher,
	FieldPubDate,
	FieldDescription,
	FieldRating,
	FieldTags,
	FieldISBN,
	FieldASIN,
	FieldLanguage,
	FieldPageCount,
	FieldCover,
}

type DBTX interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

type Snapshot struct {
	BookID              int64   `json:"book_id"`
	Title               string  `json:"title"`
	Authors             string  `json:"authors"`
	Series              string  `json:"series"`
	SeriesNumber        float64 `json:"series_number"`
	SeriesNumberDisplay string  `json:"series_number_display"`
	Publisher           string  `json:"publisher"`
	PubDate             string  `json:"pub_date"`
	Description         string  `json:"description"`
	Rating              float64 `json:"rating"`
	Genres              string  `json:"genres"`
	Tags                string  `json:"tags"`
	ISBN                string  `json:"isbn"`
	ASIN                string  `json:"asin"`
	Language            string  `json:"language"`
	PageCount           int64   `json:"page_count"`
	CoverPath           string  `json:"cover_path"`
	CoverSource         string  `json:"cover_source"`
	CoverUpdatedOn      int64   `json:"cover_updated_on"`
	LockedFields        string  `json:"locked_fields"`
	ExtractedFromHash   string  `json:"extracted_from_hash"`
	MetadataUpdatedAt   int64   `json:"metadata_updated_at"`
	OwnerUserID         int64   `json:"owner_user_id"`
}

func LoadSnapshot(db DBTX, bookID int64) (Snapshot, bool, error) {
	var snapshot Snapshot
	err := db.QueryRow(`
		SELECT book_id,
		       COALESCE(title, ''),
		       COALESCE(authors, '[]'),
		       COALESCE(series, ''),
		       COALESCE(series_number, 0),
		       COALESCE(series_number_display, ''),
		       COALESCE(publisher, ''),
		       COALESCE(pub_date, ''),
		       COALESCE(description, ''),
		       COALESCE(rating, 0),
		       COALESCE(genres, '[]'),
		       COALESCE(tags, '[]'),
		       COALESCE(isbn, ''),
		       COALESCE(asin, ''),
		       COALESCE(language, ''),
		       COALESCE(page_count, 0),
		       COALESCE(cover_path, ''),
		       COALESCE(cover_source, ''),
		       COALESCE(cover_updated_on, 0),
		       COALESCE(locked_fields, '[]'),
		       COALESCE(extracted_from_hash, ''),
		       COALESCE(metadata_updated_at, 0),
		       COALESCE(owner_user_id, 1)
		FROM book_metadata
		WHERE book_id = ?
	`, bookID).Scan(
		&snapshot.BookID,
		&snapshot.Title,
		&snapshot.Authors,
		&snapshot.Series,
		&snapshot.SeriesNumber,
		&snapshot.SeriesNumberDisplay,
		&snapshot.Publisher,
		&snapshot.PubDate,
		&snapshot.Description,
		&snapshot.Rating,
		&snapshot.Genres,
		&snapshot.Tags,
		&snapshot.ISBN,
		&snapshot.ASIN,
		&snapshot.Language,
		&snapshot.PageCount,
		&snapshot.CoverPath,
		&snapshot.CoverSource,
		&snapshot.CoverUpdatedOn,
		&snapshot.LockedFields,
		&snapshot.ExtractedFromHash,
		&snapshot.MetadataUpdatedAt,
		&snapshot.OwnerUserID,
	)
	if err == sql.ErrNoRows {
		return Snapshot{BookID: bookID, Authors: "[]", Genres: "[]", Tags: "[]", LockedFields: "[]"}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, err
	}
	return snapshot, true, nil
}

// LibraryProtectionEnabled reports whether the book's library protects all
// existing metadata from automatic updates. Field-level locks remain separate
// so disabling the library policy does not discard explicit user protections.
func LibraryProtectionEnabled(db DBTX, bookID int64) (bool, error) {
	var enabled int
	err := db.QueryRow(`
		SELECT COALESCE(l.metadata_protection_enabled, 0)
		FROM book b
		JOIN library l ON l.id = b.library_id
		WHERE b.id = ?
	`, bookID).Scan(&enabled)
	if err != nil {
		return false, err
	}
	return enabled == 1, nil
}

func RecordRevision(db DBTX, snapshot Snapshot, fields []string, source string, actorUserID int64) error {
	fields = NormalizeFields(fields)
	if len(fields) == 0 {
		return nil
	}
	previousJSON, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	var actor any
	if actorUserID > 0 {
		actor = actorUserID
	}
	result, err := db.Exec(`
		INSERT INTO book_metadata_revision
		    (book_id, changed_at, changed_by_user_id, change_source, changed_fields, previous_metadata_json)
		VALUES (?, ?, ?, ?, ?, ?)
	`, snapshot.BookID, time.Now().Unix(), actor, strings.TrimSpace(source), string(fieldsJSON), string(previousJSON))
	if err != nil {
		return err
	}
	revisionID, _ := result.LastInsertId()
	_, err = db.Exec(`
		DELETE FROM book_metadata_revision
		WHERE book_id = ?
		  AND id NOT IN (
		      SELECT id
		      FROM book_metadata_revision
		      WHERE book_id = ?
		      ORDER BY changed_at DESC, id DESC
		      LIMIT 50
		  )
		  AND id <> ?
	`, snapshot.BookID, snapshot.BookID, revisionID)
	return err
}

func ParseLocked(raw string) map[string]bool {
	var fields []string
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		fields = nil
	}
	result := make(map[string]bool, len(fields))
	for _, field := range NormalizeFields(fields) {
		result[field] = true
	}
	return result
}

func MergeLocked(raw string, fields ...string) string {
	locked := ParseLocked(raw)
	for _, field := range NormalizeFields(fields) {
		locked[field] = true
	}
	return EncodeLocked(locked)
}

func RemoveLocked(raw string, fields ...string) string {
	locked := ParseLocked(raw)
	for _, field := range NormalizeFields(fields) {
		delete(locked, field)
	}
	return EncodeLocked(locked)
}

func EncodeLocked(fields map[string]bool) string {
	values := make([]string, 0, len(fields))
	for field, enabled := range fields {
		if enabled {
			values = append(values, field)
		}
	}
	sort.Strings(values)
	encoded, _ := json.Marshal(values)
	return string(encoded)
}

func NormalizeFields(fields []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = normalizeField(field)
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		result = append(result, field)
	}
	sort.Strings(result)
	return result
}

func normalizeField(field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	switch field {
	case "cover_path", "cover_source":
		field = FieldCover
	case "genres":
		field = FieldTags
	}
	for _, allowed := range AllFields {
		if field == allowed {
			return field
		}
	}
	return ""
}
