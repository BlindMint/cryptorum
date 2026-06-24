package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestMetadataDiagnosticsAllFailed(t *testing.T) {
	diagnostics := []MetadataProviderDiagnostic{
		{Provider: "google_books", Status: "failed", Error: "status 500"},
		{Provider: "open_library", Status: "failed", Error: "timeout"},
	}
	if !metadataDiagnosticsAllFailed(diagnostics) {
		t.Fatalf("expected all failed diagnostics to be detected")
	}

	diagnostics = append(diagnostics, MetadataProviderDiagnostic{Provider: "bookbrainz", Status: "zero_results"})
	if metadataDiagnosticsAllFailed(diagnostics) {
		t.Fatalf("expected zero-result provider to prevent all-failed classification")
	}
}

func TestMetadataDiagnosticsSummary(t *testing.T) {
	failed := metadataDiagnosticsSummary([]MetadataProviderDiagnostic{
		{Provider: "google_books", Status: "failed", Error: "status 500"},
		{Provider: "open_library", Status: "failed", Error: "timeout"},
	})
	if failed != "Metadata providers failed: Google Books: status 500; Open Library: timeout" {
		t.Fatalf("unexpected failure summary: %q", failed)
	}

	zero := metadataDiagnosticsSummary([]MetadataProviderDiagnostic{
		{Provider: "google_books", Status: "zero_results"},
		{Provider: "open_library", Status: "zero_results"},
	})
	if zero != "Metadata providers responded but returned no results." {
		t.Fatalf("unexpected zero-result summary: %q", zero)
	}
}

func TestWriteMetadataSearchResponseCompatibility(t *testing.T) {
	candidates := []MetadataCandidate{{Provider: "open_library", Title: "Dune"}}
	diagnostics := []MetadataProviderDiagnostic{{Provider: "open_library", Status: "ok", Count: 1}}

	arrayRecorder := httptest.NewRecorder()
	writeMetadataSearchResponse(arrayRecorder, candidates, diagnostics, false)
	if arrayRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", arrayRecorder.Code)
	}
	var arrayPayload []MetadataCandidate
	if err := json.NewDecoder(arrayRecorder.Body).Decode(&arrayPayload); err != nil {
		t.Fatalf("expected default response to be an array: %v", err)
	}
	if len(arrayPayload) != 1 || arrayPayload[0].Title != "Dune" {
		t.Fatalf("unexpected array payload: %+v", arrayPayload)
	}

	objectRecorder := httptest.NewRecorder()
	writeMetadataSearchResponse(objectRecorder, candidates, diagnostics, true)
	if objectRecorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", objectRecorder.Code)
	}
	var objectPayload MetadataSearchResponse
	if err := json.NewDecoder(objectRecorder.Body).Decode(&objectPayload); err != nil {
		t.Fatalf("expected diagnostics response to be an object: %v", err)
	}
	if len(objectPayload.Results) != 1 || len(objectPayload.Diagnostics) != 1 {
		t.Fatalf("unexpected object payload: %+v", objectPayload)
	}
}
