package app

import (
	"io"
	"strings"
	"testing"

	"github.com/goodtiger/openclaw-install/internal/install"
	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/internal/ui"
)

func TestChooseModeAllowsWindowsNativeSelection(t *testing.T) {
	prompter := ui.NewPrompter(strings.NewReader("2\n"), io.Discard)

	mode, err := chooseMode(prompter, system.Info{OS: "windows"}, false, install.ModeDocker)
	if err != nil {
		t.Fatalf("chooseMode() error = %v", err)
	}
	if mode != install.ModeNative {
		t.Fatalf("chooseMode() = %q, want %q", mode, install.ModeNative)
	}
}
