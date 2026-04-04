package ui

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestAskYesNoAcceptsChineseAnswers(t *testing.T) {
	prompt := NewPrompter(strings.NewReader("是\n"), &bytes.Buffer{})

	yes, err := prompt.AskYesNo("确认继续吗？", false)
	if err != nil {
		t.Fatalf("AskYesNo() error = %v", err)
	}
	if !yes {
		t.Fatal("expected Chinese affirmative answer to be accepted")
	}
}

func TestAskYesNoShowsChineseValidationMessage(t *testing.T) {
	var out bytes.Buffer
	prompt := NewPrompter(strings.NewReader("maybe\n否\n"), &out)

	yes, err := prompt.AskYesNo("确认继续吗？", true)
	if err != nil {
		t.Fatalf("AskYesNo() error = %v", err)
	}
	if yes {
		t.Fatal("expected final answer to be false")
	}
	if !strings.Contains(out.String(), "请输入 yes/no，也支持 y/n 或 是/否。") {
		t.Fatalf("expected Chinese validation message, got:\n%s", out.String())
	}
}

func TestAskStringSecretUsesSecretReaderAndHidesDefaultValue(t *testing.T) {
	var out bytes.Buffer
	prompt := &Prompter{
		reader: bufio.NewReader(strings.NewReader("unused\n")),
		out:    &out,
		readSecret: func() (string, error) {
			return "", nil
		},
	}

	value, err := prompt.AskString("API Key", "existing-secret", true)
	if err != nil {
		t.Fatalf("AskString() error = %v", err)
	}
	if value != "existing-secret" {
		t.Fatalf("AskString() = %q, want %q", value, "existing-secret")
	}
	if strings.Contains(out.String(), "existing-secret") {
		t.Fatalf("expected secret default to stay hidden, got prompt:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "[留空则沿用现有值]") {
		t.Fatalf("expected secret placeholder prompt, got:\n%s", out.String())
	}
}

// Tests for AskChoice method
func TestAskChoice(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		options      []string
		defaultValue string
		expected     string
		expectError  bool
	}{
		{
			name:        "select by number",
			input:       "1\n",
			options:     []string{"alpha", "beta"},
			expected:    "alpha",
			expectError: false,
		},
		{
			name:        "select by text",
			input:       "beta\n",
			options:     []string{"alpha", "beta"},
			expected:    "beta",
			expectError: false,
		},
		{
			name:        "select by valid index within range",
			input:       "2\n",   // Valid index within bounds
			options:     []string{"alpha", "beta", "gamma"},
			expected:    "beta",
			expectError: false,
		},
		{
			name:         "select default on empty input",
			input:        "\n",
			options:      []string{"alpha", "beta"},
			defaultValue: "beta",
			expected:     "beta",
			expectError:  false,
		},
		{
			name:        "non-existent option then valid input",
			input:       "gamma\n2\n",  // gamma doesn't exist, then select by number
			options:     []string{"alpha", "beta"},
			expected:    "beta",
			expectError: false,
		},
		{
			name:        "case insensitive matching",
			input:       "BETA\n",
			options:     []string{"alpha", "beta"},
			expected:    "beta",
			expectError: false,
		},
		{
			name:        "select from single option",
			input:       "1\n",
			options:     []string{"only_option"},
			expected:    "only_option",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			out := &bytes.Buffer{}
			p := NewPrompter(in, out)
			
			result, err := p.AskChoice("Pick one", tt.options, tt.defaultValue)
			
			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Fatalf("expected %s, got %s", tt.expected, result)
				}
			}
		})
	}
}

func TestAskChoiceEmptyOptions(t *testing.T) {
	in := strings.NewReader("")
	out := &bytes.Buffer{}
	p := NewPrompter(in, out)
	
	_, err := p.AskChoice("Pick one", []string{}, "")
	if err == nil {
		t.Fatal("expected error for empty options")
	}
	if !strings.Contains(err.Error(), "没有可选项") {
		t.Fatalf("expected error to mention lack of options, got: %v", err)
	}
}

func TestNewPrompterTTYBehavior(t *testing.T) {
	// We can't fully test the actual TTY behavior, but we can test non-TTY
	// and ensure that readSecret is not set for non-terminal inputs
	
	// Non-TTY input (String Reader) - should not have readSecret
	in := strings.NewReader("test\n")
	out := &bytes.Buffer{}
	p := NewPrompter(in, out)
	
	// Since our input isn't an os.File, readSecret should be nil
	if p.readSecret != nil {
		t.Fatal("expected readSecret to be nil for non-TTY input")
	}
}

// Additional tests to improve overall coverage
func TestReadLineEOF(t *testing.T) {
	in := strings.NewReader("hello") // No newline
	out := &bytes.Buffer{}
	p := NewPrompter(in, out)
	
	result, err := p.readLine()
	if err != nil {
		t.Fatalf("unexpected error reading line: %v", err)
	}
	if result != "hello" {
		t.Fatalf("expected hello, got %s", result)
	}
}

func TestAskStringHandlesEmptyInput(t *testing.T) {
	in := strings.NewReader("\n")
	out := &bytes.Buffer{}
	p := NewPrompter(in, out)
	
	result, err := p.AskString("Enter value", "default_value", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "default_value" {
		t.Fatalf("expected default_value, got %s", result)
	}
}

func TestAskYesNoHandlesEmptyInput(t *testing.T) {
	in := strings.NewReader("\n")
	out := &bytes.Buffer{}
	p := NewPrompter(in, out)
	
	result, err := p.AskYesNo("Continue?", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != true {
		t.Fatalf("expected true, got %v", result)
	}
}
