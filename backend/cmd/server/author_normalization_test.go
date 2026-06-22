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

func TestAuthorCredentialSuffixVariantsMatch(t *testing.T) {
	inputs := []string{
		"E. D. Steward, Ph.D.",
		"E.D. Steward PhD",
		"E D Steward, PHD",
		"E. D. Steward Ph. D.",
	}
	want := "edstewardphd"
	for _, input := range inputs {
		if got := normalizedAuthorMatchKey(input); got != want {
			t.Fatalf("normalizedAuthorMatchKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAuthorCredentialSuffixOmissionDoesNotMatch(t *testing.T) {
	if authorNamesMatch("E. D. Steward", "E. D. Steward, Ph.D.") {
		t.Fatal("expected credential-bearing and credential-omitted names to remain distinct")
	}
}

func TestCanonicalAuthorDisplayFormatsCredentialSuffixes(t *testing.T) {
	tests := map[string]string{
		"E.D. Steward PhD":         "E. D. Steward, Ph.D.",
		"E. D. Steward, Ph.D.":     "E. D. Steward, Ph.D.",
		"Steward, E.D., PhD":       "E. D. Steward, Ph.D.",
		"Dr E.D. Steward PhD":      "Dr. E. D. Steward, Ph.D.",
		"Jane Smith MD":            "Jane Smith, M.D.",
		"Jane Smith, M.D., Ph.D.":  "Jane Smith, M.D., Ph.D.",
		"Professor Jane Smith PhD": "Prof. Jane Smith, Ph.D.",
	}

	for input, want := range tests {
		if got := canonicalAuthorDisplay(input); got != want {
			t.Fatalf("canonicalAuthorDisplay(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCanonicalAuthorDisplayDoesNotTreatTitleCaseShortWordsAsCredentials(t *testing.T) {
	tests := map[string]string{
		"Jane Ma":         "Jane Ma",
		"Jane Smith Ma":   "Jane Smith Ma",
		"Jane Smith MA":   "Jane Smith, M.A.",
		"Jane Smith M.A.": "Jane Smith, M.A.",
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
