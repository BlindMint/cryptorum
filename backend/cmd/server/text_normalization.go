package main

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func stripDiacritics(value string) string {
	if value == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range norm.NFD.String(value) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}
