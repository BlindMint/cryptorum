package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"cryptorum/internal/covers"
	"cryptorum/internal/metadata"
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
	ASIN        string   `json:"asin,omitempty"`
	CoverURL    string   `json:"cover_url,omitempty"`
	PageCount   int      `json:"page_count,omitempty"`
	Language    string   `json:"language,omitempty"`
	Rating      float64  `json:"rating,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	MatchScore  float64  `json:"match_score,omitempty"`
}

type MetadataSearchFields struct {
	Query     string
	Title     string
	Author    string
	ISBN      string
	ASIN      string
	Series    string
	Publisher string
	Provider  string
	Strict    bool
}

type metadataSearchVariant struct {
	Fields MetadataSearchFields
	Query  string
	Bonus  float64
}

type metadataSearchCacheEntry struct {
	Candidates []MetadataCandidate
	ExpiresAt  time.Time
}

var metadataSearchCache = struct {
	sync.Mutex
	entries map[string]metadataSearchCacheEntry
}{
	entries: map[string]metadataSearchCacheEntry{},
}

const metadataSearchCacheTTL = 10 * time.Minute

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
	Amazon        []string `json:"amazon"`
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
		{ID: "bookbrainz", Name: "BookBrainz"},
		{ID: "library_of_congress", Name: "Library of Congress"},
		{ID: "wikidata", Name: "Wikidata"},
		{ID: "internet_archive", Name: "Internet Archive"},
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
	fields := MetadataSearchFields{
		Query:     strings.TrimSpace(values.Get("q")),
		Title:     strings.TrimSpace(values.Get("title")),
		Author:    strings.TrimSpace(values.Get("author")),
		ISBN:      strings.TrimSpace(values.Get("isbn")),
		ASIN:      strings.TrimSpace(values.Get("asin")),
		Series:    strings.TrimSpace(values.Get("series")),
		Publisher: strings.TrimSpace(values.Get("publisher")),
		Provider:  strings.TrimSpace(values.Get("provider")),
		Strict:    parseBoolQuery(values.Get("strict")),
	}
	query := metadataFieldsQuery(fields)

	if query == "" {
		errorResponse(w, http.StatusBadRequest, "Query parameter 'q' is required")
		return
	}

	candidates := searchMetadataCandidates(fields)
	if candidates == nil {
		candidates = []MetadataCandidate{}
	}

	if limitValue := strings.TrimSpace(values.Get("limit")); limitValue != "" {
		if limit, err := strconv.Atoi(limitValue); err == nil && limit > 0 && len(candidates) > limit {
			candidates = candidates[:limit]
		}
	}

	jsonResponse(w, http.StatusOK, candidates)
}

func metadataFieldsQuery(fields MetadataSearchFields) string {
	if strings.TrimSpace(fields.Query) != "" {
		return strings.TrimSpace(fields.Query)
	}
	queryParts := []string{
		fields.Title,
		fields.Author,
		fields.ISBN,
		fields.ASIN,
		fields.Series,
		fields.Publisher,
	}
	return strings.TrimSpace(strings.Join(queryParts, " "))
}

func searchMetadataCandidates(fields MetadataSearchFields) []MetadataCandidate {
	variants := metadataSearchVariants(fields)
	if len(variants) == 0 {
		return nil
	}

	bestByKey := map[string]MetadataCandidate{}
	resultsByQuery := map[string][]MetadataCandidate{}
	for _, variant := range variants {
		results, ok := resultsByQuery[variant.Query]
		if !ok {
			results = fetchMetadataCandidates(variant.Query, fields.Provider)
			resultsByQuery[variant.Query] = results
		}
		for _, candidate := range results {
			candidate.MatchScore = scoreMetadataCandidateForFields(variant.Fields, candidate) + variant.Bonus + providerMatchBonus(candidate.Provider)
			key := metadataCandidateKey(candidate)
			existing, exists := bestByKey[key]
			if !exists || candidate.MatchScore > existing.MatchScore {
				bestByKey[key] = candidate
			}
		}
	}

	var candidates []MetadataCandidate
	for _, candidate := range bestByKey {
		candidates = append(candidates, candidate)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].MatchScore == candidates[j].MatchScore {
			leftTitle := normalizeMetadataSearchText(candidates[i].Title)
			rightTitle := normalizeMetadataSearchText(candidates[j].Title)
			if leftTitle != rightTitle {
				return leftTitle < rightTitle
			}
			leftAuthors := normalizeMetadataSearchText(strings.Join(candidates[i].Authors, " "))
			rightAuthors := normalizeMetadataSearchText(strings.Join(candidates[j].Authors, " "))
			if leftAuthors != rightAuthors {
				return leftAuthors < rightAuthors
			}
			if candidates[i].Provider != candidates[j].Provider {
				return candidates[i].Provider < candidates[j].Provider
			}
			return normalizeISBN(candidates[i].ISBN) < normalizeISBN(candidates[j].ISBN)
		}
		return candidates[i].MatchScore > candidates[j].MatchScore
	})

	return candidates
}

func metadataSearchVariants(fields MetadataSearchFields) []metadataSearchVariant {
	baseQuery := metadataFieldsQuery(fields)
	if baseQuery == "" {
		return nil
	}

	variants := []metadataSearchVariant{{
		Fields: fields,
		Query:  baseQuery,
	}}

	if strings.TrimSpace(fields.Query) != "" {
		return variants
	}
	if fields.Strict {
		return variants
	}

	title := strings.TrimSpace(fields.Title)
	author := strings.TrimSpace(fields.Author)
	if title != "" && author != "" {
		swappedFields := MetadataSearchFields{
			Title:     author,
			Author:    title,
			ISBN:      fields.ISBN,
			ASIN:      fields.ASIN,
			Series:    fields.Series,
			Publisher: fields.Publisher,
		}
		swappedQuery := metadataFieldsQuery(swappedFields)
		if swappedQuery != "" && normalizeMetadataSearchText(swappedQuery) != normalizeMetadataSearchText(baseQuery) {
			variants = append(variants, metadataSearchVariant{
				Fields: swappedFields,
				Query:  swappedQuery,
				Bonus:  8,
			})
		}
		return variants
	}

	if title != "" && author == "" {
		authorOnlyFields := MetadataSearchFields{
			Author:    title,
			ISBN:      fields.ISBN,
			ASIN:      fields.ASIN,
			Series:    fields.Series,
			Publisher: fields.Publisher,
		}
		if query := metadataFieldsQuery(authorOnlyFields); query != "" {
			variants = append(variants, metadataSearchVariant{
				Fields: authorOnlyFields,
				Query:  query,
				Bonus:  5,
			})
		}
	}

	if author != "" && title == "" {
		titleOnlyFields := MetadataSearchFields{
			Title:     author,
			ISBN:      fields.ISBN,
			ASIN:      fields.ASIN,
			Series:    fields.Series,
			Publisher: fields.Publisher,
		}
		if query := metadataFieldsQuery(titleOnlyFields); query != "" {
			variants = append(variants, metadataSearchVariant{
				Fields: titleOnlyFields,
				Query:  query,
				Bonus:  5,
			})
		}
	}

	return variants
}

func parseBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func fetchMetadataCandidates(query, provider string) []MetadataCandidate {
	cacheKey := metadataSearchCacheKey(query, provider)
	if cached, ok := getCachedMetadataCandidates(cacheKey); ok {
		return cached
	}

	candidates, cacheable := fetchMetadataCandidatesUncached(query, provider)
	if cacheable {
		setCachedMetadataCandidates(cacheKey, candidates)
	}
	return candidates
}

func fetchMetadataCandidatesUncached(query, provider string) ([]MetadataCandidate, bool) {
	var candidates []MetadataCandidate
	successfulRequests := 0

	// Search from specified provider or all providers
	if provider == "" || provider == "google_books" {
		googleResults, err := searchGoogleBooks(query)
		if err == nil {
			successfulRequests++
			candidates = append(candidates, googleResults...)
		}
	}

	if provider == "" || provider == "open_library" {
		openLibResults, err := searchOpenLibrary(query)
		if err == nil {
			successfulRequests++
			candidates = append(candidates, openLibResults...)
		}
	}

	if provider == "" || provider == "bookbrainz" {
		bookBrainzResults, err := searchBookBrainz(query)
		if err == nil {
			successfulRequests++
			candidates = append(candidates, bookBrainzResults...)
		}
	}

	if provider == "" || provider == "library_of_congress" {
		locResults, err := searchLibraryOfCongress(query)
		if err == nil {
			successfulRequests++
			candidates = append(candidates, locResults...)
		}
	}

	if provider == "" || provider == "wikidata" {
		wikidataResults, err := searchWikidata(query)
		if err == nil {
			successfulRequests++
			candidates = append(candidates, wikidataResults...)
		}
	}

	if provider == "" || provider == "internet_archive" {
		archiveResults, err := searchInternetArchive(query)
		if err == nil {
			successfulRequests++
			candidates = append(candidates, archiveResults...)
		}
	}

	return candidates, successfulRequests > 0
}

func metadataSearchCacheKey(query, provider string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "|" + normalizeMetadataSearchText(query)
}

func getCachedMetadataCandidates(key string) ([]MetadataCandidate, bool) {
	metadataSearchCache.Lock()
	defer metadataSearchCache.Unlock()

	entry, ok := metadataSearchCache.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.ExpiresAt) {
		delete(metadataSearchCache.entries, key)
		return nil, false
	}
	return cloneMetadataCandidates(entry.Candidates), true
}

func setCachedMetadataCandidates(key string, candidates []MetadataCandidate) {
	metadataSearchCache.Lock()
	defer metadataSearchCache.Unlock()

	metadataSearchCache.entries[key] = metadataSearchCacheEntry{
		Candidates: cloneMetadataCandidates(candidates),
		ExpiresAt:  time.Now().Add(metadataSearchCacheTTL),
	}
}

func cloneMetadataCandidates(candidates []MetadataCandidate) []MetadataCandidate {
	if candidates == nil {
		return nil
	}
	cloned := make([]MetadataCandidate, len(candidates))
	for i, candidate := range candidates {
		cloned[i] = candidate
		cloned[i].Authors = append([]string(nil), candidate.Authors...)
		cloned[i].Genres = append([]string(nil), candidate.Genres...)
	}
	return cloned
}

func metadataCandidateKey(candidate MetadataCandidate) string {
	parts := []string{
		candidate.Provider,
		normalizeMetadataSearchText(candidate.Title),
		normalizeMetadataSearchText(strings.Join(candidate.Authors, " ")),
		normalizeMetadataSearchText(candidate.ISBN),
		normalizeMetadataSearchText(candidate.ASIN),
	}
	return strings.Join(parts, "|")
}

func providerMatchBonus(provider string) float64 {
	switch provider {
	case "google_books":
		return 6
	case "open_library":
		return 5
	case "bookbrainz":
		return 4.5
	case "library_of_congress":
		return 3.5
	case "wikidata":
		return 1.25
	case "internet_archive":
		return 1
	default:
		return 0
	}
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
	asin := normalizeMetadataSearchText(candidate.ASIN)
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
	if asin != "" {
		switch {
		case strings.Contains(normalizedQuery, asin):
			score += 65
		case asin == normalizedQuery:
			score += 75
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

func scoreMetadataCandidateForFields(fields MetadataSearchFields, candidate MetadataCandidate) float64 {
	query := metadataFieldsQuery(fields)
	if strings.TrimSpace(fields.Query) != "" {
		return scoreMetadataCandidate(query, candidate)
	}

	score := 0.0
	titleScore, titleQuality, titleConflict := scoreIdentityField(fields.Title, candidate.Title, 82, 48, 34, 45)
	authorScore, authorQuality, authorConflict := scoreIdentityField(fields.Author, strings.Join(candidate.Authors, " "), 68, 40, 28, 40)
	seriesScore, _, seriesConflict := scoreIdentityField(fields.Series, candidate.Series, 14, 9, 6, 6)
	publisherScore, _, _ := scoreIdentityField(fields.Publisher, candidate.Publisher, 10, 6, 4, 0)
	score += titleScore + authorScore + seriesScore + publisherScore

	isbnScore, isbnQuality, isbnConflict := scoreISBNMatch(fields.ISBN, candidate.ISBN)
	score += isbnScore
	asinScore, asinQuality, asinConflict := scoreASINMatch(fields.ASIN, candidate.ASIN)
	score += asinScore

	if titleQuality >= 0.92 && authorQuality >= 0.92 {
		score += 38
	} else if titleQuality >= 0.65 && authorQuality >= 0.65 {
		score += 22
	}

	if isbnQuality >= 1 {
		switch {
		case titleQuality >= 0.65 && authorQuality >= 0.65:
			score += 42
		case titleConflict && authorConflict:
			score -= 95
		case titleConflict || authorConflict:
			score -= 38
		}
	}

	if isbnConflict {
		if titleQuality >= 0.85 && authorQuality >= 0.85 {
			score -= 25
		} else if titleQuality >= 0.65 && authorQuality >= 0.65 {
			score -= 40
		} else {
			score -= 70
		}
	}
	if asinQuality >= 1 && titleQuality >= 0.65 && authorQuality >= 0.65 {
		score += 28
	}
	if asinConflict {
		if titleQuality >= 0.85 && authorQuality >= 0.85 {
			score -= 18
		} else if titleQuality >= 0.65 && authorQuality >= 0.65 {
			score -= 28
		} else {
			score -= 52
		}
	}

	if titleConflict && authorConflict {
		score -= 80
	} else {
		if titleConflict {
			score -= 35
		}
		if authorConflict {
			score -= 32
		}
	}

	if seriesConflict && titleQuality < 0.65 {
		score -= 10
	}

	score += scoreDescriptionTokenOverlap(query, candidate.Description)
	return score
}

func scoreIdentityField(queryValue, candidateValue string, exactScore, containsScore, tokenScore, conflictPenalty float64) (float64, float64, bool) {
	query := normalizeMetadataSearchText(queryValue)
	candidate := normalizeMetadataSearchText(candidateValue)
	if query == "" || candidate == "" {
		return 0, 0, false
	}

	switch {
	case query == candidate:
		return exactScore, 1, false
	case strings.Contains(candidate, query) || strings.Contains(query, candidate):
		return containsScore, 0.78, false
	}

	quality := tokenOverlapQuality(query, candidate)
	if quality > 0 {
		return tokenScore * quality, quality, false
	}

	conflict := conflictPenalty > 0 && len(significantTokens(query)) > 0 && len(significantTokens(candidate)) > 0
	return -conflictPenalty, 0, conflict
}

func scoreISBNMatch(queryValue, candidateValue string) (float64, float64, bool) {
	query := normalizeISBN(queryValue)
	candidate := normalizeISBN(candidateValue)
	if query == "" || candidate == "" {
		return 0, 0, false
	}
	if query == candidate {
		return 130, 1, false
	}
	if len(query) >= 10 && len(candidate) >= 10 && (strings.Contains(query, candidate) || strings.Contains(candidate, query)) {
		return 82, 0.72, false
	}
	return -65, 0, true
}

func scoreASINMatch(queryValue, candidateValue string) (float64, float64, bool) {
	query := normalizeASIN(queryValue)
	candidate := normalizeASIN(candidateValue)
	if query == "" || candidate == "" {
		return 0, 0, false
	}
	if query == candidate {
		return 88, 1, false
	}
	return -48, 0, true
}

func normalizeASIN(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func normalizeISBN(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range value {
		if (r >= '0' && r <= '9') || r == 'X' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func significantTokens(value string) []string {
	tokens := strings.Fields(value)
	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if len(token) >= 3 {
			result = append(result, token)
		}
	}
	return result
}

func tokenOverlapQuality(query, candidate string) float64 {
	queryTokens := significantTokens(query)
	if len(queryTokens) == 0 {
		return 0
	}
	candidateTokens := significantTokens(candidate)
	if len(candidateTokens) == 0 {
		return 0
	}

	matches := 0
	for _, queryToken := range queryTokens {
		for _, candidateToken := range candidateTokens {
			if queryToken == candidateToken || strings.Contains(candidateToken, queryToken) || strings.Contains(queryToken, candidateToken) {
				matches++
				break
			}
		}
	}
	return float64(matches) / float64(len(queryTokens))
}

func scoreDescriptionTokenOverlap(query, description string) float64 {
	normalizedDescription := normalizeMetadataSearchText(description)
	if normalizedDescription == "" {
		return 0
	}
	score := 0.0
	for _, token := range significantTokens(normalizeMetadataSearchText(query)) {
		if strings.Contains(normalizedDescription, token) {
			score += 0.25
		}
	}
	if score > 6 {
		return 6
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("google books returned status %d", resp.StatusCode)
	}

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
			} else if id.Type == "ISBN_10" && candidate.ISBN == "" {
				candidate.ISBN = id.Identifier
			}
			if strings.Contains(strings.ToLower(id.Type), "asin") && candidate.ASIN == "" {
				candidate.ASIN = normalizeASIN(id.Identifier)
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("open library returned status %d", resp.StatusCode)
	}

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
		if len(doc.Amazon) > 0 {
			candidate.ASIN = normalizeASIN(doc.Amazon[0])
		}

		// Get cover URL
		if len(doc.Covers) > 0 {
			candidate.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", doc.Covers[0])
		}

		candidates = append(candidates, candidate)
	}

	return candidates, nil
}

func searchBookBrainz(query string) ([]MetadataCandidate, error) {
	apiURL := fmt.Sprintf("https://bookbrainz.org/search?from=0&q=%s&size=8", url.QueryEscape(query))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("bookbrainz returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	lines := htmlToLines(string(body))
	candidates := []MetadataCandidate{}
	seen := map[string]struct{}{}
	for _, line := range lines {
		cols := splitTableColumns(line)
		if len(cols) < 2 {
			continue
		}
		entityType := strings.ToLower(strings.TrimSpace(cols[0]))
		if !isBookBrainzEntityType(entityType) {
			continue
		}

		title, authors := parseBookBrainzSearchColumns(entityType, cols)
		if title == "" && len(authors) == 0 {
			continue
		}

		candidate := MetadataCandidate{
			Provider: "bookbrainz",
			Title:    title,
			Authors:  authors,
		}
		key := metadataCandidateKey(candidate)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, candidate)
	}

	return candidates, nil
}

func searchLibraryOfCongress(query string) ([]MetadataCandidate, error) {
	apiURL := fmt.Sprintf("https://www.loc.gov/search/?fo=json&c=8&q=%s", url.QueryEscape(query))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("library of congress returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	candidates := []MetadataCandidate{}
	for _, item := range result.Results {
		candidate := MetadataCandidate{
			Provider:    "library_of_congress",
			Title:       firstStringFromAny(item["title"]),
			Description: firstStringFromAny(item["description"]),
			Publisher:   firstStringFromAny(item["publisher"]),
			PubDate:     firstStringFromAny(item["date"]),
			CoverURL:    firstStringFromAny(item["image_url"]),
		}
		candidate.Authors = firstStringSliceFromAny(item["creator"])
		if len(candidate.Authors) == 0 {
			candidate.Authors = firstStringSliceFromAny(item["contributor"])
		}
		candidate.ISBN = firstStringFromAny(item["isbn"])
		if candidate.Title != "" {
			candidates = append(candidates, candidate)
		}
	}

	return candidates, nil
}

func searchWikidata(query string) ([]MetadataCandidate, error) {
	apiURL := fmt.Sprintf("https://www.wikidata.org/w/api.php?action=wbsearchentities&search=%s&language=en&limit=8&format=json&type=item", url.QueryEscape(query))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("wikidata returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var search struct {
		Search []struct {
			ID          string `json:"id"`
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"search"`
	}
	if err := json.Unmarshal(body, &search); err != nil {
		return nil, err
	}

	candidates := []MetadataCandidate{}
	for _, item := range search.Search {
		if item.ID == "" {
			continue
		}
		entityURL := fmt.Sprintf("https://www.wikidata.org/wiki/Special:EntityData/%s.json", url.PathEscape(item.ID))
		entityResp, err := client.Get(entityURL)
		if err != nil {
			continue
		}
		if entityResp.StatusCode < 200 || entityResp.StatusCode >= 300 {
			entityResp.Body.Close()
			continue
		}
		entityBody, err := io.ReadAll(entityResp.Body)
		entityResp.Body.Close()
		if err != nil {
			continue
		}

		var entityData struct {
			Entities map[string]struct {
				Labels map[string]struct {
					Value string `json:"value"`
				} `json:"labels"`
				Descriptions map[string]struct {
					Value string `json:"value"`
				} `json:"descriptions"`
				Claims map[string][]struct {
					Mainsnak struct {
						Datavalue struct {
							Value any `json:"value"`
						} `json:"datavalue"`
					} `json:"mainsnak"`
				} `json:"claims"`
			} `json:"entities"`
		}
		if err := json.Unmarshal(entityBody, &entityData); err != nil {
			continue
		}

		entity := entityData.Entities[item.ID]
		candidate := MetadataCandidate{
			Provider:    "wikidata",
			Title:       firstEntityLabel(entity.Labels),
			Description: firstEntityDescription(entity.Descriptions),
		}
		if candidate.Title == "" {
			candidate.Title = item.Label
		}
		extractWikidataIdentifiers(&candidate, entity.Claims)
		if candidate.Title != "" {
			candidates = append(candidates, candidate)
		}
	}

	return candidates, nil
}

func searchInternetArchive(query string) ([]MetadataCandidate, error) {
	apiURL := fmt.Sprintf("https://archive.org/advancedsearch.php?q=%s&fl[]=identifier&fl[]=title&fl[]=creator&fl[]=publisher&fl[]=date&fl[]=isbn&fl[]=language&fl[]=description&rows=8&page=1&output=json", url.QueryEscape(query))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("internet archive returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Response struct {
			Docs []map[string]any `json:"docs"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	candidates := []MetadataCandidate{}
	for _, doc := range result.Response.Docs {
		candidate := MetadataCandidate{
			Provider:    "internet_archive",
			Title:       firstStringFromAny(doc["title"]),
			Publisher:   firstStringFromAny(doc["publisher"]),
			PubDate:     firstStringFromAny(doc["date"]),
			Description: firstStringFromAny(doc["description"]),
		}
		candidate.Authors = firstStringSliceFromAny(doc["creator"])
		if len(candidate.Authors) == 0 {
			candidate.Authors = firstStringSliceFromAny(doc["author"])
		}
		candidate.ISBN = firstStringFromAny(doc["isbn"])
		candidate.ASIN = firstStringFromAny(doc["asin"])
		if candidate.ASIN != "" {
			candidate.ASIN = normalizeASIN(candidate.ASIN)
		}
		if identifier := firstStringFromAny(doc["identifier"]); identifier != "" {
			candidate.CoverURL = fmt.Sprintf("https://archive.org/services/img/%s", url.PathEscape(identifier))
		}
		if candidate.Title != "" {
			candidates = append(candidates, candidate)
		}
	}

	return candidates, nil
}

func htmlToLines(raw string) []string {
	replacements := []struct {
		pattern *regexp.Regexp
		value   string
	}{
		{regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`), " "},
		{regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`), " "},
		{regexp.MustCompile(`(?i)</(p|div|li|tr|h[1-6]|dt|dd|br|table|section|article|header|footer|ul|ol)>`), "\n"},
		{regexp.MustCompile(`(?i)<br\s*/?>`), "\n"},
		{regexp.MustCompile(`(?s)<[^>]+>`), " "},
	}
	for _, replacement := range replacements {
		raw = replacement.pattern.ReplaceAllString(raw, replacement.value)
	}
	raw = html.UnescapeString(raw)
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return cleaned
}

func splitTableColumns(line string) []string {
	parts := strings.Split(line, "|")
	cols := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			cols = append(cols, trimmed)
		}
	}
	return cols
}

func isBookBrainzEntityType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "edition", "work", "edition group", "author", "series", "publisher":
		return true
	default:
		return false
	}
}

func parseBookBrainzSearchColumns(entityType string, cols []string) (string, []string) {
	name := strings.TrimSpace(cols[1])
	aliases := ""
	if len(cols) > 2 {
		aliases = strings.TrimSpace(cols[2])
	}

	title := name
	var authors []string
	if strings.EqualFold(entityType, "author") {
		authors = []string{name}
		return title, authors
	}

	if titlePart, authorPart, ok := splitTitleAndAuthor(name); ok {
		title = titlePart
		authors = splitCandidateAuthors(authorPart)
	}
	if len(authors) == 0 && aliases != "" {
		if _, aliasAuthors, ok := splitTitleAndAuthor(aliases); ok && len(aliasAuthors) > 0 {
			authors = splitCandidateAuthors(aliasAuthors)
		}
	}

	return title, authors
}

func splitTitleAndAuthor(value string) (string, string, bool) {
	separators := []string{" — ", " - ", " | "}
	for _, sep := range separators {
		if parts := strings.SplitN(value, sep, 2); len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			if left != "" && right != "" {
				return left, right, true
			}
		}
	}
	return "", "", false
}

func splitCandidateAuthors(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '&'
	})
	authors := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			authors = append(authors, trimmed)
		}
	}
	return authors
}

func firstStringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		for _, item := range v {
			if s := firstStringFromAny(item); s != "" {
				return s
			}
		}
	case []string:
		for _, item := range v {
			if s := strings.TrimSpace(item); s != "" {
				return s
			}
		}
	}
	return ""
}

func firstStringSliceFromAny(value any) []string {
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil
		}
		return splitCandidateAuthors(v)
	case []any:
		var result []string
		for _, item := range v {
			if s := firstStringFromAny(item); s != "" {
				result = append(result, s)
			}
		}
		return result
	case []string:
		var result []string
		for _, item := range v {
			if s := strings.TrimSpace(item); s != "" {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func firstEntityLabel(labels map[string]struct {
	Value string `json:"value"`
}) string {
	if label, ok := labels["en"]; ok && strings.TrimSpace(label.Value) != "" {
		return strings.TrimSpace(label.Value)
	}
	for _, label := range labels {
		if strings.TrimSpace(label.Value) != "" {
			return strings.TrimSpace(label.Value)
		}
	}
	return ""
}

func firstEntityDescription(descriptions map[string]struct {
	Value string `json:"value"`
}) string {
	if description, ok := descriptions["en"]; ok && strings.TrimSpace(description.Value) != "" {
		return strings.TrimSpace(description.Value)
	}
	for _, description := range descriptions {
		if strings.TrimSpace(description.Value) != "" {
			return strings.TrimSpace(description.Value)
		}
	}
	return ""
}

func extractWikidataIdentifiers(candidate *MetadataCandidate, claims map[string][]struct {
	Mainsnak struct {
		Datavalue struct {
			Value any `json:"value"`
		} `json:"datavalue"`
	} `json:"mainsnak"`
}) {
	for claimID, claimList := range claims {
		for _, claim := range claimList {
			value := firstStringFromAny(claim.Mainsnak.Datavalue.Value)
			if value == "" {
				continue
			}
			switch claimID {
			case "P212", "P957":
				if candidate.ISBN == "" {
					candidate.ISBN = value
				}
			case "P5745":
				if candidate.ASIN == "" {
					candidate.ASIN = normalizeASIN(value)
				}
			case "P18":
				if candidate.CoverURL == "" {
					candidate.CoverURL = fmt.Sprintf("https://commons.wikimedia.org/wiki/Special:FilePath/%s", url.PathEscape(value))
				}
			}
		}
	}
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

// RegenerateBookCoverHandler re-extracts and regenerates a cover from the book's source file.
func RegenerateBookCoverHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageMetadata) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	bookID := chi.URLParam(r, "bookID")
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

	var filePath string
	if err := appDB.QueryRow(`SELECT path FROM book_file WHERE book_id = ? LIMIT 1`, bookIDInt).Scan(&filePath); err != nil {
		errorResponse(w, http.StatusNotFound, "Book file not found")
		return
	}

	filePath = translateHostPathToContainerPath(filePath)

	meta, err := metadata.Extract(filePath)
	if err != nil || meta == nil || len(meta.CoverData) == 0 {
		errorResponse(w, http.StatusNotFound, "No cover could be extracted from the source file")
		return
	}

	settings := covers.LoadSettings(appDB.DB)
	processed, err := covers.ProcessCover(meta.CoverData, settings)
	if err != nil || len(processed) == 0 {
		processed = meta.CoverData
	}

	coverPath, err := covers.SaveCoverBytes(appConfig.GetCoversPath(), bookIDInt, processed)
	if err != nil || coverPath == "" {
		slog.Error("Failed to save regenerated cover", "bookID", bookIDInt, "error", err)
		errorResponse(w, http.StatusInternalServerError, "Failed to regenerate cover")
		return
	}

	var previousPath string
	_ = appDB.QueryRow(`SELECT COALESCE(cover_path, '') FROM book_metadata WHERE book_id = ?`, bookIDInt).Scan(&previousPath)

	_, err = appDB.Exec(`
		INSERT INTO book_metadata (book_id, cover_path, cover_updated_on, authors, genres, locked_fields)
		VALUES (?, ?, ?, '[]', '[]', '[]')
		ON CONFLICT(book_id) DO UPDATE SET
			cover_path = excluded.cover_path,
			cover_updated_on = excluded.cover_updated_on
	`, bookIDInt, coverPath, time.Now().Unix())
	if err != nil {
		slog.Error("Failed to update regenerated cover path", "bookID", bookIDInt, "error", err)
		errorResponse(w, http.StatusInternalServerError, "Failed to update cover metadata")
		return
	}

	if previousPath != "" && previousPath != coverPath {
		_ = os.Remove(previousPath)
	}

	recordAppLog("info", "covers", "Regenerated book cover", map[string]any{
		"book_id": bookIDInt,
	})
	jsonResponse(w, http.StatusOK, map[string]string{"status": "regenerated"})
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
