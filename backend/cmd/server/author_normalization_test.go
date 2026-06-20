package main

import "testing"

func TestNormalizedAuthorMatchKeyTreatsInitialVariantsAsEquivalent(t *testing.T) {
	inputs := []string{"J. P. Cooper", "J.P. Cooper", "JP Cooper", "J P Cooper"}
	want := "jpcooper"
	for _, input := range inputs {
		if got := normalizedAuthorMatchKey(input); got != want {
			t.Fatalf("normalizedAuthorMatchKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCanonicalAuthorDisplayFormatsInitials(t *testing.T) {
	tests := map[string]string{
		"J. P. Cooper": "J. P. Cooper",
		"J.P. Cooper":  "J. P. Cooper",
		"JP Cooper":    "J. P. Cooper",
		"J P Cooper":   "J. P. Cooper",
		"JRR Tolkien":  "J. R. R. Tolkien",
	}

	for input, want := range tests {
		if got := canonicalAuthorDisplay(input); got != want {
			t.Fatalf("canonicalAuthorDisplay(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAuthorNamesMatch(t *testing.T) {
	if !authorNamesMatch("J.P. Cooper", "J P Cooper") {
		t.Fatal("expected initial variants to match")
	}
	if authorNamesMatch("J.P. Cooper", "Jane Cooper") {
		t.Fatal("expected distinct names not to match")
	}
}
