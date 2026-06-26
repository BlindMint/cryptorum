package main

import (
	"testing"

	"cryptorum/internal/db"
)

func TestNormalizedAuthorMatchKeyTreatsInitialVariantsAsEquivalent(t *testing.T) {
	inputs := []string{"J. P. Cooper", "J.P. Cooper", "JP Cooper", "J P Cooper"}
	want := "jpcooper"
	for _, input := range inputs {
		if got := normalizedAuthorMatchKey(input); got != want {
			t.Fatalf("normalizedAuthorMatchKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizedAuthorMatchKeyRemovesDiacritics(t *testing.T) {
	left := normalizedAuthorMatchKey("A. Freitas-Magalhães")
	right := normalizedAuthorMatchKey("A. Freitas-Magalhaes")
	if left == "" || left != right {
		t.Fatalf("expected accented and unaccented author keys to match, got %q and %q", left, right)
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

func TestNormalizedAuthorSQLExpressionRemovesDisplayPunctuation(t *testing.T) {
	previousDB := appDB
	testDB, err := db.New(t.TempDir())
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	appDB = testDB
	t.Cleanup(func() {
		_ = testDB.Close()
		appDB = previousDB
	})

	expression := normalizedAuthorSQLExpression(sqlStringLiteral("Advanced Web Attacks and Exploitation (AWAE)"))
	var got string
	if err := appDB.QueryRow("SELECT " + expression).Scan(&got); err != nil {
		t.Fatalf("evaluate normalized author SQL expression: %v", err)
	}
	if want := normalizedAuthorMatchKey("Advanced Web Attacks and Exploitation (AWAE)"); got != want {
		t.Fatalf("SQL normalized author = %q, want %q", got, want)
	}
}

func TestNormalizedAuthorSQLExpressionRemovesUnicodeEllipsis(t *testing.T) {
	previousDB := appDB
	testDB, err := db.New(t.TempDir())
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	appDB = testDB
	t.Cleanup(func() {
		_ = testDB.Close()
		appDB = previousDB
	})

	author := "… Fifth World Congress on Disaster Management Volume V. Proceedings of the International Conference on Disaster Management"
	expression := normalizedAuthorSQLExpression(sqlStringLiteral(author))
	var got string
	if err := appDB.QueryRow("SELECT " + expression).Scan(&got); err != nil {
		t.Fatalf("evaluate normalized author SQL expression: %v", err)
	}
	if want := normalizedAuthorMatchKey(author); got != want {
		t.Fatalf("SQL normalized author = %q, want %q", got, want)
	}
}

func TestNormalizedAuthorSQLExpressionMatchesGoKeyForProblematicImportedValues(t *testing.T) {
	previousDB := appDB
	testDB, err := db.New(t.TempDir())
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	appDB = testDB
	t.Cleanup(func() {
		_ = testDB.Close()
		appDB = previousDB
	})

	authors := []string{
		"†Ä@_",
		"©2004 Bruce R. Cordell",
		"(Imported Author",
		")Imported Author",
		"真的老狼 Zhen De Lao Lang",
	}

	for _, author := range authors {
		expression := normalizedAuthorSQLExpression(sqlStringLiteral(author))
		var got string
		if err := appDB.QueryRow("SELECT " + expression).Scan(&got); err != nil {
			t.Fatalf("evaluate normalized author SQL expression for %q: %v", author, err)
		}
		if want := normalizedAuthorMatchKey(author); got != want {
			t.Fatalf("SQL normalized author for %q = %q, want %q", author, got, want)
		}
	}
}

func TestAuthorMetadataOptionKeepsRawFilterValue(t *testing.T) {
	rawAuthor := "… Fifth World Congress on Disaster Management Volume V. Proceedings of the International Conference on Disaster Management"
	counts := map[string]*authorMetadataOption{}
	seen := map[string]bool{}

	addAuthorMetadataOption(counts, seen, rawAuthor, 1)
	options := sortedAuthorMetadataOptions(counts)

	if len(options) != 1 {
		t.Fatalf("expected one author option, got %d", len(options))
	}
	if options[0].Value != rawAuthor {
		t.Fatalf("author option value = %q, want raw value %q", options[0].Value, rawAuthor)
	}
	if options[0].Name == "" {
		t.Fatal("expected author option display name to be populated")
	}
}
