package filenameinfo

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	recognizedExtensions = map[string]bool{
		".azw": true, ".azw3": true, ".cb7": true, ".cbr": true, ".cbz": true,
		".doc": true, ".docx": true, ".epub": true, ".fb2": true, ".flac": true,
		".m4a": true, ".m4b": true, ".mobi": true, ".mp3": true, ".ogg": true,
		".pdf": true, ".rtf": true, ".txt": true, ".wav": true,
	}
	suspiciousTitleExtensions = []string{".qxp", ".indb", ".doc", ".docx", ".pdf", ".epub"}
	suspiciousAuthorValues    = map[string]bool{
		"anonymous": true, "calibre": true, "microsoft word": true, "unknown": true,
		"various": true, "zamzar": true, "z-library": true, "etc": true, "etc.": true, "www": true,
	}
	spacePattern           = regexp.MustCompile(`\s+`)
	numericTitlePattern    = regexp.MustCompile(`^[0-9][0-9 ._-]*$`)
	hexTitlePattern        = regexp.MustCompile(`(?i)^[0-9a-f]{32,}$`)
	asinTitlePattern       = regexp.MustCompile(`(?i)^b0[0-9a-z]{8}(?:\.[a-z0-9]+)?$`)
	duplicateSuffixPattern = regexp.MustCompile(`\s+\([0-9]{1,2}\)$`)
	authorAcronymPattern   = regexp.MustCompile(`^[A-Z0-9]{2,8}$`)
	authorYearPattern      = regexp.MustCompile(`(?:^|\D)(?:18|19|20|21)[0-9]{2}(?:\D|$)`)
	authorSourcePattern    = regexp.MustCompile(`(?i)\s+\(z-library\)$`)
	domainTitlePattern     = regexp.MustCompile(`(?i)^[^\s]+\.(?:com|net|org)$`)
	authorSplitPattern     = regexp.MustCompile(`(?i)\s*(?:;|\s+&\s+|\s+and\s+)\s*`)
)

type Analysis struct {
	Title            string
	Authors          []string
	Pattern          string
	Confidence       string
	AmbiguousAuthors bool
}

func Analyze(path string) Analysis {
	name := Stem(path)
	result := Analysis{Title: name, Authors: []string{}, Pattern: "filename title", Confidence: "medium"}
	if name == "" {
		return result
	}

	if title, author, ok := trailingParenthetical(name); ok && likelyParentheticalAuthor(author) {
		result.Title = title
		result.Authors, result.AmbiguousAuthors = splitAuthors(author)
		result.Pattern = "trailing parenthetical author"
		result.Confidence = "medium"
		return finalize(result)
	}

	if index := strings.LastIndex(name, " - "); index > 0 {
		title := normalize(name[:index])
		author := normalizeAuthorSource(name[index+3:])
		if title != "" && likelyAuthor(author) &&
			(strings.Count(name, " - ") > 1 || (!looksLikeSeriesTitlePrefix(title) && !looksLikeReversedDash(title, author))) {
			result.Title = title
			result.Authors, result.AmbiguousAuthors = splitAuthors(author)
			result.Pattern = "final dash author"
			result.Confidence = "high"
			return finalize(result)
		}
	}

	lower := strings.ToLower(name)
	if index := strings.LastIndex(lower, " by "); index > 0 {
		title := normalize(name[:index])
		author := normalizeAuthorSource(name[index+4:])
		if title != "" && likelyAuthor(author) && !looksLikeTitlePhrase(title, author) {
			result.Title = title
			result.Authors, result.AmbiguousAuthors = splitAuthors(author)
			result.Pattern = "by author"
			result.Confidence = "high"
		}
	}
	return finalize(result)
}

func looksLikeTitlePhrase(title, author string) bool {
	titleWords := strings.Fields(strings.ToLower(title))
	authorWords := strings.Fields(strings.ToLower(author))
	if len(titleWords) == 0 || len(authorWords) == 0 {
		return false
	}
	if strings.Trim(titleWords[len(titleWords)-1], "\"'.,:;!?()[]{}") == strings.Trim(authorWords[0], "\"'.,:;!?()[]{}") {
		return true
	}
	if len(authorWords) == 1 {
		switch strings.Trim(authorWords[0], "\"'.,:;!?()[]{}") {
		case "me", "you", "us", "it", "them", "night", "day", "side", "sideways":
			return true
		}
	}
	return false
}

func finalize(result Analysis) Analysis {
	if SuspiciousTitle(result.Title) || result.AmbiguousAuthors || authorTextNeedsReview(result.Authors) ||
		strings.Contains(result.Title, "…") || strings.Contains(result.Title, "...") {
		result.Confidence = "ambiguous"
	}
	return result
}

func Stem(path string) string {
	name := filepath.Base(path)
	for {
		extension := strings.ToLower(filepath.Ext(name))
		if !recognizedExtensions[extension] {
			return normalize(name)
		}
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
}

func SuspiciousTitle(title string) bool {
	trimmed := strings.TrimSpace(title)
	lower := strings.ToLower(trimmed)
	if trimmed == "" || lower == "untitled" || lower == "unknown" || lower == "book" ||
		lower == "document" || lower == "test" || lower == "upload" {
		return true
	}
	if !hasLetterOrDigit(trimmed) || domainTitlePattern.MatchString(lower) || numericTitlePattern.MatchString(lower) ||
		hexTitlePattern.MatchString(lower) || asinTitlePattern.MatchString(lower) {
		return true
	}
	for _, extension := range suspiciousTitleExtensions {
		if strings.HasSuffix(lower, extension) {
			return true
		}
	}
	return false
}

func SuspiciousAuthors(authors []string, valid bool) bool {
	if !valid || len(authors) == 0 {
		return true
	}
	for _, author := range authors {
		if suspiciousAuthorValues[strings.ToLower(strings.TrimSpace(author))] || !hasLetterOrDigit(author) {
			return true
		}
	}
	return false
}

func trailingParenthetical(name string) (string, string, bool) {
	if !strings.HasSuffix(name, ")") {
		return "", "", false
	}
	depth := 0
	for index := len(name) - 1; index >= 0; index-- {
		switch name[index] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				title := normalize(name[:index])
				author := normalize(name[index+1 : len(name)-1])
				return title, author, title != "" && author != ""
			}
		}
	}
	return "", "", false
}

func likelyAuthor(value string) bool {
	value = normalize(value)
	if value == "" || len([]rune(value)) > 180 {
		return false
	}
	lower := strings.ToLower(value)
	role := strings.Trim(lower, " .()[]")
	if role == "auth" || role == "author" || role == "ed" || role == "eds" || role == "editor" || role == "editors" || suspiciousAuthorValues[role] {
		return false
	}
	for _, fragment := range []string{" edition", "book ", "volume ", "vol. ", "series", "anthology", "short story", "novel", "guide", "manual", "workbook", "collection"} {
		if strings.Contains(lower, fragment) {
			return false
		}
	}
	if len(strings.Fields(value)) > 18 {
		return false
	}
	for _, character := range value {
		if unicode.IsLetter(character) {
			return true
		}
	}
	return false
}

func likelyParentheticalAuthor(value string) bool {
	value = normalize(value)
	return !authorAcronymPattern.MatchString(value) && !authorYearPattern.MatchString(value) && likelyAuthor(value)
}

func looksLikeSeriesTitlePrefix(value string) bool {
	fields := strings.Fields(value)
	if len(fields) < 2 {
		return false
	}
	last := strings.Trim(fields[len(fields)-1], "#.()[]{}")
	if _, err := strconv.ParseFloat(last, 64); err == nil {
		return len(fields) <= 3 || strings.Contains(strings.ToLower(value), "series")
	}
	return false
}

func looksLikeReversedDash(left, right string) bool {
	leftWords := strings.Fields(left)
	rightWords := strings.Fields(strings.ToLower(right))
	if len(leftWords) == 0 || len(leftWords) > 4 || len(rightWords) == 0 {
		return false
	}
	switch strings.Trim(rightWords[0], "\"'([{_") {
	case "a", "an", "the":
		return true
	default:
		return false
	}
}

func splitAuthors(value string) ([]string, bool) {
	value = normalize(value)
	if strings.Contains(value, ",") {
		return []string{value}, true
	}
	parts := authorSplitPattern.Split(value, -1)
	authors := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = normalize(part); part != "" {
			authors = append(authors, part)
		}
	}
	return authors, false
}

func normalizeAuthorSource(value string) string {
	value = authorSourcePattern.ReplaceAllString(normalize(value), "")
	return normalize(duplicateSuffixPattern.ReplaceAllString(value, ""))
}

func authorTextNeedsReview(authors []string) bool {
	for _, author := range authors {
		if len(strings.Fields(author)) <= 4 {
			continue
		}
		lower := strings.ToLower(author)
		organization := false
		for _, marker := range []string{"agency", "committee", "institute", "organization", "press", "society", "team", "university"} {
			if strings.Contains(lower, marker) {
				organization = true
				break
			}
		}
		if !organization {
			return true
		}
	}
	return false
}

func hasLetterOrDigit(value string) bool {
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return true
		}
	}
	return false
}

func normalize(value string) string {
	return strings.TrimSpace(spacePattern.ReplaceAllString(value, " "))
}
