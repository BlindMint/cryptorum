package main

import (
	"reflect"
	"testing"
)

func TestNormalizeMetadataTagListSortsAndDeduplicates(t *testing.T) {
	got := normalizeMetadataTagList([]string{"zeta", " Alpha ", "beta", "alpha", "", "Beta"})
	want := []string{"Alpha", "beta", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeMetadataTagList() = %#v, want %#v", got, want)
	}
}

func TestNormalizeMetadataStringListPreservesOrder(t *testing.T) {
	got := normalizeMetadataStringList([]string{"Warhammer", " warhammer ", "Science Fiction"})
	want := []string{"Warhammer", "Science Fiction"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeMetadataStringList() = %#v, want %#v", got, want)
	}
}
