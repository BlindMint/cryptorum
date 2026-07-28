package metaprotection

import (
	"reflect"
	"testing"
)

func TestLockedFieldsAreNormalizedAndMerged(t *testing.T) {
	raw := `["genres","cover_path","title","title","unknown"]`
	merged := MergeLocked(raw, "authors", "cover_source")
	got := NormalizeFields(keys(ParseLocked(merged)))
	want := []string{"authors", "cover", "tags", "title"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("locked fields = %#v, want %#v", got, want)
	}

	removed := RemoveLocked(merged, "cover", "genres")
	got = NormalizeFields(keys(ParseLocked(removed)))
	want = []string{"authors", "title"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining locked fields = %#v, want %#v", got, want)
	}
}

func keys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return result
}
