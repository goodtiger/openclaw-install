package app

import (
	"io"
	"strings"
	"testing"

	"github.com/goodtiger/openclaw-install/internal/install"
	"github.com/goodtiger/openclaw-install/internal/ui"
	"github.com/goodtiger/openclaw-install/presets"
)

func TestChooseModeAllowsWindowsNativeSelection(t *testing.T) {
	prompter := ui.NewPrompter(strings.NewReader("2\n"), io.Discard)

	mode, err := chooseMode(prompter, false, install.ModeDocker)
	if err != nil {
		t.Fatalf("chooseMode() error = %v", err)
	}
	if mode != install.ModeNative {
		t.Fatalf("chooseMode() = %q, want %q", mode, install.ModeNative)
	}
}

func TestChooseProviderPresetRejectsEmptyBundle(t *testing.T) {
	prompter := ui.NewPrompter(strings.NewReader(""), io.Discard)

	_, err := chooseProviderPreset(prompter, presets.Bundle{}, "", true, "")
	if err == nil {
		t.Fatal("expected empty provider bundle to be rejected")
	}
}
