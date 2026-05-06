package main

import (
	"encoding/json"
	"math"
	"sort"
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
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Authors     string `json:"authors"`
	Description string `json:"description"`
	Series      string `json:"series,omitempty"`
	CoverPath   string `json:"cover_path"`
	Status      string `json:"status"`
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

func searchBooks(query string, libraryID string, current *AppUser) ([]SearchResult, error) {
	queryTokens := searchTokens(query)
	if len(queryTokens) == 0 {
		return []SearchResult{}, nil
	}

	candidates := make(map[int64]bookSearchCandidate)
	if ftsQuery := buildFTSQuery(queryTokens); ftsQuery != "" {
		ftsCandidates, err := queryFTSBookCandidates(ftsQuery, libraryID, current)
		if err == nil {
			for i, candidate := range ftsCandidates {
				candidate.hasFTS = true
				candidate.position = i
				candidates[candidate.result.ID] = candidate
			}
		}
	}

	tokenCandidates, err := queryTokenLikeBookCandidates(queryTokens, libraryID, current)
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
		fallbackCandidates, err := queryFallbackBookCandidates(libraryID, current)
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

	sort.Slice(scored, func(i, j int) bool {
		if math.Abs(scored[i].score-scored[j].score) < 0.0001 {
			return scored[i].result.Title < scored[j].result.Title
		}
		return scored[i].score > scored[j].score
	})

	if len(scored) > searchLimit {
		scored = scored[:searchLimit]
	}

	results := make([]SearchResult, len(scored))
	for i, item := range scored {
		results[i] = item.result
	}
	return results, nil
}

func queryFTSBookCandidates(ftsQuery string, libraryID string, current *AppUser) ([]bookSearchCandidate, error) {
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	var libraryClause string
	var libraryArgs []interface{}
	if libraryID != "" {
		libraryClause = " AND b.library_id = ?"
		libraryArgs = append(libraryArgs, libraryID)
	}

	args := append(append([]interface{}{}, ownerArgs...), libraryArgs...)
	args = append(args, ftsQuery, searchCandidateLimit)
	rows, err := appDB.Query(`
		SELECT b.id, COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.description, '') as description,
		       COALESCE(bm.series, '') as series,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(rp.status, 'unread') as status,
		       bm25(book_fts, 5.0, 3.0, 0.5, 2.0) as rank
		FROM book_fts
		JOIN book_metadata bm ON bm.id = book_fts.rowid
		JOIN book b ON bm.book_id = b.id
		JOIN library l ON b.library_id = l.id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		WHERE (`+ownerClause+`)`+libraryClause+` AND book_fts MATCH ?
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
			&candidate.result.CoverPath,
			&candidate.result.Status,
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
) ([]bookSearchCandidate, error) {
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	var libraryClause string
	var libraryArgs []interface{}
	if libraryID != "" {
		libraryClause = " AND b.library_id = ?"
		libraryArgs = append(libraryArgs, libraryID)
	}

	searchText := `LOWER(
		COALESCE(bm.title, '') || ' ' ||
		COALESCE(bm.authors, '') || ' ' ||
		COALESCE(bm.description, '') || ' ' ||
		COALESCE(bm.series, '') || ' ' ||
		COALESCE(bm.isbn, '') || ' ' ||
		COALESCE(bm.asin, '')
	)`
	tokenConditions := make([]string, 0, len(queryTokens))
	args := append(append([]interface{}{}, ownerArgs...), libraryArgs...)
	for _, token := range queryTokens {
		tokenConditions = append(tokenConditions, searchText+" LIKE ?")
		args = append(args, "%"+token+"%")
	}
	args = append(args, searchCandidateLimit)

	rows, err := appDB.Query(`
		SELECT b.id, COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.description, '') as description,
		       COALESCE(bm.series, '') as series,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(rp.status, 'unread') as status
		FROM book_metadata bm
		JOIN book b ON bm.book_id = b.id
		JOIN library l ON b.library_id = l.id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		WHERE (`+ownerClause+`)`+libraryClause+` AND `+strings.Join(tokenConditions, " AND ")+`
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
			&candidate.result.CoverPath,
			&candidate.result.Status,
		); err != nil {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func queryFallbackBookCandidates(libraryID string, current *AppUser) ([]bookSearchCandidate, error) {
	ownerClause, ownerArgs := userOwnershipClause(current, "l")
	var libraryClause string
	var libraryArgs []interface{}
	if libraryID != "" {
		libraryClause = " AND b.library_id = ?"
		libraryArgs = append(libraryArgs, libraryID)
	}

	args := append(append([]interface{}{}, ownerArgs...), libraryArgs...)
	args = append(args, searchFallbackScanLimit)
	rows, err := appDB.Query(`
		SELECT b.id, COALESCE(bm.title, '') as title,
		       COALESCE(bm.authors, '[]') as authors,
		       COALESCE(bm.description, '') as description,
		       COALESCE(bm.series, '') as series,
		       COALESCE(bm.cover_path, '') as cover_path,
		       COALESCE(rp.status, 'unread') as status
		FROM book_metadata bm
		JOIN book b ON bm.book_id = b.id
		JOIN library l ON b.library_id = l.id
		LEFT JOIN reading_progress rp ON b.id = rp.book_id
		WHERE (`+ownerClause+`)`+libraryClause+`
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
			&candidate.result.CoverPath,
			&candidate.result.Status,
		); err != nil {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
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
	titleScore, titleCoverage := scoreSearchField(queryTokens, candidate.result.Title)
	authorsScore, authorsCoverage := scoreSearchField(queryTokens, authorsSearchText(candidate.result.Authors))
	seriesScore, seriesCoverage := scoreSearchField(queryTokens, candidate.result.Series)
	descriptionScore, descriptionCoverage := scoreSearchField(queryTokens, candidate.result.Description)

	score := titleScore*5.0 + authorsScore*3.0 + seriesScore*2.0 + descriptionScore*0.5
	bestCoverage := math.Max(titleCoverage, math.Max(authorsCoverage, math.Max(seriesCoverage, descriptionCoverage)))
	totalCoverage := combinedSearchCoverage(queryTokens, []string{
		candidate.result.Title,
		authorsSearchText(candidate.result.Authors),
		candidate.result.Series,
		candidate.result.Description,
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
		return strings.Join(authors, " ")
	}
	return raw
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
