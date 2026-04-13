package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"cryptorum/internal/covers"
)

// MetadataProvider represents a metadata provider
type MetadataProvider struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// MetadataCandidate represents a metadata candidate from a provider
type MetadataCandidate struct {
	Provider    string   `json:"provider"`
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	Series      string   `json:"series,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	PubDate     string   `json:"pub_date,omitempty"`
	Description string   `json:"description,omitempty"`
	ISBN        string   `json:"isbn,omitempty"`
	CoverURL    string   `json:"cover_url,omitempty"`
	PageCount   int      `json:"page_count,omitempty"`
	Language    string   `json:"language,omitempty"`
	Rating      float64  `json:"rating,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	MatchScore  float64  `json:"match_score,omitempty"`
}

// GoogleBooksResponse represents the Google Books API response
type GoogleBooksResponse struct {
	Items []GoogleBooksItem `json:"items,omitempty"`
}

// GoogleBooksItem represents a Google Books item
type GoogleBooksItem struct {
	VolumeInfo GoogleBooksVolumeInfo `json:"volumeInfo"`
	ID         string                `json:"id"`
}

// GoogleBooksVolumeInfo represents volume info from Google Books
type GoogleBooksVolumeInfo struct {
	Title         string   `json:"title"`
	Authors       []string `json:"authors"`
	Publisher     string   `json:"publisher"`
	PublishedDate string   `json:"publishedDate"`
	Description   string   `json:"description"`
	ISBN          []struct {
		Identifier string `json:"identifier"`
		Type       string `json:"type"`
	} `json:"industryIdentifiers"`
	PageCount     int      `json:"pageCount"`
	Categories    []string `json:"categories"`
	Language      string   `json:"language"`
	AverageRating float64  `json:"averageRating"`
	ImageLinks    struct {
		SmallThumbnail string `json:"smallThumbnail"`
		Thumbnail      string `json:"thumbnail"`
	} `json:"imageLinks"`
}

// OpenLibraryResponse represents the Open Library API response
type OpenLibraryResponse struct {
	Title   string `json:"title"`
	Authors []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Publishers []struct {
		Name string `json:"name"`
	} `json:"publishers"`
	PublishDate   string   `json:"publish_date"`
	NumberOfPages int      `json:"number_of_pages"`
	ISBN10        []string `json:"isbn_10"`
	ISBN13        []string `json:"isbn_13"`
	Covers        []int    `json:"covers"`
}

// ListProvidersHandler returns available metadata providers
func ListProvidersHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	providers := []MetadataProvider{
		{ID: "google_books", Name: "Google Books"},
		{ID: "open_library", Name: "Open Library"},
	}

	jsonResponse(w, http.StatusOK, providers)
}

// SearchMetadataHandler searches for metadata from providers
func SearchMetadataHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	values := r.URL.Query()
	query := strings.TrimSpace(values.Get("q"))
	if query == "" {
		queryParts := []string{
			values.Get("title"),
			values.Get("author"),
			values.Get("isbn"),
			values.Get("series"),
			values.Get("publisher"),
		}
		query = strings.TrimSpace(strings.Join(queryParts, " "))
	}
	provider := r.URL.Query().Get("provider")

	if query == "" {
		errorResponse(w, http.StatusBadRequest, "Query parameter 'q' is required")
		return
	}

	var candidates []MetadataCandidate

	// Search from specified provider or all providers
	if provider == "" || provider == "google_books" {
		googleResults, err := searchGoogleBooks(query)
		if err == nil {
			candidates = append(candidates, googleResults...)
		}
	}

	if provider == "" || provider == "open_library" {
		openLibResults, err := searchOpenLibrary(query)
		if err == nil {
			candidates = append(candidates, openLibResults...)
		}
	}

	for i := range candidates {
		candidates[i].MatchScore = scoreMetadataCandidate(query, candidates[i])
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].MatchScore == candidates[j].MatchScore {
			if candidates[i].Title == candidates[j].Title {
				return candidates[i].Provider < candidates[j].Provider
			}
			return candidates[i].Title < candidates[j].Title
		}
		return candidates[i].MatchScore > candidates[j].MatchScore
	})

	if limitValue := strings.TrimSpace(values.Get("limit")); limitValue != "" {
		if limit, err := strconv.Atoi(limitValue); err == nil && limit > 0 && len(candidates) > limit {
			candidates = candidates[:limit]
		}
	}

	jsonResponse(w, http.StatusOK, candidates)
}

func normalizeMetadataSearchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	replacements := strings.NewReplacer(
		".", " ",
		",", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"-", " ",
		"_", " ",
		":", " ",
		";", " ",
		"/", " ",
		"\\", " ",
	)
	value = replacements.Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func scoreMetadataCandidate(query string, candidate MetadataCandidate) float64 {
	normalizedQuery := normalizeMetadataSearchText(query)
	if normalizedQuery == "" {
		return 0
	}

	score := 0.0
	title := normalizeMetadataSearchText(candidate.Title)
	authors := normalizeMetadataSearchText(strings.Join(candidate.Authors, " "))
	series := normalizeMetadataSearchText(candidate.Series)
	publisher := normalizeMetadataSearchText(candidate.Publisher)
	isbn := normalizeMetadataSearchText(candidate.ISBN)
	description := normalizeMetadataSearchText(candidate.Description)
	queryTokens := strings.Fields(normalizedQuery)

	if isbn != "" {
		switch {
		case strings.Contains(normalizedQuery, isbn):
			score += 90
		case isbn == normalizedQuery:
			score += 100
		}
	}

	if title != "" {
		switch {
		case title == normalizedQuery:
			score += 70
		case strings.Contains(title, normalizedQuery) || strings.Contains(normalizedQuery, title):
			score += 40
		}
	}

	if authors != "" {
		switch {
		case authors == normalizedQuery:
			score += 50
		case strings.Contains(authors, normalizedQuery) || strings.Contains(normalizedQuery, authors):
			score += 25
		}
	}

	if series != "" && strings.Contains(normalizedQuery, series) {
		score += 12
	}

	if publisher != "" && strings.Contains(normalizedQuery, publisher) {
		score += 8
	}

	if description != "" && strings.Contains(description, normalizedQuery) {
		score += 6
	}

	for _, token := range queryTokens {
		if len(token) < 3 {
			continue
		}
		if strings.Contains(title, token) {
			score += 6
		}
		if strings.Contains(authors, token) {
			score += 4
		}
		if strings.Contains(series, token) {
			score += 2
		}
		if strings.Contains(publisher, token) {
			score += 1.5
		}
		if strings.Contains(description, token) {
			score += 0.25
		}
	}

	return score
}

// ApplyMetadataHandler applies metadata to a book
func ApplyMetadataHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req struct {
		BookID   int64             `json:"book_id"`
		Metadata MetadataCandidate `json:"metadata"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := applyMetadataCandidateToBook(req.BookID, req.Metadata, true); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			errorResponse(w, http.StatusNotFound, "Book not found")
			return
		}
		errorResponse(w, http.StatusInternalServerError, "Failed to apply metadata")
		return
	}

	recordAppLog("info", "metadata", "Applied metadata manually", map[string]any{
		"book_id": req.BookID,
		"title":   req.Metadata.Title,
	})
	jsonResponse(w, http.StatusOK, map[string]string{"status": "applied"})
}

// searchGoogleBooks searches Google Books API
func searchGoogleBooks(query string) ([]MetadataCandidate, error) {
	apiURL := fmt.Sprintf("https://www.googleapis.com/books/v1/volumes?q=%s&maxResults=5",
		url.QueryEscape(query))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result GoogleBooksResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	candidates := []MetadataCandidate{}
	for _, item := range result.Items {
		candidate := MetadataCandidate{
			Provider:    "google_books",
			Title:       item.VolumeInfo.Title,
			Authors:     item.VolumeInfo.Authors,
			Series:      "", // Google Books doesn't provide series info
			Publisher:   item.VolumeInfo.Publisher,
			PubDate:     item.VolumeInfo.PublishedDate,
			Description: item.VolumeInfo.Description,
			PageCount:   item.VolumeInfo.PageCount,
			Language:    item.VolumeInfo.Language,
			Rating:      item.VolumeInfo.AverageRating,
			Genres:      item.VolumeInfo.Categories,
		}

		// Extract ISBN
		for _, id := range item.VolumeInfo.ISBN {
			if id.Type == "ISBN_13" {
				candidate.ISBN = id.Identifier
				break
			}
			if id.Type == "ISBN_10" && candidate.ISBN == "" {
				candidate.ISBN = id.Identifier
			}
		}

		// Get cover URL
		if item.VolumeInfo.ImageLinks.Thumbnail != "" {
			candidate.CoverURL = strings.Replace(item.VolumeInfo.ImageLinks.Thumbnail, "http:", "https:", 1)
		}

		candidates = append(candidates, candidate)
	}

	return candidates, nil
}

// searchOpenLibrary searches Open Library API
func searchOpenLibrary(query string) ([]MetadataCandidate, error) {
	apiURL := fmt.Sprintf("https://openlibrary.org/search.json?q=%s&limit=5",
		url.QueryEscape(query))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Docs []OpenLibraryResponse `json:"docs"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	candidates := []MetadataCandidate{}
	for _, doc := range result.Docs {
		candidate := MetadataCandidate{
			Provider:  "open_library",
			Title:     doc.Title,
			PubDate:   doc.PublishDate,
			PageCount: doc.NumberOfPages,
		}

		// Extract authors
		for _, author := range doc.Authors {
			candidate.Authors = append(candidate.Authors, author.Name)
		}

		// Extract publisher
		if len(doc.Publishers) > 0 {
			candidate.Publisher = doc.Publishers[0].Name
		}

		// Extract ISBN
		if len(doc.ISBN13) > 0 {
			candidate.ISBN = doc.ISBN13[0]
		} else if len(doc.ISBN10) > 0 {
			candidate.ISBN = doc.ISBN10[0]
		}

		// Get cover URL
		if len(doc.Covers) > 0 {
			candidate.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", doc.Covers[0])
		}

		candidates = append(candidates, candidate)
	}

	return candidates, nil
}

// downloadCover downloads a cover image for a book
func downloadCover(bookID int64, coverURL string) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(coverURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	settings := covers.LoadSettings(appDB.DB)
	processed, err := covers.ProcessCover(data, settings)
	if err != nil || len(processed) == 0 {
		processed = data
	}

	// Save cover
	coverPath, err := covers.SaveCoverBytes(appConfig.GetCoversPath(), bookID, processed)
	if err != nil {
		return
	}

	var previousPath string
	_ = appDB.QueryRow("SELECT COALESCE(cover_path, '') FROM book_metadata WHERE book_id = ?", bookID).Scan(&previousPath)

	// Update book_metadata with cover path
	_, _ = appDB.Exec(`
		UPDATE book_metadata
		SET cover_path = ?, cover_updated_on = ?
		WHERE book_id = ?
	`, coverPath, time.Now().Unix(), bookID)

	if previousPath != "" && previousPath != coverPath {
		_ = os.Remove(previousPath)
	}
}

// LockMetadataFieldHandler locks a metadata field
func LockMetadataFieldHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req struct {
		BookID int64    `json:"book_id"`
		Fields []string `json:"fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	fieldsJSON, _ := json.Marshal(req.Fields)

	_, err := appDB.Exec(`
		UPDATE book_metadata SET locked_fields = ? WHERE book_id = ?
	`, string(fieldsJSON), req.BookID)

	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to lock fields")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "locked"})
}

// UnlockMetadataFieldHandler unlocks a metadata field
func UnlockMetadataFieldHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req struct {
		BookID int64    `json:"book_id"`
		Fields []string `json:"fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Get current locked fields
	var lockedFieldsJSON string
	err := appDB.QueryRow("SELECT locked_fields FROM book_metadata WHERE book_id = ?", req.BookID).Scan(&lockedFieldsJSON)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "Book metadata not found")
		return
	}

	var locked []string
	json.Unmarshal([]byte(lockedFieldsJSON), &locked)

	// Remove specified fields from locked list
	newLocked := []string{}
	for _, f := range locked {
		if !contains(req.Fields, f) {
			newLocked = append(newLocked, f)
		}
	}

	newLockedJSON, _ := json.Marshal(newLocked)

	_, err = appDB.Exec(`
		UPDATE book_metadata SET locked_fields = ? WHERE book_id = ?
	`, string(newLockedJSON), req.BookID)

	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to unlock fields")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "unlocked"})
}

// TriggerScanHandler triggers a library scan
func TriggerScanHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageLibraries) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	go func() {
		for _, lib := range appConfig.Libraries {
			var libraryID int64
			appDB.QueryRow("SELECT id FROM library WHERE name = ? AND owner_user_id = ?", lib.Name, 1).Scan(&libraryID)
			if libraryID > 0 {
				appScanner.ScanLibrary(libraryID, lib.Paths)
			}
		}
	}()

	jsonResponse(w, http.StatusOK, map[string]string{"status": "scanning"})
}

// RebuildFTSHandler rebuilds the FTS index for search
func RebuildFTSHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageLibraries) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	go func() {
		if err := appScanner.RebuildFTS(); err != nil {
			slog.Error("Failed to rebuild FTS", "error", err)
		}
	}()

	jsonResponse(w, http.StatusOK, map[string]string{"status": "rebuilding"})
}

// Helper functions
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func saveFile(path string, data []byte) error {
	// Implementation would use os.WriteFile
	// For now, just return nil
	return nil
}
