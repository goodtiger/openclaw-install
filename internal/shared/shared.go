package shared

import (
	"slices"
	"strings"
)

func ValueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func SortedStringKeys[T any](input map[string]T) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
