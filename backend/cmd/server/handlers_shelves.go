package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type ShelfResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	IsMagic   int    `json:"is_magic"`
	RulesJSON string `json:"rules_json,omitempty"`
	SortBy    string `json:"sort_by,omitempty"`
	SortDir   string `json:"sort_dir,omitempty"`
	SortOrder int64  `json:"sort_order"`
	BookCount int64  `json:"book_count"`
}

func getShelvesHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	ownerClause, ownerArgs := userOwnershipClause(current, "s")
	libraryOwnerClause, libraryOwnerArgs := userOwnershipClause(current, "l")
	queryArgs := append([]interface{}{}, libraryOwnerArgs...)
	queryArgs = append(queryArgs, ownerArgs...)
	rows, err := appDB.Query(`
		SELECT s.id, s.name, COALESCE(s.icon, '') as icon, s.is_magic,
		       COALESCE(s.rules_json, '') as rules_json,
		       COALESCE(s.sort_by, '') as sort_by,
		       COALESCE(s.sort_dir, '') as sort_dir,
		       COALESCE(s.sort_order, 0) as sort_order,
		       COUNT(DISTINCT CASE WHEN bf.id IS NOT NULL AND `+libraryOwnerClause+` THEN b.id END) as book_count
		FROM shelf s
		LEFT JOIN book_shelf bs ON s.id = bs.shelf_id
		LEFT JOIN book b ON bs.book_id = b.id
		LEFT JOIN library l ON b.library_id = l.id
		LEFT JOIN book_file bf ON bf.book_id = b.id AND bf.missing_at IS NULL
		WHERE `+ownerClause+`
		GROUP BY s.id, s.name, s.icon, s.is_magic, s.rules_json, s.sort_by, s.sort_dir, s.sort_order
		ORDER BY CASE WHEN COALESCE(s.sort_order, 0) = 0 THEN 1 ELSE 0 END, s.sort_order, s.name COLLATE NOCASE
	`, queryArgs...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch shelves")
		return
	}

	shelves := []ShelfResponse{}
	for rows.Next() {
		var s ShelfResponse
		if err := rows.Scan(&s.ID, &s.Name, &s.Icon, &s.IsMagic, &s.RulesJSON, &s.SortBy, &s.SortDir, &s.SortOrder, &s.BookCount); err != nil {
			continue
		}
		shelves = append(shelves, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch shelves")
		return
	}
	rows.Close()

	for index := range shelves {
		if shelves[index].IsMagic != 1 {
			continue
		}
		count, err := countMagicShelfBooks(shelves[index].RulesJSON, current)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to count magic shelf books")
			return
		}
		shelves[index].BookCount = count
	}

	jsonResponse(w, http.StatusOK, shelves)
}

func rejectMagicShelfMembershipMutation(w http.ResponseWriter, shelfID string) bool {
	var isMagic int
	if err := appDB.QueryRow("SELECT is_magic FROM shelf WHERE id = ?", shelfID).Scan(&isMagic); err != nil {
		errorResponse(w, http.StatusNotFound, "Shelf not found")
		return true
	}
	if isMagic == 1 {
		errorResponse(w, http.StatusBadRequest, "Smart shelves are managed by rules")
		return true
	}
	return false
}

func buildMagicShelfConditions(rulesJSON string) (string, []interface{}, error) {
	if strings.TrimSpace(rulesJSON) == "" {
		return "1 = 1", nil, nil
	}

	// Parse rules
	var rules struct {
		Conditions []struct {
			Field    string      `json:"field"`
			Operator string      `json:"operator"`
			Value    interface{} `json:"value"`
		} `json:"conditions"`
	}

	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return "", nil, err
	}

	// Build WHERE clause from conditions
	var conditions []string
	var args []interface{}

	for _, condition := range rules.Conditions {
		switch condition.Field {
		case "status":
			switch condition.Operator {
			case "equals":
				conditions = append(conditions, "COALESCE(rp.status, 'unread') = ?")
				args = append(args, condition.Value)
			case "not_equals":
				conditions = append(conditions, "COALESCE(rp.status, 'unread') != ?")
				args = append(args, condition.Value)
			}
		case "authors":
			switch condition.Operator {
			case "contains":
				key := normalizedAuthorMatchKey(strings.TrimSpace(fmt.Sprintf("%v", condition.Value)))
				if key != "" {
					conditions = append(conditions, `EXISTS (SELECT 1 FROM json_each(COALESCE(bm.authors, '[]')) WHERE `+normalizedAuthorSQLExpression("value")+` LIKE ?)`)
					args = append(args, "%"+key+"%")
				}
			}
		case "series":
			value := strings.TrimSpace(fmt.Sprintf("%v", condition.Value))
			if value == "" {
				break
			}
			switch condition.Operator {
			case "equals":
				conditions = append(conditions, "COALESCE(bm.series, '') = ?")
				args = append(args, value)
			case "contains":
				conditions = append(conditions, "COALESCE(bm.series, '') LIKE ?")
				args = append(args, "%"+value+"%")
			}
		case "genres", "tags":
			value := strings.TrimSpace(fmt.Sprintf("%v", condition.Value))
			if value == "" {
				break
			}
			switch condition.Operator {
			case "contains":
				tagCondition := hierarchicalJSONFilterCondition("bm.tags")
				genreCondition := hierarchicalJSONFilterCondition("bm.genres")
				conditions = append(conditions, "("+tagCondition+" OR "+genreCondition+")")
				args = append(args, hierarchicalJSONFilterArgs(value)...)
				args = append(args, hierarchicalJSONFilterArgs(value)...)
			}
		case "publisher":
			value := strings.TrimSpace(fmt.Sprintf("%v", condition.Value))
			if value == "" {
				break
			}
			switch condition.Operator {
			case "equals":
				conditions = append(conditions, "COALESCE(bm.publisher, '') = ?")
				args = append(args, value)
			case "contains":
				conditions = append(conditions, "COALESCE(bm.publisher, '') LIKE ?")
				args = append(args, "%"+value+"%")
			}
		case "language":
			value := strings.TrimSpace(fmt.Sprintf("%v", condition.Value))
			if value == "" {
				break
			}
			switch condition.Operator {
			case "equals":
				conditions = append(conditions, "COALESCE(bm.language, '') = ?")
				args = append(args, value)
			}
		case "rating":
			if rating, err := strconv.ParseFloat(fmt.Sprintf("%v", condition.Value), 64); err == nil {
				switch condition.Operator {
				case "equals":
					conditions = append(conditions, "COALESCE(bm.rating, 0) = ?")
					args = append(args, rating)
				case "greater_than":
					conditions = append(conditions, "COALESCE(bm.rating, 0) > ?")
					args = append(args, rating)
				case "less_than":
					conditions = append(conditions, "COALESCE(bm.rating, 0) < ?")
					args = append(args, rating)
				}
			}
		case "page_count":
			if pages, err := strconv.Atoi(fmt.Sprintf("%v", condition.Value)); err == nil {
				switch condition.Operator {
				case "equals":
					conditions = append(conditions, "COALESCE(bm.page_count, 0) = ?")
					args = append(args, pages)
				case "greater_than":
					conditions = append(conditions, "COALESCE(bm.page_count, 0) > ?")
					args = append(args, pages)
				case "less_than":
					conditions = append(conditions, "COALESCE(bm.page_count, 0) < ?")
					args = append(args, pages)
				}
			}
		}
	}

	whereClause := "1 = 1"
	if len(conditions) > 0 {
		whereClause = strings.Join(conditions, " AND ")
	} else if len(rules.Conditions) > 0 {
		whereClause = "0 = 1"
	}

	return whereClause, args, nil
}

func evaluateMagicShelfRules(rulesJSON string, sortBy string, sortDir string, user *AppUser) (*sql.Rows, error) {
	whereClause, args, err := buildMagicShelfConditions(rulesJSON)
	if err != nil {
		return nil, err
	}
	ownerClause, ownerArgs := userOwnershipClause(user, "l")
	orderBy := bookListOrderBy(sortBy, sortDir)

	query := fmt.Sprintf(`
		SELECT b.id, b.library_id, b.added_at,
		       COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.series, '') as series,
		       COALESCE(bm.series_number, 0) as series_number,
		       COALESCE(bm.series_number_display, '') as series_number_display,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(rp.status, 'unread') as status,
		       COALESCE(rp.updated_at, 0) as last_read_at,
		       COALESCE(bf.format, '') as format
			FROM book b
			JOIN library l ON b.library_id = l.id
			LEFT JOIN book_metadata bm ON b.id = bm.book_id
			LEFT JOIN reading_progress rp ON b.id = rp.book_id AND rp.owner_user_id = ?
			LEFT JOIN (
				SELECT book_id, MIN(format) AS format
				FROM book_file
				WHERE missing_at IS NULL
				GROUP BY book_id
			) bf ON b.id = bf.book_id
			WHERE (%s) AND %s
			  AND EXISTS (SELECT 1 FROM book_file bf WHERE bf.book_id = b.id AND bf.missing_at IS NULL)
			ORDER BY %s
		`, whereClause, ownerClause, orderBy)

	queryArgs := append([]interface{}{userIDForScopedRows(user)}, args...)
	queryArgs = append(queryArgs, ownerArgs...)
	return appDB.Query(query, queryArgs...)
}

func countMagicShelfBooks(rulesJSON string, user *AppUser) (int64, error) {
	whereClause, args, err := buildMagicShelfConditions(rulesJSON)
	if err != nil {
		return 0, err
	}
	ownerClause, ownerArgs := userOwnershipClause(user, "l")

	query := fmt.Sprintf(`
		SELECT COUNT(DISTINCT b.id)
		FROM book b
		JOIN library l ON b.library_id = l.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id AND rp.owner_user_id = ?
		WHERE (%s) AND %s
		  AND EXISTS (SELECT 1 FROM book_file bf WHERE bf.book_id = b.id AND bf.missing_at IS NULL)
	`, whereClause, ownerClause)

	queryArgs := append([]interface{}{userIDForScopedRows(user)}, args...)
	queryArgs = append(queryArgs, ownerArgs...)
	var count int64
	if err := appDB.QueryRow(query, queryArgs...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func createShelfHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageLibraries) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	var req struct {
		Name      string `json:"name"`
		Icon      string `json:"icon"`
		IsMagic   int    `json:"is_magic"`
		RulesJSON string `json:"rules_json"`
		SortBy    string `json:"sort_by"`
		SortDir   string `json:"sort_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		errorResponse(w, http.StatusBadRequest, "Invalid request: name is required")
		return
	}

	var nextSortOrder int64
	_ = appDB.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM shelf WHERE owner_user_id = ?`, current.ID).Scan(&nextSortOrder)
	result, err := appDB.Exec(`
		INSERT INTO shelf (name, icon, is_magic, rules_json, sort_by, sort_dir, owner_user_id, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, req.Name, req.Icon, req.IsMagic, req.RulesJSON, req.SortBy, req.SortDir, current.ID, nextSortOrder)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to create shelf")
		return
	}

	id, _ := result.LastInsertId()
	jsonResponse(w, http.StatusCreated, ShelfResponse{
		ID:        id,
		Name:      req.Name,
		Icon:      req.Icon,
		IsMagic:   req.IsMagic,
		SortBy:    req.SortBy,
		SortDir:   req.SortDir,
		SortOrder: nextSortOrder,
	})
}

func getShelfHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	shelfID := chi.URLParam(r, "shelfID")
	allowed, err := canAccessShelf(current, mustInt64(shelfID))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify shelf access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var s ShelfResponse
	libraryOwnerClause, libraryOwnerArgs := userOwnershipClause(current, "l")
	err = appDB.QueryRow(`
		SELECT s.id, s.name, COALESCE(s.icon, '') as icon, s.is_magic,
		       COALESCE(s.rules_json, '') as rules_json,
		       COALESCE(s.sort_by, '') as sort_by,
		       COALESCE(s.sort_dir, '') as sort_dir,
		       COALESCE(s.sort_order, 0) as sort_order,
		       COUNT(DISTINCT CASE WHEN bf.id IS NOT NULL AND `+libraryOwnerClause+` THEN b.id END) as book_count
		FROM shelf s
		LEFT JOIN book_shelf bs ON s.id = bs.shelf_id
		LEFT JOIN book b ON bs.book_id = b.id
		LEFT JOIN library l ON b.library_id = l.id
		LEFT JOIN book_file bf ON bf.book_id = b.id AND bf.missing_at IS NULL
		WHERE s.id = ?
		GROUP BY s.id, s.name, s.icon, s.is_magic, s.rules_json, s.sort_by, s.sort_dir, s.sort_order
	`, append(libraryOwnerArgs, shelfID)...).Scan(&s.ID, &s.Name, &s.Icon, &s.IsMagic, &s.RulesJSON, &s.SortBy, &s.SortDir, &s.SortOrder, &s.BookCount)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Shelf not found")
		return
	}
	if s.IsMagic == 1 {
		count, err := countMagicShelfBooks(s.RulesJSON, current)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to count magic shelf books")
			return
		}
		s.BookCount = count
	}

	jsonResponse(w, http.StatusOK, s)
}

func updateShelfOrderHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageLibraries) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	if current == nil {
		errorResponse(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var req struct {
		ShelfIDs []int64 `json:"shelf_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.ShelfIDs) == 0 {
		errorResponse(w, http.StatusBadRequest, "Invalid shelf order")
		return
	}

	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to start transaction")
		return
	}
	defer tx.Rollback()

	for index, id := range req.ShelfIDs {
		query := `UPDATE shelf SET sort_order = ? WHERE id = ?`
		args := []interface{}{index + 1, id}
		if !userCanAccessAllData(current) {
			query += ` AND owner_user_id = ?`
			args = append(args, current.ID)
		}
		result, err := tx.Exec(query, args...)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to update shelf order")
			return
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			errorResponse(w, http.StatusForbidden, "Permission denied")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to save shelf order")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func updateShelfHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	shelfID := chi.URLParam(r, "shelfID")
	allowed, err := canAccessShelf(current, mustInt64(shelfID))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify shelf access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req struct {
		Name      string `json:"name"`
		Icon      string `json:"icon"`
		RulesJSON string `json:"rules_json"`
		SortBy    string `json:"sort_by"`
		SortDir   string `json:"sort_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		errorResponse(w, http.StatusBadRequest, "Invalid request: name is required")
		return
	}

	_, err = appDB.Exec(`
		UPDATE shelf SET name = ?, icon = ?, rules_json = ?, sort_by = ?, sort_dir = ?
		WHERE id = ?
	`, req.Name, req.Icon, req.RulesJSON, req.SortBy, req.SortDir, shelfID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update shelf")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func deleteShelfHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	shelfID := chi.URLParam(r, "shelfID")
	allowed, err := canAccessShelf(current, mustInt64(shelfID))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify shelf access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	appDB.Exec("DELETE FROM book_shelf WHERE shelf_id = ?", shelfID)
	_, err = appDB.Exec("DELETE FROM shelf WHERE id = ?", shelfID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to delete shelf")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func getShelfBooksHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	shelfID := chi.URLParam(r, "shelfID")
	allowed, err := canAccessShelf(current, mustInt64(shelfID))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify shelf access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var isMagic int
	var rulesJSON string
	var sortBy string
	var sortDir string
	err = appDB.QueryRow("SELECT is_magic, COALESCE(rules_json, ''), COALESCE(sort_by, ''), COALESCE(sort_dir, '') FROM shelf WHERE id = ?", shelfID).Scan(&isMagic, &rulesJSON, &sortBy, &sortDir)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch shelf info")
		return
	}

	var rows *sql.Rows
	if isMagic == 1 {
		rows, err = evaluateMagicShelfRules(rulesJSON, sortBy, sortDir, current)
	} else {
		ownerClause, ownerArgs := userOwnershipClause(current, "l")
		query := `
			SELECT b.id, b.library_id, b.added_at,
			       COALESCE(bm.title, '') as title,
			       COALESCE(bm.authors, '[]') as authors,
			       COALESCE(bm.series, '') as series,
			       COALESCE(bm.series_number, 0) as series_number,
			       COALESCE(bm.series_number_display, '') as series_number_display,
			       COALESCE(bm.cover_path, '') as cover_path,
			       COALESCE(rp.status, 'unread') as status,
			       COALESCE(rp.updated_at, 0) as last_read_at,
			       COALESCE(bf.format, '') as format
			FROM book_shelf bs
				JOIN book b ON bs.book_id = b.id
				JOIN library l ON b.library_id = l.id
				LEFT JOIN book_metadata bm ON b.id = bm.book_id
				LEFT JOIN reading_progress rp ON b.id = rp.book_id AND rp.owner_user_id = ?
				LEFT JOIN (
					SELECT book_id, MIN(format) AS format
					FROM book_file
					WHERE missing_at IS NULL
					GROUP BY book_id
				) bf ON b.id = bf.book_id
				WHERE bs.shelf_id = ? AND ` + ownerClause + `
				  AND EXISTS (SELECT 1 FROM book_file bf WHERE bf.book_id = b.id AND bf.missing_at IS NULL)
				ORDER BY ` + bookListOrderBy(sortBy, sortDir)
		rows, err = appDB.Query(query, append([]interface{}{userIDForScopedRows(current), shelfID}, ownerArgs...)...)
	}

	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch shelf books")
		return
	}
	defer rows.Close()

	type BookResponse struct {
		ID                  int64   `json:"id"`
		LibraryID           int64   `json:"library_id"`
		AddedAt             int64   `json:"added_at"`
		Title               string  `json:"title"`
		Authors             string  `json:"authors"`
		Series              string  `json:"series"`
		SeriesNumber        float64 `json:"series_number"`
		SeriesNumberDisplay string  `json:"series_number_display"`
		CoverPath           string  `json:"cover_path"`
		Status              string  `json:"status"`
		LastReadAt          int64   `json:"last_read_at"`
		Format              string  `json:"format"`
	}

	books := []BookResponse{}
	for rows.Next() {
		var b BookResponse
		if err := rows.Scan(&b.ID, &b.LibraryID, &b.AddedAt, &b.Title, &b.Authors, &b.Series, &b.SeriesNumber, &b.SeriesNumberDisplay, &b.CoverPath, &b.Status, &b.LastReadAt, &b.Format); err != nil {
			continue
		}
		books = append(books, b)
	}

	jsonResponse(w, http.StatusOK, books)
}

func addBookToShelfHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	shelfID := chi.URLParam(r, "shelfID")

	allowed, err := canAccessShelf(current, mustInt64(shelfID))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify shelf access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	if rejectMagicShelfMembershipMutation(w, shelfID) {
		return
	}

	var req struct {
		BookID int64 `json:"book_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BookID == 0 {
		errorResponse(w, http.StatusBadRequest, "Invalid request: book_id is required")
		return
	}

	bookAllowed, err := canAccessBook(current, req.BookID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !bookAllowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	_, err = appDB.Exec(`
		INSERT OR IGNORE INTO book_shelf (book_id, shelf_id) VALUES (?, ?)
	`, req.BookID, shelfID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to add book to shelf")
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func removeBookFromShelfHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	shelfID := chi.URLParam(r, "shelfID")
	bookID := chi.URLParam(r, "bookID")
	allowed, err := canAccessShelf(current, mustInt64(shelfID))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify shelf access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	if rejectMagicShelfMembershipMutation(w, shelfID) {
		return
	}

	bookAllowed, err := canAccessBook(current, mustInt64(bookID))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
		return
	}
	if !bookAllowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	_, err = appDB.Exec("DELETE FROM book_shelf WHERE shelf_id = ? AND book_id = ?", shelfID, bookID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to remove book from shelf")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func bulkRemoveBooksFromShelfHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	shelfID := chi.URLParam(r, "shelfID")

	var req struct {
		BookIDs []int64 `json:"book_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.BookIDs) == 0 {
		errorResponse(w, http.StatusBadRequest, "No books selected")
		return
	}

	allowed, err := canAccessShelf(current, mustInt64(shelfID))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to verify shelf access")
		return
	}
	if !allowed {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}
	if rejectMagicShelfMembershipMutation(w, shelfID) {
		return
	}

	tx, err := appDB.Begin()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to remove books from shelf")
		return
	}
	defer tx.Rollback()

	for _, bookID := range req.BookIDs {
		bookAllowed, err := canAccessBook(current, bookID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to verify book access")
			return
		}
		if !bookAllowed {
			errorResponse(w, http.StatusForbidden, "Permission denied")
			return
		}

		if _, err := tx.Exec("DELETE FROM book_shelf WHERE shelf_id = ? AND book_id = ?", shelfID, bookID); err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to remove books from shelf")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to remove books from shelf")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
