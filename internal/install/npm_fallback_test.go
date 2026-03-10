package install

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/presets"
)

type npmFallbackExecutor struct {
	commands   []string
	registries []string
	prefixDir  string
}

func (e *npmFallbackExecutor) Run(ctx context.Context, cmd string, args []string, env map[string]string, dir string, stdout, stderr io.Writer) error {
	e.commands = append(e.commands, cmd+" "+joinArgs(args))
	if filepath.Base(cmd) == "npm" && len(args) == 2 && args[0] == "prefix" && args[1] == "-g" {
		_, _ = io.WriteString(stdout, e.prefixDir)
		return nil
	}
	if filepath.Base(cmd) == "npm" && len(args) >= 3 && args[0] == "install" && args[1] == "-g" && args[2] == "openclaw" {
		registry := env["NPM_CONFIG_REGISTRY"]
		e.registries = append(e.registries, registry)
		if strings.Contains(registry, "npmmirror") {
			return fmt.Errorf("getaddrinfo ENOTFOUND registry.npmmirror.com")
		}
	}
	return nil
}

func TestInstallNativeModeFallsBackToNextNPMRegistry(t *testing.T) {
	binDir := t.TempDir()
	npmPath := filepath.Join(binDir, "npm")
	if err := os.WriteFile(npmPath, []byte("#!/usr/bin/env sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile npmPath: %v", err)
	}
	t.Setenv("PATH", binDir)

	prefixDir := filepath.Join(binDir, "npm-global")
	if err := os.MkdirAll(prefixDir, 0o755); err != nil {
		t.Fatalf("MkdirAll prefixDir: %v", err)
	}

	executor := &npmFallbackExecutor{prefixDir: prefixDir}
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
	workflow.progress.Step("安装 OpenClaw 运行时")

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

	if !strings.Contains(out.String(), "继续尝试下一个源") {
		t.Fatalf("expected fallback progress message, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "切换到 official 后 npm 安装成功") {
		t.Fatalf("expected success fallback progress message, got:\n%s", out.String())
	}
}
