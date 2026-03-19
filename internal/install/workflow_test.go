package install

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/presets"
)

func TestNormalizeAndValidateDoesNotMutateReceiver(t *testing.T) {
	req := Request{
		Provider: config.ProviderConfig{
			ID:           "deepseek",
			Name:         "DeepSeek",
			BaseURL:      "https://api.deepseek.com/v1",
			PrimaryModel: "deepseek-chat",
		},
		AppVersion: "0.1.0",
	}

	normalized, err := req.NormalizeAndValidate(system.Info{OS: "linux"})
	if err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}

	if req.Mode != "" {
		t.Fatalf("original request mode mutated to %q", req.Mode)
	}
	if req.Provider.Type != "" {
		t.Fatalf("original provider type mutated to %q", req.Provider.Type)
	}
	if normalized.Mode == "" {
		t.Fatal("expected normalized mode to be populated")
	}
	if normalized.Provider.Type != "openai-compatible" {
		t.Fatalf("normalized provider type = %q, want %q", normalized.Provider.Type, "openai-compatible")
	}
}

func TestInstallNativeModeRejectsInvalidNPMRegistryURL(t *testing.T) {
	binDir := t.TempDir()
	npmPath := filepath.Join(binDir, "npm")
	if err := os.WriteFile(npmPath, []byte("#!/usr/bin/env sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile npmPath: %v", err)
	}
	t.Setenv("PATH", binDir)

	workflow := NewWorkflow(presets.Bundle{
		Mirrors: presets.MirrorManifest{
			Categories: map[string][]presets.MirrorCandidate{
				"npm_registry": {
					{Name: "bad", BaseURL: "http://registry.example.com"},
				},
			},
		},
	}, &recordingExecutor{})

	err := workflow.installNativeMode(context.Background(), system.Info{}, MirrorSelection{
		"npm_registry": {
			Name:    "bad",
			BaseURL: "http://registry.example.com",
		},
	}, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("expected invalid npm registry URL to be rejected")
	}
}
