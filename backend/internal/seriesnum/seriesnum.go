package seriesnum

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var romanPattern = regexp.MustCompile(`^[IVXLCDM]+$`)

// Parse returns a sortable numeric value and optional display value for a series number.
// Decimal values are stored as numbers only. Roman numerals are stored as their numeric
// equivalent plus a canonical Roman display string.
func Parse(input string) (float64, string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return 0, "", nil
	}

	if number, err := strconv.ParseFloat(value, 64); err == nil {
		if number < 0 {
			return 0, "", errors.New("series number cannot be negative")
		}
		return number, "", nil
	}

	upper := strings.ToUpper(value)
	if !romanPattern.MatchString(upper) {
		return 0, "", fmt.Errorf("invalid series number %q", input)
	}

	number := romanToInt(upper)
	if number <= 0 || number > 3999 || ToRoman(number) != upper {
		return 0, "", fmt.Errorf("invalid roman numeral %q", input)
	}

	return float64(number), upper, nil
}

// ParseJSON accepts series numbers from existing numeric API clients and newer string clients.
func ParseJSON(raw json.RawMessage) (float64, string, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return 0, "", nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return Parse(text)
	}

	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		if number < 0 {
			return 0, "", errors.New("series number cannot be negative")
		}
		return number, "", nil
	}

	return 0, "", errors.New("series number must be a number or roman numeral")
}

func romanToInt(value string) int {
	values := map[rune]int{
		'I': 1,
		'V': 5,
		'X': 10,
		'L': 50,
		'C': 100,
		'D': 500,
		'M': 1000,
	}

	total := 0
	previous := 0
	for i := len(value) - 1; i >= 0; i-- {
		current := values[rune(value[i])]
		if current < previous {
			total -= current
		} else {
			total += current
			previous = current
		}
	}
	return total
}

func ToRoman(number int) string {
	if number <= 0 || number > 3999 {
		return ""
	}

	pairs := []struct {
		value  int
		symbol string
	}{
		{1000, "M"},
		{900, "CM"},
		{500, "D"},
		{400, "CD"},
		{100, "C"},
		{90, "XC"},
		{50, "L"},
		{40, "XL"},
		{10, "X"},
		{9, "IX"},
		{5, "V"},
		{4, "IV"},
		{1, "I"},
	}

	var builder strings.Builder
	for _, pair := range pairs {
		for number >= pair.value {
			builder.WriteString(pair.symbol)
			number -= pair.value
		}
	}
	return builder.String()
}
