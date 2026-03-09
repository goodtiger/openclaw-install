package install

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/presets"
)

type npmFallbackExecutor struct {
	commands   []string
	registries []string
}

func (e *npmFallbackExecutor) Run(ctx context.Context, cmd string, args []string, env map[string]string, dir string, stdout, stderr io.Writer) error {
	e.commands = append(e.commands, cmd+" "+joinArgs(args))
	if cmd == "npm" && len(args) >= 3 && args[0] == "install" && args[1] == "-g" && args[2] == "openclaw" {
		registry := env["NPM_CONFIG_REGISTRY"]
		e.registries = append(e.registries, registry)
		if strings.Contains(registry, "npmmirror") {
			return fmt.Errorf("getaddrinfo ENOTFOUND registry.npmmirror.com")
		}
	}
	return nil
}

func TestInstallNativeModeFallsBackToNextNPMRegistry(t *testing.T) {
	t.Setenv("PATH", "")

	executor := &npmFallbackExecutor{}
	workflow := NewWorkflow(presets.Bundle{
		Mirrors: presets.MirrorManifest{
			Categories: map[string][]presets.MirrorCandidate{
				"npm_registry": {
					{Name: "npmmirror", BaseURL: "https://registry.npmmirror.com"},
					{Name: "official", BaseURL: "https://registry.npmjs.org"},
				},
			},
		},
	}, executor)

	var out bytes.Buffer
	workflow.progress = newProgressTracker(&out, 1)
	workflow.progress.Step("Installing OpenClaw runtime")

	err := workflow.installNativeMode(context.Background(), system.Info{}, MirrorSelection{
		"npm_registry": {
			Name:    "npmmirror",
			BaseURL: "https://registry.npmmirror.com",
		},
	}, &out, io.Discard)
	if err != nil {
		t.Fatalf("installNativeMode() error = %v", err)
	}

	wantRegistries := []string{
		"https://registry.npmmirror.com",
		"https://registry.npmjs.org",
	}
	if strings.Join(executor.registries, ",") != strings.Join(wantRegistries, ",") {
		t.Fatalf("npm registries = %#v, want %#v", executor.registries, wantRegistries)
	}

	if !strings.Contains(out.String(), "retrying next candidate") {
		t.Fatalf("expected fallback progress message, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "succeeded after falling back to official") {
		t.Fatalf("expected success fallback progress message, got:\n%s", out.String())
	}
}
