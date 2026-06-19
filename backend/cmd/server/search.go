package main

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	searchLimit             = 50
	searchCandidateLimit    = 250
	searchFallbackScanLimit = 5000
	minBookSearchScore      = 1.45
)

type SearchResult struct {
	ID                  int64   `json:"id"`
	Title               string  `json:"title"`
	Authors             string  `json:"authors"`
	Description         string  `json:"description"`
	Series              string  `json:"series,omitempty"`
	SeriesNumber        float64 `json:"series_number,omitempty"`
	SeriesNumberDisplay string  `json:"series_number_display,omitempty"`
	Format              string  `json:"format,omitempty"`
	FilePath            string  `json:"file_path,omitempty"`
	CoverPath           string  `json:"cover_path"`
	Status              string  `json:"status"`
	Percent             float64 `json:"percent"`
	Opened              bool    `json:"opened"`
	AddedAt             int64   `json:"added_at"`
	LastReadAt          int64   `json:"last_read_at"`
}

type bookSearchCandidate struct {
	result   SearchResult
	ftsRank  float64
	hasFTS   bool
	position int
}

type scoredBookSearchResult struct {
	result SearchResult
	score  float64
}

type BookSearchFilters struct {
	Author     []string
	Series     []string
	Genre      []string
	Tags       []string
	Status     []string
	Format     []string
	FilterMode string
	Sort       string
	SortDir    string
}

const activeFileSearchTextSQL = `COALESCE((
	SELECT group_concat(
		COALESCE(bf.format, '') || ' ' ||
		CASE
			WHEN lp.path IS NOT NULL AND bf.path = lp.path THEN ''
			WHEN lp.path IS NOT NULL AND bf.path LIKE lp.path || '/%' THEN substr(bf.path, length(lp.path) + 2)
			ELSE bf.path
		END,
		' '
	)
	FROM book_file bf
	LEFT JOIN library_path lp
		ON lp.library_id = b.library_id
		AND (bf.path = lp.path OR bf.path LIKE lp.path || '/%')
	WHERE bf.book_id = b.id AND bf.missing_at IS NULL
), '')`

func searchBooks(query string, libraryID string, current *AppUser, filters BookSearchFilters) ([]SearchResult, error) {
	queryTokens := searchTokens(query)
	if len(queryTokens) == 0 {
		return []SearchResult{}, nil
	}

	candidates := make(map[int64]bookSearchCandidate)
	if ftsQuery := buildFTSQuery(queryTokens); ftsQuery != "" {
		ftsCandidates, err := queryFTSBookCandidates(ftsQuery, libraryID, current, filters)
		if err == nil {
			for i, candidate := range ftsCandidates {
				candidate.hasFTS = true
				candidate.position = i
				candidates[candidate.result.ID] = candidate
			}
		}
	}

	tokenCandidates, err := queryTokenLikeBookCandidates(queryTokens, libraryID, current, filters)
	if err != nil {
		return nil, err
	}
	for _, candidate := range tokenCandidates {
		if existing, exists := candidates[candidate.result.ID]; exists {
			candidate.hasFTS = existing.hasFTS
			candidate.position = existing.position
			candidate.ftsRank = existing.ftsRank
		}
		candidates[candidate.result.ID] = candidate
	}

	if len(candidates) < searchCandidateLimit {
		fallbackCandidates, err := queryFallbackBookCandidates(libraryID, current, filters)
		if err != nil {
			return nil, err
		}
		for _, candidate := range fallbackCandidates {
			if _, exists := candidates[candidate.result.ID]; !exists {
				candidates[candidate.result.ID] = candidate
			}
		}
	}

	scored := make([]scoredBookSearchResult, 0, len(candidates))
	for _, candidate := range candidates {
		score := scoreBookSearchCandidate(queryTokens, candidate)
		if score >= minBookSearchScore {
			scored = append(scored, scoredBookSearchResult{
				result: candidate.result,
				score:  score,
			})
		}
	}

	sortScoredBookSearchResults(scored, filters.Sort, filters.SortDir)

	if len(scored) > searchLimit {
		scored = scored[:searchLimit]
	}

	results := make([]SearchResult, len(scored))
	for i, item := range scored {
		results[i] = item.result
	}
	return results, nil
}

func queryFTSBookCandidates(ftsQuery string, libraryID string, current *AppUser, filters BookSearchFilters) ([]bookSearchCandidate, error) {
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	var libraryClause string
	var libraryArgs []interface{}
	if libraryID != "" {
		libraryClause = " AND b.library_id = ?"
		libraryArgs = append(libraryArgs, libraryID)
	}
	filterClause, filterArgs := buildBookSearchFilterClause(filters)

	args := append(append([]interface{}{}, ownerArgs...), libraryArgs...)
	args = append(args, filterArgs...)
	args = append(args, ftsQuery, searchCandidateLimit)
	rows, err := appDB.Query(`
		SELECT b.id,
		       COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.description, '') as description,
		       COALESCE(bm.series, '') as series,
		       COALESCE(bm.series_number, 0) as series_number,
		       COALESCE(bm.series_number_display, '') as series_number_display,
		       COALESCE((SELECT bf.format FROM book_file bf WHERE bf.book_id = b.id AND bf.missing_at IS NULL ORDER BY bf.format ASC LIMIT 1), '') as format,
		       `+activeFileSearchTextSQL+` as file_path,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(rp.status, 'unread') as status,
		       COALESCE(rp.percent, 0) as percent,
		       CASE WHEN rp.book_id IS NOT NULL THEN 1 ELSE 0 END as opened,
		       b.added_at,
		       COALESCE(rp.updated_at, 0) as last_read_at,
		       bm25(book_fts, 5.0, 3.0, 0.5, 2.0) as rank
		FROM book_fts
		JOIN book_metadata bm ON bm.id = book_fts.rowid
		JOIN book b ON bm.book_id = b.id
		JOIN library l ON b.library_id = l.id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		WHERE (`+ownerClause+`)`+libraryClause+filterClause+` AND book_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []bookSearchCandidate
	for rows.Next() {
		var candidate bookSearchCandidate
		if err := rows.Scan(
			&candidate.result.ID,
			&candidate.result.Title,
			&candidate.result.Authors,
			&candidate.result.Description,
			&candidate.result.Series,
			&candidate.result.SeriesNumber,
			&candidate.result.SeriesNumberDisplay,
			&candidate.result.Format,
			&candidate.result.FilePath,
			&candidate.result.CoverPath,
			&candidate.result.Status,
			&candidate.result.Percent,
			&candidate.result.Opened,
			&candidate.result.AddedAt,
			&candidate.result.LastReadAt,
			&candidate.ftsRank,
		); err != nil {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func queryTokenLikeBookCandidates(
	queryTokens []string,
	libraryID string,
	current *AppUser,
	filters BookSearchFilters,
) ([]bookSearchCandidate, error) {
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	var libraryClause string
	var libraryArgs []interface{}
	if libraryID != "" {
		libraryClause = " AND b.library_id = ?"
		libraryArgs = append(libraryArgs, libraryID)
	}
	filterClause, filterArgs := buildBookSearchFilterClause(filters)

	searchText := `LOWER(
		COALESCE(bm.title, '') || ' ' ||
		COALESCE(bm.authors, '') || ' ' ||
		REPLACE(REPLACE(COALESCE(bm.authors, ''), '.', ''), ' ', '') || ' ' ||
		COALESCE(bm.description, '') || ' ' ||
		COALESCE(bm.series, '') || ' ' ||
		COALESCE(bm.series_number_display, '') || ' ' ||
		CASE WHEN COALESCE(bm.series_number, 0) != 0 THEN CAST(bm.series_number AS TEXT) ELSE '' END || ' ' ||
		COALESCE(bm.isbn, '') || ' ' ||
		COALESCE(bm.asin, '') || ' ' ||
		` + activeFileSearchTextSQL + `
	)`
	tokenConditions := make([]string, 0, len(queryTokens))
	args := append(append([]interface{}{}, ownerArgs...), libraryArgs...)
	args = append(args, filterArgs...)
	for _, token := range queryTokens {
		tokenConditions = append(tokenConditions, searchText+" LIKE ?")
		args = append(args, "%"+token+"%")
	}
	args = append(args, searchCandidateLimit)

	rows, err := appDB.Query(`
		SELECT b.id,
		       COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.description, '') as description,
		       COALESCE(bm.series, '') as series,
		       COALESCE(bm.series_number, 0) as series_number,
		       COALESCE(bm.series_number_display, '') as series_number_display,
		       COALESCE((SELECT bf.format FROM book_file bf WHERE bf.book_id = b.id AND bf.missing_at IS NULL ORDER BY bf.format ASC LIMIT 1), '') as format,
		       `+activeFileSearchTextSQL+` as file_path,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(rp.status, 'unread') as status,
		       COALESCE(rp.percent, 0) as percent,
		       CASE WHEN rp.book_id IS NOT NULL THEN 1 ELSE 0 END as opened,
		       b.added_at,
		       COALESCE(rp.updated_at, 0) as last_read_at
		FROM book_metadata bm
		JOIN book b ON bm.book_id = b.id
		JOIN library l ON b.library_id = l.id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		WHERE (`+ownerClause+`)`+libraryClause+filterClause+` AND `+strings.Join(tokenConditions, " AND ")+`
		ORDER BY
			CASE WHEN LOWER(COALESCE(bm.title, '')) LIKE ? THEN 0 ELSE 1 END,
			b.added_at DESC
		LIMIT ?
	`, append(args[:len(args)-1], "%"+strings.Join(queryTokens, "%")+"%", args[len(args)-1])...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []bookSearchCandidate
	for rows.Next() {
		var candidate bookSearchCandidate
		if err := rows.Scan(
			&candidate.result.ID,
			&candidate.result.Title,
			&candidate.result.Authors,
			&candidate.result.Description,
			&candidate.result.Series,
			&candidate.result.SeriesNumber,
			&candidate.result.SeriesNumberDisplay,
			&candidate.result.Format,
			&candidate.result.FilePath,
			&candidate.result.CoverPath,
			&candidate.result.Status,
			&candidate.result.Percent,
			&candidate.result.Opened,
			&candidate.result.AddedAt,
			&candidate.result.LastReadAt,
		); err != nil {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func queryFallbackBookCandidates(libraryID string, current *AppUser, filters BookSearchFilters) ([]bookSearchCandidate, error) {
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	var libraryClause string
	var libraryArgs []interface{}
	if libraryID != "" {
		libraryClause = " AND b.library_id = ?"
		libraryArgs = append(libraryArgs, libraryID)
	}
	filterClause, filterArgs := buildBookSearchFilterClause(filters)

	args := append(append([]interface{}{}, ownerArgs...), libraryArgs...)
	args = append(args, filterArgs...)
	args = append(args, searchFallbackScanLimit)
	rows, err := appDB.Query(`
		SELECT b.id,
		       COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.description, '') as description,
		       COALESCE(bm.series, '') as series,
		       COALESCE(bm.series_number, 0) as series_number,
		       COALESCE(bm.series_number_display, '') as series_number_display,
		       COALESCE((SELECT bf.format FROM book_file bf WHERE bf.book_id = b.id AND bf.missing_at IS NULL ORDER BY bf.format ASC LIMIT 1), '') as format,
		       `+activeFileSearchTextSQL+` as file_path,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(rp.status, 'unread') as status,
		       COALESCE(rp.percent, 0) as percent,
		       CASE WHEN rp.book_id IS NOT NULL THEN 1 ELSE 0 END as opened,
		       b.added_at,
		       COALESCE(rp.updated_at, 0) as last_read_at
		FROM book_metadata bm
		JOIN book b ON bm.book_id = b.id
		JOIN library l ON b.library_id = l.id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		WHERE (`+ownerClause+`)`+libraryClause+filterClause+`
		ORDER BY b.added_at DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []bookSearchCandidate
	for rows.Next() {
		var candidate bookSearchCandidate
		if err := rows.Scan(
			&candidate.result.ID,
			&candidate.result.Title,
			&candidate.result.Authors,
			&candidate.result.Description,
			&candidate.result.Series,
			&candidate.result.SeriesNumber,
			&candidate.result.SeriesNumberDisplay,
			&candidate.result.Format,
			&candidate.result.FilePath,
			&candidate.result.CoverPath,
			&candidate.result.Status,
			&candidate.result.Percent,
			&candidate.result.Opened,
			&candidate.result.AddedAt,
			&candidate.result.LastReadAt,
		); err != nil {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func buildBookSearchFilterClause(filters BookSearchFilters) (string, []interface{}) {
	var filterConditions []string
	var filterArgs []interface{}

	addFilterCondition := func(condition string, values ...interface{}) {
		filterConditions = append(filterConditions, condition)
		filterArgs = append(filterArgs, values...)
	}

	for _, value := range filters.Status {
		addFilterCondition("COALESCE(rp.status, 'unread') = ?", value)
	}
	for _, value := range filters.Author {
		addAuthorFilterCondition(addFilterCondition, "bm.authors", value)
	}
	for _, value := range filters.Series {
		addFilterCondition("COALESCE(bm.series, '') = ?", value)
	}
	for _, value := range filters.Genre {
		addHierarchicalJSONFilterCondition(addFilterCondition, "bm.genres", value)
	}
	for _, value := range filters.Tags {
		addHierarchicalJSONFilterCondition(addFilterCondition, "bm.tags", value)
	}
	for _, value := range filters.Format {
		format := strings.ToLower(strings.TrimSpace(value))
		if format != "" {
			addFilterCondition("EXISTS (SELECT 1 FROM book_file filter_bf WHERE filter_bf.book_id = b.id AND filter_bf.missing_at IS NULL AND LOWER(filter_bf.format) = ?)", format)
		}
	}

	if len(filterConditions) == 0 {
		return "", nil
	}

	filterMode := strings.ToUpper(filters.FilterMode)
	switch filterMode {
	case "OR":
		return " AND (" + strings.Join(filterConditions, " OR ") + ")", filterArgs
	case "NOT":
		return " AND NOT (" + strings.Join(filterConditions, " OR ") + ")", filterArgs
	default:
		return " AND " + strings.Join(filterConditions, " AND "), filterArgs
	}
}

func sortScoredBookSearchResults(scored []scoredBookSearchResult, sortBy, sortDir string) {
	desc := strings.EqualFold(sortDir, "desc")
	sortBy = strings.TrimSpace(sortBy)
	if sortBy == "" {
		sortBy = "title"
	}

	sort.Slice(scored, func(i, j int) bool {
		left := scored[i].result
		right := scored[j].result
		compare := 0
		switch sortBy {
		case "authors":
			compare = strings.Compare(authorsSearchText(left.Authors), authorsSearchText(right.Authors))
		case "added_at":
			compare = compareInt64(left.AddedAt, right.AddedAt)
		case "last_read":
			compare = compareInt64(left.LastReadAt, right.LastReadAt)
		case "series":
			compare = compareSeriesSearchResults(left, right, desc)
			return compare < 0
		case "relevance":
			if math.Abs(scored[i].score-scored[j].score) < 0.0001 {
				compare = strings.Compare(left.Title, right.Title)
			} else if scored[i].score > scored[j].score {
				compare = -1
			} else {
				compare = 1
			}
		default:
			compare = strings.Compare(strings.ToLower(left.Title), strings.ToLower(right.Title))
		}
		if compare == 0 {
			compare = strings.Compare(strings.ToLower(left.Title), strings.ToLower(right.Title))
		}
		if desc {
			return compare > 0
		}
		return compare < 0
	})
}

func compareInt64(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareSeriesNumber(left, right float64) int {
	if left == 0 && right != 0 {
		return 1
	}
	if left != 0 && right == 0 {
		return -1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareMissingLast(left, right string) int {
	left = strings.ToLower(strings.TrimSpace(left))
	right = strings.ToLower(strings.TrimSpace(right))
	if left == "" && right != "" {
		return 1
	}
	if left != "" && right == "" {
		return -1
	}
	return strings.Compare(left, right)
}

func compareSeriesSearchResults(left, right SearchResult, desc bool) int {
	seriesCompare := compareMissingLast(left.Series, right.Series)
	if seriesCompare != 0 {
		return seriesCompare
	}

	seriesCompare = strings.Compare(strings.ToLower(left.Series), strings.ToLower(right.Series))
	if desc {
		seriesCompare = -seriesCompare
	}
	if seriesCompare != 0 {
		return seriesCompare
	}

	numberCompare := compareSeriesNumber(left.SeriesNumber, right.SeriesNumber)
	if desc && left.SeriesNumber != 0 && right.SeriesNumber != 0 {
		numberCompare = -numberCompare
	}
	if numberCompare != 0 {
		return numberCompare
	}

	return strings.Compare(strings.ToLower(left.Title), strings.ToLower(right.Title))
}

func buildFTSQuery(tokens []string) string {
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if len([]rune(token)) < 2 {
			continue
		}
		parts = append(parts, token+"*")
	}
	return strings.Join(parts, " AND ")
}

func scoreBookSearchCandidate(queryTokens []string, candidate bookSearchCandidate) float64 {
	seriesText := seriesSearchText(candidate.result)
	titleScore, titleCoverage := scoreSearchField(queryTokens, candidate.result.Title)
	authorsScore, authorsCoverage := scoreSearchField(queryTokens, authorsSearchText(candidate.result.Authors))
	seriesScore, seriesCoverage := scoreSearchField(queryTokens, seriesText)
	descriptionScore, descriptionCoverage := scoreSearchField(queryTokens, candidate.result.Description)
	fileScore, fileCoverage := scoreSearchField(queryTokens, candidate.result.Format+" "+candidate.result.FilePath)

	score := titleScore*5.0 + authorsScore*3.0 + seriesScore*2.0 + descriptionScore*0.5 + fileScore*1.25
	bestCoverage := math.Max(titleCoverage, math.Max(authorsCoverage, math.Max(seriesCoverage, math.Max(descriptionCoverage, fileCoverage))))
	totalCoverage := combinedSearchCoverage(queryTokens, []string{
		candidate.result.Title,
		authorsSearchText(candidate.result.Authors),
		seriesText,
		candidate.result.Description,
		candidate.result.Format + " " + candidate.result.FilePath,
	})

	score += totalCoverage * 2.0
	if bestCoverage >= 1 {
		score += 1.25
	}
	if candidate.hasFTS {
		score += 1.0 + math.Max(0, 1.0-(float64(candidate.position)/float64(searchCandidateLimit)))
	}
	if exactNormalizedContains(candidate.result.Title, strings.Join(queryTokens, " ")) {
		score += 2.0
	}

	if totalCoverage < requiredSearchCoverage(len(queryTokens)) {
		return 0
	}
	return score
}

func seriesSearchText(result SearchResult) string {
	var parts []string
	if result.Series != "" {
		parts = append(parts, result.Series)
	}
	if result.SeriesNumberDisplay != "" {
		parts = append(parts, result.SeriesNumberDisplay)
	}
	if result.SeriesNumber != 0 {
		parts = append(parts, strconvFormatSeriesNumber(result.SeriesNumber))
	}
	return strings.Join(parts, " ")
}

func strconvFormatSeriesNumber(value float64) string {
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func scoreSearchField(queryTokens []string, field string) (float64, float64) {
	fieldTokens := searchTokens(field)
	if len(queryTokens) == 0 || len(fieldTokens) == 0 {
		return 0, 0
	}

	total := 0.0
	matched := 0
	for _, queryToken := range queryTokens {
		best := 0.0
		for _, fieldToken := range fieldTokens {
			if score := scoreSearchToken(queryToken, fieldToken); score > best {
				best = score
			}
		}
		if best >= 0.62 {
			matched++
			total += best
		}
	}
	return total / float64(len(queryTokens)), float64(matched) / float64(len(queryTokens))
}

func combinedSearchCoverage(queryTokens []string, fields []string) float64 {
	if len(queryTokens) == 0 {
		return 0
	}
	matched := 0
	for _, queryToken := range queryTokens {
		best := 0.0
		for _, field := range fields {
			for _, fieldToken := range searchTokens(field) {
				if score := scoreSearchToken(queryToken, fieldToken); score > best {
					best = score
				}
			}
		}
		if best >= 0.62 {
			matched++
		}
	}
	return float64(matched) / float64(len(queryTokens))
}

func requiredSearchCoverage(tokenCount int) float64 {
	if tokenCount <= 2 {
		return 1
	}
	if tokenCount == 3 {
		return 0.67
	}
	return 0.6
}

func scoreSearchToken(queryToken string, candidateToken string) float64 {
	if queryToken == "" || candidateToken == "" {
		return 0
	}
	if queryToken == candidateToken {
		return 1
	}
	if singularSearchToken(queryToken) == singularSearchToken(candidateToken) {
		return 0.96
	}
	if strings.HasPrefix(candidateToken, queryToken) || strings.HasPrefix(queryToken, candidateToken) {
		return 0.9
	}
	if len([]rune(queryToken)) >= 4 &&
		(strings.Contains(candidateToken, queryToken) || strings.Contains(queryToken, candidateToken)) {
		return 0.78
	}

	distance := damerauLevenshteinDistance(queryToken, candidateToken)
	maxLen := max(len([]rune(queryToken)), len([]rune(candidateToken)))
	switch {
	case maxLen <= 4 && distance == 1:
		return 0.7
	case maxLen <= 7 && distance <= 1:
		return 0.82
	case maxLen > 7 && distance <= 2:
		return 0.74
	default:
		return 0
	}
}

func searchTokens(value string) []string {
	normalized := normalizeSearchText(value)
	if normalized == "" {
		return nil
	}

	rawTokens := strings.Fields(normalized)
	tokens := make([]string, 0, len(rawTokens))
	seen := make(map[string]bool)
	for _, token := range rawTokens {
		if len([]rune(token)) < 2 || searchStopWords[token] {
			continue
		}
		if !seen[token] {
			tokens = append(tokens, token)
			seen[token] = true
		}
	}
	return tokens
}

func normalizeSearchText(value string) string {
	var builder strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				builder.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func singularSearchToken(token string) string {
	if len(token) <= 3 {
		return token
	}
	if strings.HasSuffix(token, "ies") && len(token) > 4 {
		return strings.TrimSuffix(token, "ies") + "y"
	}
	if strings.HasSuffix(token, "es") && len(token) > 4 {
		return strings.TrimSuffix(token, "es")
	}
	if strings.HasSuffix(token, "s") && !strings.HasSuffix(token, "ss") {
		return strings.TrimSuffix(token, "s")
	}
	return token
}

func exactNormalizedContains(field string, query string) bool {
	return strings.Contains(normalizeSearchText(field), normalizeSearchText(query))
}

func authorsSearchText(raw string) string {
	var authors []string
	if err := json.Unmarshal([]byte(raw), &authors); err == nil {
		parts := make([]string, 0, len(authors)*2)
		for _, author := range authors {
			parts = append(parts, author, normalizedAuthorMatchKey(author))
		}
		return strings.Join(parts, " ")
	}
	return raw + " " + normalizedAuthorMatchKey(raw)
}

func damerauLevenshteinDistance(left string, right string) int {
	a := []rune(left)
	b := []rune(right)
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	dist := make([][]int, len(a)+1)
	for i := range dist {
		dist[i] = make([]int, len(b)+1)
		dist[i][0] = i
	}
	for j := 1; j <= len(b); j++ {
		dist[0][j] = j
	}

	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			dist[i][j] = min(
				dist[i-1][j]+1,
				dist[i][j-1]+1,
				dist[i-1][j-1]+cost,
			)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				dist[i][j] = min(dist[i][j], dist[i-2][j-2]+1)
			}
		}
	}
	return dist[len(a)][len(b)]
}

var searchStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"by": true, "for": true, "from": true, "in": true, "into": true, "is": true,
	"of": true, "on": true, "or": true, "the": true, "to": true, "with": true,
}
