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
