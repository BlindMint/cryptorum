package main

import "testing"

func TestScoreBookSearchCandidateWordBasedQueries(t *testing.T) {
	candidate := bookSearchCandidate{
		result: SearchResult{
			ID:      1,
			Title:   "Hacking Connected Cars",
			Authors: `["Example Author"]`,
		},
	}

	tests := []string{
		"hacking cars",
		"connect cars",
		"connected car",
	}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			tokens := searchTokens(query)
			score := scoreBookSearchCandidate(tokens, candidate)
			if score < minBookSearchScore {
				t.Fatalf("expected %q to match, score %.2f below threshold %.2f", query, score, minBookSearchScore)
			}
		})
	}
}

func TestScoreBookSearchCandidateTypos(t *testing.T) {
	candidate := bookSearchCandidate{
		result: SearchResult{
			ID:      1,
			Title:   "Hacking Connected Cars",
			Authors: `["Example Author"]`,
		},
	}

	tokens := searchTokens("hacjing card")
	score := scoreBookSearchCandidate(tokens, candidate)
	if score < minBookSearchScore {
		t.Fatalf("expected typo query to match, score %.2f below threshold %.2f", score, minBookSearchScore)
	}
}

func TestSearchTokensNormalizeDiacritics(t *testing.T) {
	tests := map[string][]string{
		"A. Freitas-Magalhães": {"freitas", "magalhaes"},
		"São Paulo Stories":    {"sao", "paulo", "stories"},
	}

	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got := searchTokens(input)
			if len(got) != len(want) {
				t.Fatalf("searchTokens(%q) = %#v, want %#v", input, got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("searchTokens(%q) = %#v, want %#v", input, got, want)
				}
			}
		})
	}
}

func TestScoreBookSearchCandidateMatchesUnaccentedQueryAcrossFields(t *testing.T) {
	tokens := searchTokens("freitas magalhaes ciencia")
	candidate := bookSearchCandidate{
		result: SearchResult{
			ID:      1,
			Title:   "Ciência Emocional",
			Authors: `["A. Freitas-Magalhães"]`,
		},
	}

	score := scoreBookSearchCandidate(tokens, candidate)
	if score < minBookSearchScore {
		t.Fatalf("expected unaccented query to match accented metadata, score %.2f below threshold %.2f", score, minBookSearchScore)
	}
}

func TestScoreBookSearchCandidateMatchesOffensiveExamples(t *testing.T) {
	candidate := bookSearchCandidate{
		result: SearchResult{
			ID:       1,
			Title:    "Offensive",
			Authors:  `["Will Crudge"]`,
			Series:   "Starfleet Nemesis",
			Format:   "epub",
			FilePath: "Will Crudge/Offensive/Starfleet Nemesis Offensive Book 1 - Will Crudge.epub",
		},
	}

	tests := []string{
		"offensive",
		"offensive starfleet nemesis",
		"offensive will",
		"offensive starfleet",
		"offensive crudge",
		"starfleet nemesis",
	}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			tokens := searchTokens(query)
			score := scoreBookSearchCandidate(tokens, candidate)
			if score < minBookSearchScore {
				t.Fatalf("expected %q to match, score %.2f below threshold %.2f", query, score, minBookSearchScore)
			}
		})
	}
}

func TestScoreBookSearchCandidateRejectsUnrelatedPartialCoverage(t *testing.T) {
	candidate := bookSearchCandidate{
		result: SearchResult{
			ID:      1,
			Title:   "Hacking Connected Cars",
			Authors: `["Example Author"]`,
		},
	}

	tokens := searchTokens("hacking bread")
	score := scoreBookSearchCandidate(tokens, candidate)
	if score != 0 {
		t.Fatalf("expected partial unrelated two-token query to be rejected, got score %.2f", score)
	}
}

func TestDefaultSearchSortIsRelevance(t *testing.T) {
	scored := []scoredBookSearchResult{
		{
			result: SearchResult{ID: 1, Title: "A Broad Match"},
			score:  1.6,
		},
		{
			result: SearchResult{ID: 2, Title: "Offensive"},
			score:  8.0,
		},
	}

	sortScoredBookSearchResults(scored, "", "")
	if got := scored[0].result.ID; got != 2 {
		t.Fatalf("expected highest relevance result first, got ID %d", got)
	}
}

func TestSearchTitleSortIgnoresLeadingPunctuation(t *testing.T) {
	scored := []scoredBookSearchResult{
		{result: SearchResult{ID: 1, Title: "Zoo"}},
		{result: SearchResult{ID: 2, Title: "'Salem's Lot"}},
		{result: SearchResult{ID: 3, Title: `"Trickle Down Theory" and "Tax Cuts for the Rich"`}},
		{result: SearchResult{ID: 4, Title: "Apple"}},
	}

	sortScoredBookSearchResults(scored, "title", "asc")

	got := []int64{scored[0].result.ID, scored[1].result.ID, scored[2].result.ID, scored[3].result.ID}
	want := []int64{4, 2, 3, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("title sort order = %#v, want %#v", got, want)
		}
	}
}

func TestNormalizedSortKeyIgnoresLeadingPunctuationOnly(t *testing.T) {
	tests := map[string]string{
		"'Salem's Lot": "salem's lot",
		`"Trickle Down Theory" and "Tax Cuts for the Rich"`: `trickle down theory" and "tax cuts for the rich"`,
		"  --Alpha": "alpha",
		"Εxample":   "example",
		"Нacking":   "hacking",
	}

	for input, want := range tests {
		if got := normalizedSortKey(input); got != want {
			t.Fatalf("normalizedSortKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeSearchPagination(t *testing.T) {
	tests := []struct {
		name       string
		offset     int
		limit      int
		wantOffset int
		wantLimit  int
	}{
		{name: "defaults bad values", offset: -10, limit: 0, wantOffset: 0, wantLimit: searchDefaultPageLimit},
		{name: "keeps smaller page", offset: 75, limit: 25, wantOffset: 75, wantLimit: 25},
		{name: "caps page size", offset: 50, limit: 500, wantOffset: 50, wantLimit: searchDefaultPageLimit},
		{name: "caps offset", offset: searchMaxResults + 25, limit: 50, wantOffset: searchMaxResults, wantLimit: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOffset, gotLimit := normalizeSearchPagination(tt.offset, tt.limit)
			if gotOffset != tt.wantOffset || gotLimit != tt.wantLimit {
				t.Fatalf("normalizeSearchPagination(%d, %d) = (%d, %d), want (%d, %d)",
					tt.offset, tt.limit, gotOffset, gotLimit, tt.wantOffset, tt.wantLimit)
			}
		})
	}
}

func TestBuildFTSQueryUsesPrefixAndAndSemantics(t *testing.T) {
	query := buildFTSQuery(searchTokens("Hacking Connected Cars"))
	want := "hacking* AND connected* AND cars*"
	if query != want {
		t.Fatalf("expected %q, got %q", want, query)
	}
}
