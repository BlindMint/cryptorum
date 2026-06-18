package coverprefs

import "strings"

const (
	ComicSpreadFallbackInherit  = "inherit"
	ComicSpreadFallbackRight    = "right"
	ComicSpreadFallbackLeft     = "left"
	ComicSpreadFallbackDisabled = "disabled"
)

// NormalizeComicSpreadFallback returns a supported comic spread fallback value.
func NormalizeComicSpreadFallback(value string, allowInherit bool) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ComicSpreadFallbackRight:
		return ComicSpreadFallbackRight
	case ComicSpreadFallbackLeft:
		return ComicSpreadFallbackLeft
	case ComicSpreadFallbackDisabled:
		return ComicSpreadFallbackDisabled
	case ComicSpreadFallbackInherit:
		if allowInherit {
			return ComicSpreadFallbackInherit
		}
	}

	if allowInherit {
		return ComicSpreadFallbackInherit
	}
	return ComicSpreadFallbackRight
}

// ResolveComicSpreadFallback applies book > library > global precedence.
func ResolveComicSpreadFallback(bookValue, libraryValue, globalValue string) string {
	if value := NormalizeComicSpreadFallback(bookValue, true); value != ComicSpreadFallbackInherit {
		return value
	}
	if value := NormalizeComicSpreadFallback(libraryValue, true); value != ComicSpreadFallbackInherit {
		return value
	}
	return NormalizeComicSpreadFallback(globalValue, false)
}
