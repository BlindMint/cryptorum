package main

import (
	"strings"
	"unicode"
)

func normalizedAuthorMatchKey(author string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(author)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func canonicalAuthorDisplay(author string) string {
	author = strings.TrimSpace(strings.ReplaceAll(author, ".", " "))
	if author == "" {
		return ""
	}

	fields := strings.Fields(author)
	parts := make([]string, 0, len(fields)+2)
	for i, field := range fields {
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
	return strings.Join(parts, " ")
}

func canonicalAuthorOptionName(author string) string {
	display := canonicalAuthorDisplay(author)
	if display != "" {
		return display
	}
	return strings.TrimSpace(author)
}

func normalizedAuthorSQLExpression(valueExpression string) string {
	return "LOWER(REPLACE(REPLACE(" + valueExpression + ", '.', ''), ' ', ''))"
}

func addAuthorFilterCondition(addFilterCondition func(string, ...interface{}), column string, author string) {
	key := normalizedAuthorMatchKey(author)
	if key == "" {
		return
	}
	addFilterCondition(
		`EXISTS (SELECT 1 FROM json_each(COALESCE(`+column+`, '[]')) WHERE `+normalizedAuthorSQLExpression("value")+` = ?)`,
		key,
	)
}

func authorNamesMatch(left, right string) bool {
	leftKey := normalizedAuthorMatchKey(left)
	return leftKey != "" && leftKey == normalizedAuthorMatchKey(right)
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
