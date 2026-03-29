package shared

import (
	"slices"
	"strings"
)

// ResolvedValue holds a configuration value together with where it came from.
type ResolvedValue struct {
	Value  string
	Source string // e.g. "env:DASHSCOPE_API_KEY", "flag:--api-key", "default", ""
}

// ResolveWithSource picks the first non-empty value from a series of (value, source)
// pairs and returns both the value and its source. Returns empty if all are empty.
func ResolveWithSource(pairs ...ResolvedValue) ResolvedValue {
	for _, pair := range pairs {
		if strings.TrimSpace(pair.Value) != "" {
			return pair
		}
	}
	return ResolvedValue{}
}

// MaskSecret masks a secret value for display, showing only the last 4 chars.
// Returns "****" for values shorter than 5 chars, empty string for empty values.
func MaskSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) < 5 {
		return "****"
	}
	return "****" + value[len(value)-4:]
}

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
