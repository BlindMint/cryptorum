package main

import (
	"encoding/json"
	"sort"
	"strings"
)

func cleanedStringList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func uniqueMetadataStringList(values []string) []string {
	unique := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range cleanedStringList(values) {
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, value)
	}
	return unique
}

func normalizeMetadataStringList(values []string) []string {
	return uniqueMetadataStringList(values)
}

func normalizeMetadataTagList(values []string) []string {
	normalized := uniqueMetadataStringList(values)
	sort.Slice(normalized, func(i, j int) bool {
		left := strings.ToLower(normalized[i])
		right := strings.ToLower(normalized[j])
		if left == right {
			return normalized[i] < normalized[j]
		}
		return left < right
	})
	return normalized
}

func parseMetadataJSONList(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

func mergeMetadataTagLists(lists ...[]string) []string {
	var merged []string
	for _, values := range lists {
		merged = append(merged, values...)
	}
	return normalizeMetadataTagList(merged)
}

func mergeMetadataTagJSON(rawLists ...string) string {
	var lists [][]string
	for _, raw := range rawLists {
		lists = append(lists, parseMetadataJSONList(raw))
	}
	valuesJSON, _ := json.Marshal(mergeMetadataTagLists(lists...))
	return string(valuesJSON)
}
