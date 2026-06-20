package seriesnum

import (
	"encoding/json"
	"testing"
)

func TestParseRomanNumerals(t *testing.T) {
	tests := []struct {
		input   string
		number  float64
		display string
	}{
		{"I", 1, "I"},
		{"ii", 2, "II"},
		{"IV", 4, "IV"},
		{"IX", 9, "IX"},
		{"XL", 40, "XL"},
		{"XC", 90, "XC"},
		{"CD", 400, "CD"},
		{"CM", 900, "CM"},
		{"MCMXCIV", 1994, "MCMXCIV"},
	}

	for _, test := range tests {
		number, display, err := Parse(test.input)
		if err != nil {
			t.Fatalf("Parse(%q) returned error: %v", test.input, err)
		}
		if number != test.number || display != test.display {
			t.Fatalf("Parse(%q) = %v, %q; want %v, %q", test.input, number, display, test.number, test.display)
		}
	}
}

func TestParseNumericValues(t *testing.T) {
	tests := []struct {
		input  string
		number float64
	}{
		{"4", 4},
		{"04", 4},
		{"4.5", 4.5},
	}

	for _, test := range tests {
		number, display, err := Parse(test.input)
		if err != nil {
			t.Fatalf("Parse(%q) returned error: %v", test.input, err)
		}
		if number != test.number || display != "" {
			t.Fatalf("Parse(%q) = %v, %q; want %v, empty display", test.input, number, display, test.number)
		}
	}
}

func TestParseRejectsInvalidRomanNumerals(t *testing.T) {
	for _, input := range []string{"IIII", "VX", "Volume II", "A", "-1"} {
		if _, _, err := Parse(input); err == nil {
			t.Fatalf("Parse(%q) succeeded, want error", input)
		}
	}
}

func TestParseJSONAcceptsNumberAndString(t *testing.T) {
	tests := []struct {
		raw     string
		number  float64
		display string
	}{
		{`7`, 7, ""},
		{`"VII"`, 7, "VII"},
		{`""`, 0, ""},
		{`null`, 0, ""},
	}

	for _, test := range tests {
		number, display, err := ParseJSON(json.RawMessage(test.raw))
		if err != nil {
			t.Fatalf("ParseJSON(%s) returned error: %v", test.raw, err)
		}
		if number != test.number || display != test.display {
			t.Fatalf("ParseJSON(%s) = %v, %q; want %v, %q", test.raw, number, display, test.number, test.display)
		}
	}
}
