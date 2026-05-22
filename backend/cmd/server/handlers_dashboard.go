package main

import (
	"net/http"
	"strconv"
	"time"
)

type dashboardSummaryResponse struct {
	TotalBooks int64 `json:"total_books"`
	Libraries  int64 `json:"libraries"`
	Reading    int64 `json:"reading"`
	Finished   int64 `json:"finished"`
}

type dashboardBookResponse struct {
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
	LastReadAt     int64   `json:"last_read_at"`
	Format         string  `json:"format"`
}

type dashboardBooksResponse struct {
	Books   []dashboardBookResponse `json:"books"`
	Offset  int                     `json:"offset"`
	Limit   int                     `json:"limit"`
	HasMore bool                    `json:"has_more"`
}

func parseDashboardLimit(r *http.Request, defaultLimit int) int {
	limit := defaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	return limit
}

func getDashboardSummaryHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	ownerClause, ownerArgs := userOwnershipClause(current, "l")

	var summary dashboardSummaryResponse
	_ = appDB.QueryRow(`
		SELECT COUNT(DISTINCT b.id)
		FROM book b
		JOIN library l ON b.library_id = l.id
		WHERE `+ownerClause+`
		  AND EXISTS (
			SELECT 1 FROM book_file bf
			WHERE bf.book_id = b.id AND bf.missing_at IS NULL
		  )
	`, ownerArgs...).Scan(&summary.TotalBooks)

	_ = appDB.QueryRow(`
		SELECT COUNT(*)
		FROM library l
		WHERE `+ownerClause, ownerArgs...).Scan(&summary.Libraries)

	progressArgs := append([]interface{}{}, ownerArgs...)
	_ = appDB.QueryRow(`
		SELECT
			COUNT(DISTINCT CASE WHEN rp.status = 'reading' THEN rp.book_id END),
			COUNT(DISTINCT CASE WHEN rp.status = 'finished' THEN rp.book_id END)
		FROM reading_progress rp
		JOIN book b ON rp.book_id = b.id
		JOIN library l ON b.library_id = l.id
		WHERE `+ownerClause+`
		  AND EXISTS (
			SELECT 1 FROM book_file bf
			WHERE bf.book_id = b.id AND bf.missing_at IS NULL
		  )
	`, progressArgs...).Scan(&summary.Reading, &summary.Finished)

	jsonResponse(w, http.StatusOK, summary)
}

func getDiscoverBooksHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	limit := parseDashboardLimit(r, 12)
	ownerClause, ownerArgs := userOwnershipClause(current, "l")

	baseFrom := `
		FROM book b
		JOIN library l ON b.library_id = l.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		LEFT JOIN (
			SELECT book_id, MIN(format) AS format
			FROM book_file
			WHERE missing_at IS NULL
			GROUP BY book_id
		) bf ON b.id = bf.book_id`
	baseWhere := ` WHERE ` + ownerClause + `
		AND COALESCE(l.exclude_from_suggestions, 0) = 0
		AND EXISTS (
			SELECT 1 FROM book_file active_bf
			WHERE active_bf.book_id = b.id AND active_bf.missing_at IS NULL
		)`

	var maxID int64
	if err := appDB.QueryRow(`SELECT COALESCE(MAX(b.id), 0) FROM book b JOIN library l ON b.library_id = l.id`+baseWhere, ownerArgs...).Scan(&maxID); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to prepare discovery books")
		return
	}
	if maxID <= 0 {
		jsonResponse(w, http.StatusOK, dashboardBooksResponse{Books: []dashboardBookResponse{}, Limit: limit})
		return
	}

	anchor := time.Now().UnixNano()%maxID + 1
	books, err := fetchDiscoverBooks(baseFrom, baseWhere, ownerArgs, anchor, limit)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch discovery books")
		return
	}

	jsonResponse(w, http.StatusOK, dashboardBooksResponse{
		Books:   books,
		Offset:  0,
		Limit:   limit,
		HasMore: false,
	})
}

func fetchDiscoverBooks(baseFrom, baseWhere string, ownerArgs []interface{}, anchor int64, limit int) ([]dashboardBookResponse, error) {
	books := make([]dashboardBookResponse, 0, limit)
	if err := appendDiscoverBooks(&books, baseFrom, baseWhere+" AND b.id >= ?", append(append([]interface{}{}, ownerArgs...), anchor), limit); err != nil {
		return nil, err
	}
	if len(books) < limit {
		if err := appendDiscoverBooks(&books, baseFrom, baseWhere+" AND b.id < ?", append(append([]interface{}{}, ownerArgs...), anchor), limit-len(books)); err != nil {
			return nil, err
		}
	}
	return books, nil
}

func appendDiscoverBooks(books *[]dashboardBookResponse, baseFrom, whereClause string, args []interface{}, limit int) error {
	if limit <= 0 {
		return nil
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
		       COALESCE(rp.updated_at, 0) as last_read_at,
		       COALESCE(bf.format, '') as format` + baseFrom + whereClause + `
		ORDER BY b.id ASC
		LIMIT ?`
	args = append(args, limit)

	rows, err := appDB.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var book dashboardBookResponse
		var opened int
		if err := rows.Scan(&book.ID, &book.LibraryID, &book.AddedAt, &book.Title, &book.Authors, &book.CoverPath, &book.CoverUpdatedOn, &book.Status, &book.Percent, &opened, &book.LastReadAt, &book.Format); err != nil {
			return err
		}
		book.Opened = opened == 1
		*books = append(*books, book)
	}
	return rows.Err()
}
