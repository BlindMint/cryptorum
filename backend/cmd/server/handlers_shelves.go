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
	BookCount int64  `json:"book_count"`
}

func getShelvesHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	ownerClause, ownerArgs := userOwnershipClause(current, "s")
	rows, err := appDB.Query(`
		SELECT s.id, s.name, COALESCE(s.icon, '') as icon, s.is_magic,
		       COALESCE(s.rules_json, '') as rules_json,
		       COALESCE(s.sort_by, '') as sort_by,
		       COALESCE(s.sort_dir, '') as sort_dir,
		       COUNT(bs.book_id) as book_count
		FROM shelf s
		LEFT JOIN book_shelf bs ON s.id = bs.shelf_id
		WHERE `+ownerClause+`
		GROUP BY s.id
		ORDER BY s.name
	`, ownerArgs...)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch shelves")
		return
	}
	defer rows.Close()

	shelves := []ShelfResponse{}
	for rows.Next() {
		var s ShelfResponse
		if err := rows.Scan(&s.ID, &s.Name, &s.Icon, &s.IsMagic, &s.RulesJSON, &s.SortBy, &s.SortDir, &s.BookCount); err != nil {
			continue
		}
		shelves = append(shelves, s)
	}

	jsonResponse(w, http.StatusOK, shelves)
}

func evaluateMagicShelfRules(shelfID string, rulesJSON string, user *AppUser) (*sql.Rows, error) {
	// Parse rules
	var rules struct {
		Conditions []struct {
			Field    string      `json:"field"`
			Operator string      `json:"operator"`
			Value    interface{} `json:"value"`
		} `json:"conditions"`
	}

	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return nil, err
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
				conditions = append(conditions, "COALESCE(bm.authors, '[]') LIKE ?")
				args = append(args, "%"+condition.Value.(string)+"%")
			}
		case "series":
			switch condition.Operator {
			case "equals":
				conditions = append(conditions, "COALESCE(bm.series, '') = ?")
				args = append(args, condition.Value)
			case "contains":
				conditions = append(conditions, "COALESCE(bm.series, '') LIKE ?")
				args = append(args, "%"+condition.Value.(string)+"%")
			}
		case "genres":
			switch condition.Operator {
			case "contains":
				conditions = append(conditions, "COALESCE(bm.genres, '[]') LIKE ?")
				args = append(args, "%"+condition.Value.(string)+"%")
			}
		case "publisher":
			switch condition.Operator {
			case "equals":
				conditions = append(conditions, "COALESCE(bm.publisher, '') = ?")
				args = append(args, condition.Value)
			case "contains":
				conditions = append(conditions, "COALESCE(bm.publisher, '') LIKE ?")
				args = append(args, "%"+condition.Value.(string)+"%")
			}
		case "language":
			switch condition.Operator {
			case "equals":
				conditions = append(conditions, "COALESCE(bm.language, '') = ?")
				args = append(args, condition.Value)
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
	}
	ownerClause, ownerArgs := userOwnershipClause(user, "l")

	query := fmt.Sprintf(`
		SELECT b.id, b.library_id, b.added_at,
		       COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(rp.status, 'unread') as status
		FROM book b
		JOIN library l ON b.library_id = l.id
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		WHERE (%s) AND %s
		ORDER BY bm.title
	`, whereClause, ownerClause)

	return appDB.Query(query, append(args, ownerArgs...)...)
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

	result, err := appDB.Exec(`
		INSERT INTO shelf (name, icon, is_magic, rules_json, sort_by, sort_dir, owner_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, req.Name, req.Icon, req.IsMagic, req.RulesJSON, req.SortBy, req.SortDir, current.ID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to create shelf")
		return
	}

	id, _ := result.LastInsertId()
	jsonResponse(w, http.StatusCreated, ShelfResponse{
		ID:      id,
		Name:    req.Name,
		Icon:    req.Icon,
		IsMagic: req.IsMagic,
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
	err = appDB.QueryRow(`
		SELECT s.id, s.name, COALESCE(s.icon, '') as icon, s.is_magic,
		       COALESCE(s.rules_json, '') as rules_json,
		       COALESCE(s.sort_by, '') as sort_by,
		       COALESCE(s.sort_dir, '') as sort_dir,
		       COUNT(bs.book_id) as book_count
		FROM shelf s
		LEFT JOIN book_shelf bs ON s.id = bs.shelf_id
		WHERE s.id = ?
		GROUP BY s.id
	`, shelfID).Scan(&s.ID, &s.Name, &s.Icon, &s.IsMagic, &s.RulesJSON, &s.SortBy, &s.SortDir, &s.BookCount)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Shelf not found")
		return
	}

	jsonResponse(w, http.StatusOK, s)
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

	// Check if this is a magic shelf
	var isMagic int
	var rulesJSON string
	err = appDB.QueryRow("SELECT is_magic, rules_json FROM shelf WHERE id = ?", shelfID).Scan(&isMagic, &rulesJSON)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch shelf info")
		return
	}

	var rows *sql.Rows
	if isMagic == 1 {
		// For magic shelves, evaluate rules to get matching books
		rows, err = evaluateMagicShelfRules(shelfID, rulesJSON, current)
	} else {
		// For regular shelves, get manually added books
		ownerClause, ownerArgs := userOwnershipClause(current, "l")
		rows, err = appDB.Query(`
			SELECT b.id, b.library_id, b.added_at,
			       COALESCE(bm.title, '') as title,
			       COALESCE(bm.authors, '[]') as authors,
			       COALESCE(bm.cover_path, '') as cover_path,
			       COALESCE(rp.status, 'unread') as status
			FROM book_shelf bs
			JOIN book b ON bs.book_id = b.id
			JOIN library l ON b.library_id = l.id
			LEFT JOIN book_metadata bm ON b.id = bm.book_id
			LEFT JOIN reading_progress rp ON b.id = rp.book_id
			WHERE bs.shelf_id = ? AND `+ownerClause+`
			ORDER BY bm.title
		`, append([]interface{}{shelfID}, ownerArgs...)...)
	}

	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch shelf books")
		return
	}
	defer rows.Close()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to fetch shelf books")
		return
	}
	defer rows.Close()

	type BookResponse struct {
		ID        int64  `json:"id"`
		LibraryID int64  `json:"library_id"`
		AddedAt   int64  `json:"added_at"`
		Title     string `json:"title"`
		Authors   string `json:"authors"`
		CoverPath string `json:"cover_path"`
		Status    string `json:"status"`
	}

	books := []BookResponse{}
	for rows.Next() {
		var b BookResponse
		if err := rows.Scan(&b.ID, &b.LibraryID, &b.AddedAt, &b.Title, &b.Authors, &b.CoverPath, &b.Status); err != nil {
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
