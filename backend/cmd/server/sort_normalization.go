package main

import (
	"strings"
	"unicode"
)

const sqlLeadingSortTrimChars = " \t\n\r\"'`“”‘’.,:;!?¿¡()[]{}<>-–—_/#\\|*+~=…"

var latinConfusableSortReplacer = strings.NewReplacer(
	"Α", "A", "А", "A",
	"Β", "B", "В", "B",
	"Ε", "E", "Е", "E",
	"Ζ", "Z",
	"Η", "H", "Н", "H",
	"Ι", "I", "І", "I",
	"Κ", "K", "К", "K",
	"Μ", "M", "М", "M",
	"Ν", "N",
	"Ο", "O", "О", "O",
	"Ρ", "P", "Р", "P",
	"Τ", "T", "Т", "T",
	"Υ", "Y", "У", "Y",
	"Χ", "X", "Х", "X",
	"Ϲ", "C", "С", "C",
	"α", "a", "а", "a",
	"β", "b", "в", "b",
	"ε", "e", "е", "e",
	"η", "h", "н", "h",
	"ι", "i", "і", "i",
	"κ", "k", "к", "k",
	"μ", "m", "м", "m",
	"ο", "o", "о", "o",
	"ρ", "p", "р", "p",
	"τ", "t", "т", "t",
	"υ", "y", "у", "y",
	"χ", "x", "х", "x",
	"ϲ", "c", "с", "c",
)

var sqlLatinConfusableSortReplacements = [][2]string{
	{"Α", "A"}, {"А", "A"},
	{"Β", "B"}, {"В", "B"},
	{"Ε", "E"}, {"Е", "E"},
	{"Ζ", "Z"},
	{"Η", "H"}, {"Н", "H"},
	{"Ι", "I"}, {"І", "I"},
	{"Κ", "K"}, {"К", "K"},
	{"Μ", "M"}, {"М", "M"},
	{"Ν", "N"},
	{"Ο", "O"}, {"О", "O"},
	{"Ρ", "P"}, {"Р", "P"},
	{"Τ", "T"}, {"Т", "T"},
	{"Υ", "Y"}, {"У", "Y"},
	{"Χ", "X"}, {"Х", "X"},
	{"Ϲ", "C"}, {"С", "C"},
	{"α", "a"}, {"а", "a"},
	{"β", "b"}, {"в", "b"},
	{"ε", "e"}, {"е", "e"},
	{"η", "h"}, {"н", "h"},
	{"ι", "i"}, {"і", "i"},
	{"κ", "k"}, {"к", "k"},
	{"μ", "m"}, {"м", "m"},
	{"ο", "o"}, {"о", "o"},
	{"ρ", "p"}, {"р", "p"},
	{"τ", "t"}, {"т", "t"},
	{"υ", "y"}, {"у", "y"},
	{"χ", "x"}, {"х", "x"},
	{"ϲ", "c"}, {"с", "c"},
}

func normalizedSortKey(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimLeftFunc(value, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsSpace(r)
	})
	value = latinConfusableSortReplacer.Replace(value)
	return strings.ToLower(value)
}

func sqlStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func normalizedSortSQL(expression string) string {
	normalized := "LTRIM(" + expression + ", " + sqlStringLiteral(sqlLeadingSortTrimChars) + ")"
	for _, replacement := range sqlLatinConfusableSortReplacements {
		normalized = "REPLACE(" + normalized + ", " + sqlStringLiteral(replacement[0]) + ", " + sqlStringLiteral(replacement[1]) + ")"
	}
	return "LOWER(" + normalized + ")"
}

func titleSortSQL() string {
	return normalizedSortSQL("COALESCE(bm.title, '')")
}

func authorsSortSQL() string {
	return normalizedSortSQL("REPLACE(REPLACE(REPLACE(COALESCE(bm.authors, ''), '[', ''), ']', ''), '\"', '')")
}

func seriesSortSQL() string {
	return normalizedSortSQL("COALESCE(bm.series, '')")
}
