package app

import (
	"os"
	"testing"

	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/presets"
)

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"ascii", "hello", 5},
		{"chinese", "你好", 4},
		{"mixed", "安装完成", 8},
		{"mixed2", "完成安装", 8},
		{"single-byte", "a", 1},
		{"single-cjk", "你", 2},
		{"mixed-emoji", "Hello你好", 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayWidth(tt.input); got != tt.want {
				t.Errorf("displayWidth(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{"all-empty", []string{"", "", ""}, ""},
		{"first-non-empty", []string{"hello", "", ""}, "hello"},
		{"middle-non-empty", []string{"", "world", ""}, "world"},
		{"last-non-empty", []string{"", "", "last"}, "last"},
		{"with-whitespace", []string{"", "  ", "test"}, "test"},
		{"single-non-empty", []string{"only"}, "only"},
		{"single-empty", []string{""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstNonEmpty(tt.input...); got != tt.want {
				t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseCSV(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single-value", "hello", []string{"hello"}},
		{"multiple-values", "a,b,c", []string{"a", "b", "c"}},
		{"with-whitespace", " a , b , c ", []string{"a", "b", "c"}},
		{"trailing-comma", "a,b,", []string{"a", "b"}},
		{"leading-comma", ",a,b", []string{"a", "b"}},
		{"empty-between", "a,,b", []string{"a", "b"}},
		{"all-whitespace", "  ,  ,  ", nil},
		{"single-empty-string", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCSV(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseCSV(%q) = %v, want %v", tt.input, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseCSV(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestNormalizedProvisioner(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"empty", "", "bridge"},
		{"bridge", "bridge", "bridge"},
		{"plugin", "plugin", "plugin"},
		{"other", "custom", "custom"},
		{"whitespace", "  ", "bridge"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizedProvisioner(tt.value); got != tt.want {
				t.Errorf("normalizedProvisioner(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestBoolLabel(t *testing.T) {
	tests := []struct {
		name  string
		value bool
		want  string
	}{
		{"true", true, "已检测"},
		{"false", false, "未检测"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := boolLabel(tt.value); got != tt.want {
				t.Errorf("boolLabel(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestEnvOrEmpty(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		setup func()
		want  string
	}{
		{"unset", "NON_EXISTENT_VAR_12345", func() {}, ""},
		{"set", "TEST_VAR_12345", func() { os.Setenv("TEST_VAR_12345", "test-value") }, "test-value"},
		{"set-empty", "TEST_EMPTY_VAR_12345", func() { os.Setenv("TEST_EMPTY_VAR_12345", "") }, ""},
		{"set-whitespace", "TEST_WS_VAR_12345", func() { os.Setenv("TEST_WS_VAR_12345", "  ") }, ""},
		{"empty-key", "", func() {}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				if tt.key != "" && tt.key != "NON_EXISTENT_VAR_12345" {
					os.Unsetenv(tt.key)
				}
			})
			tt.setup()
			if got := envOrEmpty(tt.key); got != tt.want {
				t.Errorf("envOrEmpty(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestConvertProviderCatalog(t *testing.T) {
	tests := []struct {
		name     string
		input    []presets.ProviderModel
		expected []config.ProviderModel
	}{
		{
			name:     "empty",
			input:    nil,
			expected: nil,
		},
		{
			name:     "single model",
			input:    []presets.ProviderModel{{ID: "model1", Name: "Model One"}},
			expected: []config.ProviderModel{{ID: "model1", Name: "Model One"}},
		},
		{
			name: "multiple models",
			input: []presets.ProviderModel{
				{ID: "m1", Name: "M1", Reasoning: true, ContextWindow: 4096, MaxTokens: 2048},
				{ID: "m2", Name: "M2", Cost: presets.ModelCost{Input: 0.1, Output: 0.2}},
			},
			expected: []config.ProviderModel{
				{ID: "m1", Name: "M1", Reasoning: true, ContextWindow: 4096, MaxTokens: 2048},
				{ID: "m2", Name: "M2", Cost: config.ModelCost{Input: 0.1, Output: 0.2}},
			},
		},
		{
			name:     "with empty id",
			input:    []presets.ProviderModel{{ID: "", Name: "No ID"}},
			expected: []config.ProviderModel{{ID: "", Name: "No ID"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertProviderCatalog(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("convertProviderCatalog(%v) length = %d, want %d", tt.input, len(got), len(tt.expected))
				return
			}
			for i := range got {
				if got[i].ID != tt.expected[i].ID {
					t.Errorf("convertProviderCatalog()[%d].ID = %q, want %q", i, got[i].ID, tt.expected[i].ID)
				}
				if got[i].Name != tt.expected[i].Name {
					t.Errorf("convertProviderCatalog()[%d].Name = %q, want %q", i, got[i].Name, tt.expected[i].Name)
				}
				if got[i].Reasoning != tt.expected[i].Reasoning {
					t.Errorf("convertProviderCatalog()[%d].Reasoning = %v, want %v", i, got[i].Reasoning, tt.expected[i].Reasoning)
				}
				if got[i].ContextWindow != tt.expected[i].ContextWindow {
					t.Errorf("convertProviderCatalog()[%d].ContextWindow = %v, want %v", i, got[i].ContextWindow, tt.expected[i].ContextWindow)
				}
				if got[i].MaxTokens != tt.expected[i].MaxTokens {
					t.Errorf("convertProviderCatalog()[%d].MaxTokens = %v, want %v", i, got[i].MaxTokens, tt.expected[i].MaxTokens)
				}
			}
		})
	}
}

func TestProviderModelIDs(t *testing.T) {
	tests := []struct {
		name     string
		preset   presets.ProviderPreset
		catalog  []config.ProviderModel
		expected []string
	}{
		{
			name:     "empty preset and catalog",
			preset:   presets.ProviderPreset{Models: []string{}},
			catalog:  []config.ProviderModel{},
			expected: []string{},
		},
		{
			name:     "catalog takes precedence",
			preset:   presets.ProviderPreset{Models: []string{"preset1", "preset2"}},
			catalog:  []config.ProviderModel{{ID: "cat1"}, {ID: "cat2"}},
			expected: []string{"cat1", "cat2"},
		},
		{
			name:     "no catalog, use preset models",
			preset:   presets.ProviderPreset{Models: []string{"m1", "m2"}},
			catalog:  []config.ProviderModel{},
			expected: []string{"m1", "m2"},
		},
		{
			name:     "catalog with empty id skipped",
			preset:   presets.ProviderPreset{},
			catalog:  []config.ProviderModel{{ID: "valid"}, {ID: ""}, {ID: "also-valid"}},
			expected: []string{"valid", "also-valid"},
		},
		{
			name:     "preserved models slice",
			preset:   presets.ProviderPreset{Models: []string{"a", "b"}},
			catalog:  []config.ProviderModel{},
			expected: []string{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerModelIDs(tt.preset, tt.catalog)
			if len(got) != len(tt.expected) {
				t.Errorf("providerModelIDs() length = %d, want %d", len(got), len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("providerModelIDs()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestPrintHelp(t *testing.T) {
	printHelp(os.Stdout)
}
