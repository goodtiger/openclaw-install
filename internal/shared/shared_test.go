package shared

import (
	"testing"
)

func TestResolveWithSource(t *testing.T) {
	tests := []struct {
		name     string
		pairs    []ResolvedValue
		expected ResolvedValue
	}{
		{"empty pairs", []ResolvedValue{}, ResolvedValue{}},
		{"all empty values", []ResolvedValue{
			{Value: "", Source: "env"},
			{Value: "", Source: "flag"},
			{Value: "", Source: "default"},
		}, ResolvedValue{}},
		{"first match", []ResolvedValue{
			{Value: "first", Source: "env"},
			{Value: "second", Source: "flag"},
		}, ResolvedValue{Value: "first", Source: "env"}},
		{"second match", []ResolvedValue{
			{Value: "", Source: "env"},
			{Value: "second", Source: "flag"},
			{Value: "third", Source: "default"},
		}, ResolvedValue{Value: "second", Source: "flag"}},
		{"whitespace-only values", []ResolvedValue{
			{Value: "   ", Source: "env"},
			{Value: "\t\n", Source: "flag"},
			{Value: "real", Source: "default"},
		}, ResolvedValue{Value: "real", Source: "default"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveWithSource(tt.pairs...)
			if result.Value != tt.expected.Value || result.Source != tt.expected.Source {
				t.Errorf("ResolveWithSource(%v) = %v, want %v", tt.pairs, result, tt.expected)
			}
		})
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"short", "abc", "****"},
		{"very short", "ab", "****"},
		{"four chars", "abcd", "****"},
		{"five chars", "abcde", "****bcde"},
		{"normal", "sk-xxxx1234", "****1234"},
		{"long secret", "my-secret-value-very-long", "****long"},
		{"unicode", "密码1234", "****1234"},
		{"emoji", "🔑🔐🔒🔒1234", "****1234"},
		{"exactly 4 chars", "1234", "****"},
		{"exactly 5 chars", "12345", "****2345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskSecret(tt.input); got != tt.want {
				t.Errorf("MaskSecret(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValueOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		fallback string
		expected string
	}{
		{"empty value", "", "default", "default"},
		{"whitespace-only value", "   ", "default", "default"},
		{"tab-only value", "\t", "default", "default"},
		{"newline value", "\n\n", "default", "default"},
		{"valid value", "actual", "default", "actual"},
		{"whitespace with value", " actual ", "default", " actual "},
		{"empty fallback", "actual", "", "actual"},
		{"empty value empty fallback", "", "", ""},
		{"whitespace value to empty fallback", "   ", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValueOrDefault(tt.value, tt.fallback); got != tt.expected {
				t.Errorf("ValueOrDefault(%q, %q) = %q, want %q", tt.value, tt.fallback, got, tt.expected)
			}
		})
	}
}

func TestUsesBridgeProvisioner(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"empty string", "", true},
		{"bridge", "bridge", true},
		{"spaces around bridge", "  bridge  ", true},
		{"bridge with tabs", "\tbridge\t", true},
		{"bridge with newlines", "\nbridge\n", true},
		{"plugin", "plugin", false},
		{"spaces around plugin", "  plugin  ", false},
		{"openclaw", "openclaw", false},
		{"some other value", "other", false},
		{"case sensitive - BRIDGE", "BRIDGE", false},
		{"trimming - bridge with extra", "  bridge_extra  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UsesBridgeProvisioner(tt.input); got != tt.expected {
				t.Errorf("UsesBridgeProvisioner(%q) = %t, want %t", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name     string
		inputs   []string
		expected string
	}{
		{"empty inputs", []string{}, ""},
		{"all empty", []string{"", "", ""}, ""},
		{"all whitespace", []string{"   ", "\t", "\n"}, ""},
		{"first non-empty", []string{"first", "second", "third"}, "first"},
		{"first empty, second non-empty", []string{"", "second", "third"}, "second"},
		{"first two empty, third non-empty", []string{"", "", "third"}, "third"},
		{"with whitespace around value", []string{"   ", "\t", " value "}, " value "},
		{"mixed whitespace and values", []string{"   ", "\t\n", "actual"}, "actual"},
		{"single value", []string{"only"}, "only"},
		{"single empty", []string{""}, ""},
		{"single whitespace", []string{"   "}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FirstNonEmpty(tt.inputs...); got != tt.expected {
				t.Errorf("FirstNonEmpty(%v) = %q, want %q", tt.inputs, got, tt.expected)
			}
		})
	}
}

func TestSortedStringKeys(t *testing.T) {
	intMap := map[string]int{
		"apple":  1,
		"banana": 2,
		"cherry": 3,
		"date":   4,
	}

	// Test with different value types to ensure generic works
	tests := []struct {
		name        string
		input       interface{}
		expected    []string
		description string
	}{
		{"empty map", map[string]int{}, []string{}, "Empty map should return empty slice"},
		{"single key", map[string]interface{}{"one": 1}, []string{"one"}, "Single key should return that key in a slice"},
		{"multiple keys - int values", intMap, []string{"apple", "banana", "cherry", "date"}, "Multiple keys should be sorted alphabetically"},
		{"multiple keys - string values", map[string]string{
			"zebra": "animal",
			"apple": "fruit",
			"car":   "vehicle",
		}, []string{"apple", "car", "zebra"}, "String values should produce sorted keys"},
		{"multiple keys - bool values", map[string]bool{
			"last":   true,
			"first":  false,
			"middle": true,
		}, []string{"first", "last", "middle"}, "Boolean values should produce sorted keys"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result []string
			switch input := tt.input.(type) {
			case map[string]int:
				result = SortedStringKeys(input)
			case map[string]string:
				result = SortedStringKeys(input)
			case map[string]bool:
				result = SortedStringKeys(input)
			case map[string]interface{}:
				result = SortedStringKeys(input)
			default:
				t.Fatalf("unsupported type for test: %T", tt.input)
			}

			if len(result) != len(tt.expected) {
				t.Errorf("SortedStringKeys(%v) length = %d, want %d. Got: %v, Expected: %v", tt.input, len(result), len(tt.expected), result, tt.expected)
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("SortedStringKeys(%v)[%d] = %q, want %q. Full result: %v, Expected: %v", tt.input, i, result[i], tt.expected[i], result, tt.expected)
					return
				}
			}
		})
	}
}
