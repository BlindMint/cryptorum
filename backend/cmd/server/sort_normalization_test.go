package main

import (
	"strings"
	"testing"
)

func TestBookListOrderByUsesNormalizedTitleSort(t *testing.T) {
	orderBy := bookListOrderBy("title", "asc")
	if !strings.Contains(orderBy, "LTRIM(COALESCE(bm.title, '')") {
		t.Fatalf("expected title order to use normalized title expression, got %q", orderBy)
	}
	if !strings.Contains(orderBy, "REPLACE(") || !strings.Contains(orderBy, "'Ε', 'E'") || !strings.Contains(orderBy, "'Н', 'H'") {
		t.Fatalf("expected title order to normalize Latin-confusable characters, got %q", orderBy)
	}
}

func TestBookListOrderByUsesNormalizedTieBreakers(t *testing.T) {
	tests := []struct {
		name   string
		sortBy string
		want   string
	}{
		{name: "authors", sortBy: "authors", want: "REPLACE(REPLACE(REPLACE(COALESCE(bm.authors, '')"},
		{name: "series", sortBy: "series", want: "LTRIM(COALESCE(bm.series, '')"},
		{name: "last_read", sortBy: "last_read", want: "LTRIM(COALESCE(bm.title, '')"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orderBy := bookListOrderBy(tt.sortBy, "asc")
			if !strings.Contains(orderBy, tt.want) {
				t.Fatalf("expected %s order to include %q, got %q", tt.sortBy, tt.want, orderBy)
			}
		})
	}
}
