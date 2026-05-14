package scanner

import (
	"testing"

	"cryptorum/internal/metadata"
)

func TestMetadataWithFilenameTitleFallback(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		path     string
		expected string
	}{
		{
			name:     "empty title uses filename",
			title:    "",
			path:     "/books/Library/Useful Book.pdf",
			expected: "Useful Book",
		},
		{
			name:     "untitled title uses filename",
			title:    "Untitled",
			path:     "/books/Comics/Series 01 - The Beginning.cbz",
			expected: "The Beginning",
		},
		{
			name:     "existing useful title is preserved",
			title:    "Existing Title",
			path:     "/books/Other Name.epub",
			expected: "Existing Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := metadataWithFilenameTitleFallback(&metadata.BookMetadata{Title: tt.title}, tt.path)
			if meta.Title != tt.expected {
				t.Fatalf("Title = %q, want %q", meta.Title, tt.expected)
			}
		})
	}
}
