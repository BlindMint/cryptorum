package main

import (
	"archive/zip"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ledongthuc/pdf"
)

// GetStatsHandler returns library and reading statistics
func GetStatsHandler(w http.ResponseWriter, r *http.Request) {
	type FormatCounts struct {
		EPUB  int64 `json:"epub"`
		PDF   int64 `json:"pdf"`
		CBX   int64 `json:"cbx"`
		Audio int64 `json:"audio"`
		Other int64 `json:"other"`
	}

	type ActivityDay struct {
		Date     string `json:"date"`
		Sessions int    `json:"sessions"`
		Minutes  int    `json:"minutes"`
	}

	type GenreCount struct {
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}

	type AuthorCount struct {
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}

	type RatingCount struct {
		Rating float64 `json:"rating"`
		Count  int64   `json:"count"`
	}

	type YearCount struct {
		Year  int   `json:"year"`
		Count int64 `json:"count"`
	}

	type ReadingProgress struct {
		Date  string `json:"date"`
		Pages int64  `json:"pages"`
		Books int64  `json:"books"`
	}

	type CountItem struct {
		Label string `json:"label"`
		Count int64  `json:"count"`
	}

	type StatsResponse struct {
		TotalBooks            int64             `json:"total_books"`
		TotalPages            int64             `json:"total_pages"`
		TotalSeries           int64             `json:"total_series"`
		TotalAuthors          int64             `json:"total_authors"`
		TotalGenres           int64             `json:"total_genres"`
		TotalLanguages        int64             `json:"total_languages"`
		LibraryFirstAddedAt   int64             `json:"library_first_added_at"`
		LibraryLatestAddedAt  int64             `json:"library_latest_added_at"`
		Reading               int64             `json:"reading"`
		Finished              int64             `json:"finished"`
		Unread                int64             `json:"unread"`
		SessionsThisWeek      int64             `json:"sessions_this_week"`
		TotalSessionMinutes   int64             `json:"total_session_minutes"`
		AverageSessionMinutes float64           `json:"average_session_minutes"`
		CurrentReadingStreak  int64             `json:"current_reading_streak"`
		BooksByFormat         FormatCounts      `json:"books_by_format"`
		ReadingActivity       []ActivityDay     `json:"reading_activity"`
		GenreDistribution     []GenreCount      `json:"genre_distribution"`
		AuthorDistribution    []AuthorCount     `json:"author_distribution"`
		LanguageDistribution  []CountItem       `json:"language_distribution"`
		RatingDistribution    []RatingCount     `json:"rating_distribution"`
		PubYearTimeline       []YearCount       `json:"pub_year_timeline"`
		ReadingProgress       []ReadingProgress `json:"reading_progress"`
		PageCountBuckets      []CountItem       `json:"page_count_buckets"`
		PageCountMissing      int64             `json:"page_count_missing"`
		SessionBuckets        []CountItem       `json:"session_buckets"`
	}

	var stats StatsResponse

	appDB.QueryRow("SELECT COUNT(*) FROM book").Scan(&stats.TotalBooks)
	appDB.QueryRow("SELECT COALESCE(SUM(COALESCE(page_count, 0)), 0) FROM book_metadata").Scan(&stats.TotalPages)
	appDB.QueryRow("SELECT COUNT(DISTINCT series) FROM book_metadata WHERE series IS NOT NULL AND series != ''").Scan(&stats.TotalSeries)
	appDB.QueryRow("SELECT COUNT(DISTINCT language) FROM book_metadata WHERE language IS NOT NULL AND language != ''").Scan(&stats.TotalLanguages)
	appDB.QueryRow("SELECT COALESCE(MIN(added_at), 0), COALESCE(MAX(added_at), 0) FROM book").Scan(&stats.LibraryFirstAddedAt, &stats.LibraryLatestAddedAt)
	appDB.QueryRow("SELECT COUNT(*) FROM reading_progress WHERE status = 'reading'").Scan(&stats.Reading)
	appDB.QueryRow("SELECT COUNT(*) FROM reading_progress WHERE status = 'finished'").Scan(&stats.Finished)
	stats.Unread = stats.TotalBooks - stats.Reading - stats.Finished
	if stats.Unread < 0 {
		stats.Unread = 0
	}

	weekAgo := time.Now().AddDate(0, 0, -7).Unix()
	appDB.QueryRow("SELECT COUNT(*) FROM reading_session WHERE started_at > ?", weekAgo).Scan(&stats.SessionsThisWeek)
	appDB.QueryRow(`
		SELECT COALESCE(SUM(COALESCE(ended_at, started_at) - started_at), 0),
		       COALESCE(AVG(COALESCE(ended_at, started_at) - started_at), 0)
		FROM reading_session
	`).Scan(&stats.TotalSessionMinutes, &stats.AverageSessionMinutes)
	stats.TotalSessionMinutes /= 60
	stats.AverageSessionMinutes /= 60

	// Books by format (count distinct books, not files)
	rows, err := appDB.Query(`
		SELECT format, COUNT(DISTINCT book_id) as cnt
		FROM book_file
		GROUP BY format
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var format string
			var cnt int64
			rows.Scan(&format, &cnt)
			switch format {
			case "epub":
				stats.BooksByFormat.EPUB += cnt
			case "pdf":
				stats.BooksByFormat.PDF += cnt
			case "cbz", "cbr", "cb7":
				stats.BooksByFormat.CBX += cnt
			case "mp3", "m4a", "m4b", "flac", "ogg", "wav":
				stats.BooksByFormat.Audio += cnt
			default:
				stats.BooksByFormat.Other += cnt
			}
		}
	}

	// Reading activity for last 7 days
	stats.ReadingActivity = make([]ActivityDay, 0, 7)
	now := time.Now()
	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).Unix()
		dayEnd := dayStart + 86400

		var sessionCount int
		var totalSeconds int64
		appDB.QueryRow(`
			SELECT COUNT(*),
			       COALESCE(SUM(COALESCE(ended_at, started_at) - started_at), 0)
			FROM reading_session
			WHERE started_at >= ? AND started_at < ?
		`, dayStart, dayEnd).Scan(&sessionCount, &totalSeconds)

		stats.ReadingActivity = append(stats.ReadingActivity, ActivityDay{
			Date:     day.Format("Mon"),
			Sessions: sessionCount,
			Minutes:  int(totalSeconds / 60),
		})
	}

	// Genre distribution
	genreRows, _ := appDB.Query(`
		SELECT bm.genres, COUNT(*) as cnt
		FROM book_metadata bm
		WHERE bm.genres IS NOT NULL AND bm.genres != '[]'
		GROUP BY bm.genres
		ORDER BY cnt DESC
		LIMIT 10
	`)
	stats.GenreDistribution = []GenreCount{}
	for genreRows.Next() {
		var genresJson string
		var cnt int64
		genreRows.Scan(&genresJson, &cnt)

		// Parse genres JSON and count individually
		var genres []string
		json.Unmarshal([]byte(genresJson), &genres)
		for _, genre := range genres {
			found := false
			for i, existing := range stats.GenreDistribution {
				if existing.Name == genre {
					stats.GenreDistribution[i].Count += cnt
					found = true
					break
				}
			}
			if !found {
				stats.GenreDistribution = append(stats.GenreDistribution, GenreCount{Name: genre, Count: cnt})
			}
		}
	}
	genreRows.Close()

	// Sort genre distribution
	genreSlice := stats.GenreDistribution
	for i := 0; i < len(genreSlice)-1; i++ {
		for j := i + 1; j < len(genreSlice); j++ {
			if genreSlice[i].Count < genreSlice[j].Count {
				genreSlice[i], genreSlice[j] = genreSlice[j], genreSlice[i]
			}
		}
	}
	if len(genreSlice) > 10 {
		genreSlice = genreSlice[:10]
	}
	stats.GenreDistribution = genreSlice

	// Author distribution
	authorRows, _ := appDB.Query(`
		SELECT bm.authors, COUNT(*) as cnt
		FROM book_metadata bm
		WHERE bm.authors IS NOT NULL AND bm.authors != '[]'
		GROUP BY bm.authors
		ORDER BY cnt DESC
		LIMIT 10
	`)
	stats.AuthorDistribution = []AuthorCount{}
	for authorRows.Next() {
		var authorsJson string
		var cnt int64
		authorRows.Scan(&authorsJson, &cnt)

		// Parse authors JSON and count individually
		var authors []string
		json.Unmarshal([]byte(authorsJson), &authors)
		for _, author := range authors {
			found := false
			for i, existing := range stats.AuthorDistribution {
				if existing.Name == author {
					stats.AuthorDistribution[i].Count += cnt
					found = true
					break
				}
			}
			if !found {
				stats.AuthorDistribution = append(stats.AuthorDistribution, AuthorCount{Name: author, Count: cnt})
			}
		}
	}
	authorRows.Close()

	// Sort and limit
	authorSlice := stats.AuthorDistribution
	for i := 0; i < len(authorSlice)-1; i++ {
		for j := i + 1; j < len(authorSlice); j++ {
			if authorSlice[i].Count < authorSlice[j].Count {
				authorSlice[i], authorSlice[j] = authorSlice[j], authorSlice[i]
			}
		}
	}
	if len(authorSlice) > 10 {
		authorSlice = authorSlice[:10]
	}
	stats.AuthorDistribution = authorSlice

	// Language distribution
	languageRows, _ := appDB.Query(`
		SELECT COALESCE(language, '') as language, COUNT(*) as cnt
		FROM book_metadata
		WHERE language IS NOT NULL AND language != ''
		GROUP BY language
		ORDER BY cnt DESC, language ASC
		LIMIT 10
	`)
	stats.LanguageDistribution = []CountItem{}
	for languageRows.Next() {
		var language string
		var cnt int64
		languageRows.Scan(&language, &cnt)
		stats.LanguageDistribution = append(stats.LanguageDistribution, CountItem{Label: language, Count: cnt})
	}
	languageRows.Close()

	// Page count histogram using dynamic buckets and a separate missing-page-count total.
	pageRows, _ := appDB.Query(`
		SELECT COALESCE(page_count, 0)
		FROM book_metadata
	`)
	var pageCounts []int64
	var maxPageCount int64
	for pageRows.Next() {
		var pageCount int64
		if err := pageRows.Scan(&pageCount); err != nil {
			continue
		}
		if pageCount <= 0 {
			stats.PageCountMissing++
			continue
		}
		pageCounts = append(pageCounts, pageCount)
		if pageCount > maxPageCount {
			maxPageCount = pageCount
		}
	}
	pageRows.Close()

	if len(pageCounts) > 0 {
		bucketSize := maxPageCount / 6
		if bucketSize < 50 {
			bucketSize = 50
		}
		bucketCount := int((maxPageCount + bucketSize - 1) / bucketSize)
		if bucketCount < 4 {
			bucketCount = 4
		}

		stats.PageCountBuckets = make([]CountItem, 0, bucketCount)
		for start := int64(1); start <= maxPageCount; start += bucketSize {
			end := start + bucketSize - 1
			if end > maxPageCount {
				end = maxPageCount
			}
			var cnt int64
			for _, pageCount := range pageCounts {
				if pageCount >= start && pageCount <= end {
					cnt++
				}
			}
			label := fmt.Sprintf("%d-%d", start, end)
			if start == end {
				label = fmt.Sprintf("%d", start)
			}
			stats.PageCountBuckets = append(stats.PageCountBuckets, CountItem{Label: label, Count: cnt})
		}
	}

	// Session duration buckets
	sessionRows, _ := appDB.Query(`
		SELECT
			CASE
				WHEN COALESCE(ended_at, started_at) - started_at < 600 THEN '<10m'
				WHEN COALESCE(ended_at, started_at) - started_at < 1800 THEN '10-30m'
				WHEN COALESCE(ended_at, started_at) - started_at < 3600 THEN '30-60m'
				WHEN COALESCE(ended_at, started_at) - started_at < 7200 THEN '1-2h'
				ELSE '2h+'
			END as bucket,
			COUNT(*) as cnt
		FROM reading_session
		GROUP BY bucket
	`)
	stats.SessionBuckets = []CountItem{}
	for sessionRows.Next() {
		var bucket string
		var cnt int64
		sessionRows.Scan(&bucket, &cnt)
		stats.SessionBuckets = append(stats.SessionBuckets, CountItem{Label: bucket, Count: cnt})
	}
	sessionRows.Close()
	sort.Slice(stats.SessionBuckets, func(i, j int) bool {
		order := map[string]int{"<10m": 0, "10-30m": 1, "30-60m": 2, "1-2h": 3, "2h+": 4}
		return order[stats.SessionBuckets[i].Label] < order[stats.SessionBuckets[j].Label]
	})

	// Current reading streak from most recent session days
	streakRows, _ := appDB.Query(`
		SELECT DISTINCT date(started_at, 'unixepoch') as day
		FROM reading_session
		ORDER BY day DESC
	`)
	var streakDays []string
	for streakRows.Next() {
		var day string
		streakRows.Scan(&day)
		streakDays = append(streakDays, day)
	}
	streakRows.Close()
	if len(streakDays) > 0 {
		current, err := time.Parse("2006-01-02", streakDays[0])
		if err == nil {
			streak := int64(1)
			for i := 1; i < len(streakDays); i++ {
				day, err := time.Parse("2006-01-02", streakDays[i])
				if err != nil {
					break
				}
				if current.AddDate(0, 0, -1).Equal(day) {
					streak++
					current = day
					continue
				}
				break
			}
			stats.CurrentReadingStreak = streak
		}
	}

	// Rating distribution
	ratingRows, _ := appDB.Query(`
		SELECT ROUND(bm.rating, 0) as rating, COUNT(*) as cnt
		FROM book_metadata bm
		WHERE bm.rating > 0
		GROUP BY ROUND(bm.rating, 0)
		ORDER BY rating
	`)
	stats.RatingDistribution = []RatingCount{}
	for ratingRows.Next() {
		var rating float64
		var cnt int64
		ratingRows.Scan(&rating, &cnt)
		stats.RatingDistribution = append(stats.RatingDistribution, RatingCount{Rating: rating, Count: cnt})
	}
	ratingRows.Close()

	// Publication year timeline
	yearRows, _ := appDB.Query(`
		SELECT bm.pub_date, COUNT(*) as cnt
		FROM book_metadata bm
		WHERE bm.pub_date IS NOT NULL AND bm.pub_date != '' AND LENGTH(bm.pub_date) >= 4
		GROUP BY bm.pub_date
		ORDER BY bm.pub_date
	`)
	stats.PubYearTimeline = []YearCount{}
	for yearRows.Next() {
		var pubDate string
		var cnt int64
		yearRows.Scan(&pubDate, &cnt)

		// Extract year from date string (assuming YYYY or YYYY-MM-DD format)
		if len(pubDate) >= 4 {
			year, err := strconv.Atoi(pubDate[:4])
			if err == nil && year > 0 {
				found := false
				for i, existing := range stats.PubYearTimeline {
					if existing.Year == year {
						stats.PubYearTimeline[i].Count += cnt
						found = true
						break
					}
				}
				if !found {
					stats.PubYearTimeline = append(stats.PubYearTimeline, YearCount{Year: year, Count: cnt})
				}
			}
		}
	}
	yearRows.Close()

	// Reading progress over time (books finished per day for last 30 days)
	stats.ReadingProgress = []ReadingProgress{}
	now2 := time.Now()
	for i := 29; i >= 0; i-- {
		day := now2.AddDate(0, 0, -i)
		dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).Unix()
		dayEnd := dayStart + 86400

		var booksRead int64
		appDB.QueryRow(`
			SELECT COUNT(DISTINCT rs.book_id)
			FROM reading_session rs
			WHERE rs.started_at >= ? AND rs.started_at < ?
		`, dayStart, dayEnd).Scan(&booksRead)

		stats.ReadingProgress = append(stats.ReadingProgress, ReadingProgress{
			Date:  day.Format("2006-01-02"),
			Pages: 0,
			Books: booksRead,
		})
	}

	jsonResponse(w, http.StatusOK, stats)
}

// GetBookTextHandler extracts plain text from any supported book format for the speed reader
func GetBookTextHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	current := getUserFromContext(r.Context())
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

	type fileResult struct {
		Path   string
		Format string
	}

	var files []fileResult
	rows, err := appDB.Query(`
		SELECT path, format FROM book_file WHERE book_id = ?
	`, bookID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to query book files")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var fr fileResult
		if err := rows.Scan(&fr.Path, &fr.Format); err != nil {
			continue
		}
		files = append(files, fr)
	}

	if len(files) == 0 {
		errorResponse(w, http.StatusNotFound, "No files found for book")
		return
	}

	var text string
	var lastErr error

	for _, file := range files {
		format := strings.ToLower(file.Format)
		filePath := translateHostPathToContainerPath(file.Path)
		switch format {
		case "pdf":
			text, lastErr = extractPdfText(bookID, filePath)
			if lastErr == nil {
				goto success
			}
		default:
			if isSupportedTextBookFormat(format) {
				result, err := ensureProcessedTextBook(bookID, filePath, format)
				if err == nil {
					text = result.PlainText
					goto success
				}
				lastErr = err
				continue
			}

			data, err := os.ReadFile(filePath)
			if err == nil && (format == "txt" || format == "text") {
				text = string(data)
				goto success
			}
			lastErr = err
		}
	}

	if lastErr != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to extract text: %v", lastErr))
		return
	}

success:
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(text))
}

// GetEpubTextHandler extracts plain text from an EPUB for the speed reader
func GetEpubTextHandler(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	current := getUserFromContext(r.Context())
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
	err = appDB.QueryRow(`
		SELECT path FROM book_file WHERE book_id = ? AND format = 'epub' LIMIT 1
	`, bookID).Scan(&filePath)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "EPUB file not found")
		return
	}

	filePath = translateHostPathToContainerPath(filePath)
	result, err := ensureProcessedTextBook(bookID, filePath, "epub")
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to extract text: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(result.PlainText))
}

type epubContainer struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

type epubPackage struct {
	Manifest []struct {
		ID        string `xml:"id,attr"`
		Href      string `xml:"href,attr"`
		MediaType string `xml:"media-type,attr"`
	} `xml:"manifest>item"`
	Spine []struct {
		IDRef string `xml:"idref,attr"`
	} `xml:"spine>itemref"`
}

func extractEpubText(filePath string) (string, error) {
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	// Build a lookup map for fast file access
	fileMap := make(map[string]*zip.File)
	for _, f := range zr.File {
		fileMap[f.Name] = f
	}

	// Find OPF path from META-INF/container.xml
	opfPath := ""
	if cf, ok := fileMap["META-INF/container.xml"]; ok {
		rc, err := cf.Open()
		if err == nil {
			var container epubContainer
			xml.NewDecoder(rc).Decode(&container)
			rc.Close()
			if len(container.Rootfiles) > 0 {
				opfPath = container.Rootfiles[0].FullPath
			}
		}
	}

	// Fallback: find any .opf file
	if opfPath == "" {
		for name := range fileMap {
			if strings.HasSuffix(strings.ToLower(name), ".opf") {
				opfPath = name
				break
			}
		}
	}
	if opfPath == "" {
		return "", fmt.Errorf("OPF not found in EPUB")
	}

	// Parse OPF
	opfDir := filepath.Dir(opfPath)
	if opfDir == "." {
		opfDir = ""
	}

	var pkg epubPackage
	if of, ok := fileMap[opfPath]; ok {
		rc, err := of.Open()
		if err == nil {
			xml.NewDecoder(rc).Decode(&pkg)
			rc.Close()
		}
	}

	// Build manifest map: id -> resolved path
	manifestMap := make(map[string]string)
	for _, item := range pkg.Manifest {
		if strings.Contains(item.MediaType, "html") || strings.Contains(item.MediaType, "xml") {
			href := item.Href
			if opfDir != "" {
				href = opfDir + "/" + href
			}
			manifestMap[item.ID] = href
		}
	}

	// Extract text in spine order
	var sb strings.Builder
	for _, spine := range pkg.Spine {
		href, ok := manifestMap[spine.IDRef]
		if !ok {
			continue
		}
		f, ok := fileMap[href]
		if !ok {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		sb.WriteString(stripHTMLTags(string(data)))
		sb.WriteString(" ")
	}

	// Collapse whitespace
	return strings.Join(strings.Fields(sb.String()), " "), nil
}

// stripHTMLTags removes HTML/XML tags and script/style content
func stripHTMLTags(s string) string {
	var result strings.Builder
	result.Grow(len(s))
	inTag := false
	skipContent := false // for script/style blocks
	i := 0
	lower := strings.ToLower(s)

	for i < len(s) {
		// Check for opening script/style tag
		if !inTag && i+7 <= len(lower) && (lower[i:i+7] == "<script" || lower[i:i+6] == "<style") {
			skipContent = true
			inTag = true
			result.WriteByte(' ')
			i++
			continue
		}
		// Check for closing script/style
		if skipContent {
			if i+9 <= len(lower) && lower[i:i+9] == "</script>" {
				skipContent = false
				inTag = false
				i += 9
				continue
			}
			if i+8 <= len(lower) && lower[i:i+8] == "</style>" {
				skipContent = false
				inTag = false
				i += 8
				continue
			}
			i++
			continue
		}

		c := s[i]
		if c == '<' {
			inTag = true
			result.WriteByte(' ')
		} else if c == '>' {
			inTag = false
		} else if !inTag {
			// Decode basic HTML entities
			if c == '&' && i+4 <= len(s) {
				switch {
				case strings.HasPrefix(s[i:], "&amp;"):
					result.WriteByte('&')
					i += 5
					continue
				case strings.HasPrefix(s[i:], "&lt;"):
					result.WriteByte('<')
					i += 4
					continue
				case strings.HasPrefix(s[i:], "&gt;"):
					result.WriteByte('>')
					i += 4
					continue
				case strings.HasPrefix(s[i:], "&nbsp;"):
					result.WriteByte(' ')
					i += 6
					continue
				case strings.HasPrefix(s[i:], "&quot;"):
					result.WriteByte('"')
					i += 6
					continue
				case strings.HasPrefix(s[i:], "&apos;"):
					result.WriteByte('\'')
					i += 6
					continue
				}
			}
			result.WriteByte(c)
		}
		i++
	}
	return result.String()
}

// extractPdfText extracts text from a PDF file, using a per-book cache when possible.
func extractPdfText(bookID, filePath string) (string, error) {
	if text, err := loadCachedPdfText(bookID, filePath); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}

	if text, err := extractPdfTextWithPdftotext(filePath); err == nil && strings.TrimSpace(text) != "" {
		_ = saveCachedPdfText(bookID, filePath, text)
		return text, nil
	}

	if text, err := extractPdfTextWithCalibre(filePath); err == nil && strings.TrimSpace(text) != "" {
		_ = saveCachedPdfText(bookID, filePath, text)
		return text, nil
	}

	file, r, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open PDF: %w", err)
	}
	defer file.Close()

	var sb strings.Builder
	totalPages := r.NumPage()

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		p := r.Page(pageNum)
		if p.V.IsNull() {
			continue
		}

		text, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}

		sb.WriteString(text)
		sb.WriteString(" ")
	}

	text := strings.Join(strings.Fields(sb.String()), " ")
	if strings.TrimSpace(text) != "" {
		_ = saveCachedPdfText(bookID, filePath, text)
	}
	return text, nil
}

func getPdfTextCachePath(bookID, sourcePath string) string {
	sum := sha1.Sum([]byte(sourcePath))
	cacheName := hex.EncodeToString(sum[:]) + ".txt"
	return filepath.Join(appConfig.GetBookCachePath(), bookID, "pdf-text", cacheName)
}

func loadCachedPdfText(bookID, sourcePath string) (string, error) {
	cachePath := getPdfTextCachePath(bookID, sourcePath)
	cacheInfo, err := os.Stat(cachePath)
	if err != nil {
		return "", err
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return "", err
	}

	if cacheInfo.ModTime().Before(sourceInfo.ModTime()) {
		return "", fmt.Errorf("cached pdf text is stale")
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func saveCachedPdfText(bookID, sourcePath, text string) error {
	cachePath := getPdfTextCachePath(bookID, sourcePath)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return err
	}

	if err := os.WriteFile(cachePath, []byte(text), 0644); err != nil {
		return err
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err == nil {
		_ = os.Chtimes(cachePath, sourceInfo.ModTime(), sourceInfo.ModTime())
	}

	return nil
}

func extractPdfTextWithPdftotext(filePath string) (string, error) {
	pdftotextPath, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", err
	}

	cmd := exec.Command(pdftotextPath, "-enc", "UTF-8", "-layout", "-nopgbrk", filePath, "-")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext failed: %s", strings.TrimSpace(stderr.String()))
	}

	return normalizeSpeedReaderText(stdout.String()), nil
}

func extractPdfTextWithCalibre(filePath string) (string, error) {
	calibrePath, err := exec.LookPath("ebook-convert")
	if err != nil {
		return "", err
	}

	tempDir, err := os.MkdirTemp("", "cryptorum-pdf-text-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	outputPath := filepath.Join(tempDir, "book.txt")
	cmd := exec.Command(calibrePath, filePath, outputPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ebook-convert failed: %s", strings.TrimSpace(stderr.String()))
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return "", err
	}
	return normalizeSpeedReaderText(string(data)), nil
}

func normalizeSpeedReaderText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var sb strings.Builder
	sb.Grow(len(text))
	inWhitespace := false
	for _, r := range text {
		if r == '\n' || r == '\t' || r == ' ' || r == '\f' {
			if !inWhitespace {
				sb.WriteByte(' ')
				inWhitespace = true
			}
			continue
		}
		sb.WriteRune(r)
		inWhitespace = false
	}

	normalized := strings.TrimSpace(sb.String())
	replacements := []struct {
		old string
		new string
	}{
		{" ,", ","},
		{" .", "."},
		{" ;", ";"},
		{" :", ":"},
		{" !", "!"},
		{" ?", "?"},
		{" )", ")"},
		{" ]", "]"},
		{" }", "}"},
		{"( ", "("},
		{"[ ", "["},
		{"{ ", "{"},
	}
	for _, replacement := range replacements {
		normalized = strings.ReplaceAll(normalized, replacement.old, replacement.new)
	}
	return normalized
}
