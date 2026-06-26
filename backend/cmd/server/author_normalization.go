package main

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"unicode"

	"modernc.org/sqlite"
)

var authorCredentialSuffixes = map[string]string{
	"phd":   "Ph.D.",
	"md":    "M.D.",
	"do":    "D.O.",
	"jd":    "J.D.",
	"dds":   "D.D.S.",
	"dmd":   "D.M.D.",
	"dvm":   "D.V.M.",
	"edd":   "Ed.D.",
	"psyd":  "Psy.D.",
	"scd":   "Sc.D.",
	"dphil": "DPhil",
	"dmin":  "D.Min.",
	"thd":   "Th.D.",
	"rn":    "RN",
	"np":    "NP",
	"pa":    "PA",
	"cpa":   "CPA",
	"pe":    "P.E.",
	"mba":   "MBA",
	"ma":    "M.A.",
	"ms":    "M.S.",
	"mfa":   "M.F.A.",
	"mph":   "M.P.H.",
	"lcsw":  "LCSW",
	"lpc":   "LPC",
	"lmft":  "LMFT",
}

var authorCredentialSuffixesAllowLowercase = map[string]struct{}{
	"phd":   {},
	"edd":   {},
	"psyd":  {},
	"scd":   {},
	"dphil": {},
	"dmin":  {},
	"thd":   {},
	"mba":   {},
	"mfa":   {},
	"mph":   {},
	"lcsw":  {},
	"lpc":   {},
	"lmft":  {},
}

var authorTitlePrefixes = map[string]string{
	"dr":        "Dr.",
	"doctor":    "Dr.",
	"prof":      "Prof.",
	"professor": "Prof.",
	"rev":       "Rev.",
	"reverend":  "Rev.",
	"fr":        "Fr.",
	"father":    "Fr.",
	"sr":        "Sr.",
	"sister":    "Sr.",
	"br":        "Br.",
	"brother":   "Br.",
	"rabbi":     "Rabbi",
	"imam":      "Imam",
	"sheikh":    "Sheikh",
	"sir":       "Sir",
	"dame":      "Dame",
}

func init() {
	sqlite.MustRegisterDeterministicScalarFunction(
		"cryptorum_author_match_key",
		1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if len(args) == 0 || args[0] == nil {
				return "", nil
			}
			switch value := args[0].(type) {
			case string:
				return normalizedAuthorMatchKey(value), nil
			case []byte:
				return normalizedAuthorMatchKey(string(value)), nil
			default:
				return normalizedAuthorMatchKey(strings.TrimSpace(fmt.Sprint(value))), nil
			}
		},
	)
}

func normalizedAuthorMatchKey(author string) string {
	author = canonicalAuthorDisplay(author)
	var builder strings.Builder
	for _, r := range strings.ToLower(stripDiacritics(strings.TrimSpace(author))) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func canonicalAuthorDisplay(author string) string {
	author = strings.Join(strings.Fields(strings.TrimSpace(author)), " ")
	if author == "" {
		return ""
	}

	namePart, suffixes := splitAuthorCredentialSuffixes(author)
	namePart = normalizeInvertedAuthorName(namePart)
	fields := strings.Fields(strings.ReplaceAll(namePart, ".", " "))
	parts := make([]string, 0, len(fields)+2)
	for i, field := range fields {
		field = strings.Trim(field, ",")
		if field == "" {
			continue
		}
		if i == 0 {
			if prefix, ok := authorTitlePrefixes[normalizedAuthorToken(field)]; ok {
				parts = append(parts, prefix)
				continue
			}
		}
		if isSingleInitial(field) {
			parts = append(parts, strings.ToUpper(field)+".")
			continue
		}
		if i == 0 && isInitialRun(field) {
			for _, r := range field {
				parts = append(parts, strings.ToUpper(string(r))+".")
			}
			continue
		}
		parts = append(parts, field)
	}

	display := strings.Join(parts, " ")
	if len(suffixes) > 0 {
		if display != "" {
			display += ", "
		}
		display += strings.Join(suffixes, ", ")
	}
	return display
}

func canonicalAuthorOptionName(author string) string {
	display := canonicalAuthorDisplay(author)
	if display != "" {
		return display
	}
	return strings.TrimSpace(author)
}

func normalizedAuthorSQLExpression(valueExpression string) string {
	return "cryptorum_author_match_key(" + valueExpression + ")"
}

func authorFilterShouldUseExactValue(author string) bool {
	trimmed := strings.TrimSpace(author)
	if trimmed == "" {
		return true
	}
	for _, r := range trimmed {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return true
		}
		break
	}
	return len([]rune(normalizedAuthorMatchKey(trimmed))) < 2
}

func authorMetadataOptionKey(author string) string {
	if authorFilterShouldUseExactValue(author) {
		return "exact:" + strings.TrimSpace(author)
	}
	return "normalized:" + normalizedAuthorMatchKey(author)
}

func addAuthorFilterCondition(addFilterCondition func(string, ...interface{}), column string, author string) {
	rawAuthor := strings.TrimSpace(author)
	key := normalizedAuthorMatchKey(author)
	if rawAuthor == "" || key == "" {
		return
	}
	if authorFilterShouldUseExactValue(rawAuthor) {
		addFilterCondition(
			`EXISTS (SELECT 1 FROM json_each(COALESCE(`+column+`, '[]')) WHERE value = ?)`,
			rawAuthor,
		)
		return
	}
	addFilterCondition(
		`EXISTS (SELECT 1 FROM json_each(COALESCE(`+column+`, '[]')) WHERE value = ? OR `+normalizedAuthorSQLExpression("value")+` = ?)`,
		rawAuthor,
		key,
	)
}

func authorNamesMatch(left, right string) bool {
	leftKey := normalizedAuthorMatchKey(left)
	return leftKey != "" && leftKey == normalizedAuthorMatchKey(right)
}

func splitAuthorCredentialSuffixes(author string) (string, []string) {
	commaParts := splitAuthorCommaParts(author)
	if len(commaParts) > 1 {
		suffixes := []string{}
		remaining := append([]string(nil), commaParts...)
		for len(remaining) > 1 {
			suffix, ok := canonicalAuthorCredential(remaining[len(remaining)-1])
			if !ok {
				break
			}
			suffixes = append([]string{suffix}, suffixes...)
			remaining = remaining[:len(remaining)-1]
		}
		if len(suffixes) > 0 {
			return strings.Join(remaining, ", "), suffixes
		}
		return author, nil
	}

	fields := strings.Fields(author)
	suffixes := []string{}
	for len(fields) > 0 {
		index, suffix, ok := trailingAuthorCredential(fields)
		if !ok {
			break
		}
		suffixes = append([]string{suffix}, suffixes...)
		fields = fields[:index]
	}
	return strings.Join(fields, " "), suffixes
}

func splitAuthorCommaParts(author string) []string {
	rawParts := strings.Split(author, ",")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func normalizeInvertedAuthorName(author string) string {
	parts := splitAuthorCommaParts(author)
	if len(parts) != 2 {
		return author
	}
	if _, ok := canonicalAuthorCredential(parts[1]); ok {
		return author
	}
	return strings.TrimSpace(parts[1] + " " + parts[0])
}

func trailingAuthorCredential(fields []string) (int, string, bool) {
	maxWidth := 3
	if len(fields) < maxWidth {
		maxWidth = len(fields)
	}
	for width := maxWidth; width >= 1; width-- {
		start := len(fields) - width
		if suffix, ok := canonicalAuthorCredential(strings.Join(fields[start:], "")); ok {
			return start, suffix, true
		}
	}
	return len(fields), "", false
}

func canonicalAuthorCredential(value string) (string, bool) {
	token := normalizedAuthorToken(value)
	suffix, ok := authorCredentialSuffixes[token]
	if !ok {
		return "", false
	}
	if !authorCredentialLooksExplicit(value, token) {
		return "", false
	}
	return suffix, ok
}

func authorCredentialLooksExplicit(value string, token string) bool {
	if _, ok := authorCredentialSuffixesAllowLowercase[token]; ok {
		return true
	}
	hasLetter := false
	allLettersUpper := true
	hasDot := strings.Contains(value, ".")
	for _, r := range value {
		if !unicode.IsLetter(r) {
			continue
		}
		hasLetter = true
		if !unicode.IsUpper(r) {
			allLettersUpper = false
		}
	}
	return hasDot || (hasLetter && allLettersUpper)
}

func normalizedAuthorToken(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func isSingleInitial(value string) bool {
	runes := []rune(value)
	return len(runes) == 1 && unicode.IsLetter(runes[0])
}

func isInitialRun(value string) bool {
	runes := []rune(value)
	if len(runes) < 2 || len(runes) > 4 {
		return false
	}
	for _, r := range runes {
		if !unicode.IsLetter(r) || !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}
