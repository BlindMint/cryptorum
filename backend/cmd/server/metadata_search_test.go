package main

import "testing"

func TestMetadataSearchVariantsIncludeStrictISBNSearch(t *testing.T) {
	variants := metadataSearchVariants(MetadataSearchFields{
		Title:  "Warriors, Warlords and Saints",
		Author: "John Hunt",
		ISBN:   "978-1-905036-32-5",
		Strict: true,
	})

	if len(variants) < 2 {
		t.Fatalf("expected generic and ISBN variants, got %d", len(variants))
	}

	var foundISBN bool
	for _, variant := range variants {
		if variant.Mode == metadataSearchModeISBN && variant.Query == "9781905036325" {
			foundISBN = true
			if variant.Fields.Title != "" || variant.Fields.Author != "" {
				t.Fatalf("ISBN variant should score by ISBN only, got fields %+v", variant.Fields)
			}
		}
	}
	if !foundISBN {
		t.Fatalf("expected ISBN search variant, got %+v", variants)
	}
}

func TestPreferredMetadataISBNPrefersISBN13(t *testing.T) {
	got := preferredMetadataISBN([]string{"1905036326", "9781905036325"})
	if got != "9781905036325" {
		t.Fatalf("preferredMetadataISBN returned %q", got)
	}
}
