package main

import "strings"

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
