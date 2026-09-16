package filenameinfo

import "testing"

func TestAnalyze(t *testing.T) {
	tests := []struct {
		path, title, pattern, confidence string
		authors                          []string
	}{
		{"Secret History (Craig P. Bauer).pdf", "Secret History", "trailing parenthetical author", "medium", []string{"Craig P. Bauer"}},
		{"Black Hat Python by Justin Seitz.pdf", "Black Hat Python", "by author", "high", []string{"Justin Seitz"}},
		{"Political Ideologies - Andrew Heywood (Z-Library).epub", "Political Ideologies", "final dash author", "high", []string{"Andrew Heywood"}},
		{"Mastering Azure Kubernetes Service (AKS).epub", "Mastering Azure Kubernetes Service (AKS)", "filename title", "medium", nil},
		{"01 - Black Mass - vx-underground (2023).pdf", "01 - Black Mass", "final dash author", "high", []string{"vx-underground (2023)"}},
		{"Particle Physics - Brian Martin, Graham Shaw.pdf", "Particle Physics", "final dash author", "ambiguous", []string{"Brian Martin, Graham Shaw"}},
		{"11. Star by Star.mobi", "11. Star by Star", "filename title", "medium", nil},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			got := Analyze(test.path)
			if got.Title != test.title || got.Pattern != test.pattern || got.Confidence != test.confidence || !sameAuthors(got.Authors, test.authors) {
				t.Fatalf("Analyze(%q) = %+v", test.path, got)
			}
		})
	}
}

func TestSuspiciousMetadata(t *testing.T) {
	for _, title := range []string{"", "Unknown", "509564106", "B0C46X8YKT", "file.pdf", "bingdian001.com", "()"} {
		if !SuspiciousTitle(title) {
			t.Errorf("SuspiciousTitle(%q) = false", title)
		}
	}
	if SuspiciousTitle("A Useful Book") {
		t.Error("useful title marked suspicious")
	}
	if !SuspiciousAuthors([]string{"Zamzar"}, true) || SuspiciousAuthors([]string{"Ada Lovelace"}, true) {
		t.Error("unexpected author quality classification")
	}
}

func sameAuthors(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
