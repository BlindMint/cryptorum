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

func TestBuildFTSQueryUsesPrefixAndAndSemantics(t *testing.T) {
	query := buildFTSQuery(searchTokens("Hacking Connected Cars"))
	want := "hacking* AND connected* AND cars*"
	if query != want {
		t.Fatalf("expected %q, got %q", want, query)
	}
}
