package install

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/presets"
)

type mockExecutor struct {
	runFunc func(ctx context.Context, name string, args []string, env map[string]string, stdin string, stdout, stderr io.Writer) error
}

func (e *mockExecutor) Run(ctx context.Context, name string, args []string, env map[string]string, stdin string, stdout, stderr io.Writer) error {
	if e.runFunc != nil {
		return e.runFunc(ctx, name, args, env, stdin, stdout, stderr)
	}
	return nil
}

func TestResolveOpenClawWithPath_FoundInPath(t *testing.T) {
	binDir := t.TempDir()
	openclawPath := filepath.Join(binDir, "openclaw")
	if err := os.WriteFile(openclawPath, []byte("#!/usr/bin/env sh\necho 'openclaw'\n"), 0o700); err != nil {
		t.Fatalf("WriteFile openclawPath: %v", err)
	}
	t.Setenv("PATH", binDir)

	workflow := NewWorkflow(presets.Bundle{}, &mockExecutor{})

	path, inPath, err := workflow.resolveOpenClawWithPath(context.Background(), system.Info{OS: "linux"}, io.Discard)
	if err != nil {
		t.Fatalf("resolveOpenClawWithPath() error = %v", err)
	}
	if path == "" {
		t.Fatal("expected path to be non-empty")
	}
	if !inPath {
		t.Error("expected inPath to be true when found via LookPath")
	}
}

func TestResolveOpenClawWithPath_FoundViaNpmPrefix(t *testing.T) {
	prefixDir := t.TempDir()
	binDir := filepath.Join(prefixDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	npmPath := filepath.Join(prefixDir, "npm")
	if err := os.WriteFile(npmPath, []byte("#!/usr/bin/env sh\necho 'npm'\n"), 0o700); err != nil {
		t.Fatalf("WriteFile npmPath: %v", err)
	}

	openclawPath := filepath.Join(binDir, "openclaw")
	if err := os.WriteFile(openclawPath, []byte("#!/usr/bin/env sh\necho 'openclaw'\n"), 0o700); err != nil {
		t.Fatalf("WriteFile openclawPath: %v", err)
	}

	mockExec := &mockExecutor{
		runFunc: func(ctx context.Context, name string, args []string, env map[string]string, stdin string, stdout, stderr io.Writer) error {
			if len(args) >= 2 && args[0] == "prefix" && args[1] == "-g" {
				stdout.Write([]byte(prefixDir))
			}
			return nil
		},
	}

	workflow := NewWorkflow(presets.Bundle{}, mockExec)

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	os.Setenv("PATH", filepath.Dir(npmPath))

	path, inPath, err := workflow.resolveOpenClawWithPath(context.Background(), system.Info{OS: "linux"}, io.Discard)
	if err != nil {
		t.Fatalf("resolveOpenClawWithPath() error = %v", err)
	}
	if path == "" {
		t.Fatal("expected path to be non-empty")
	}
	if inPath {
		t.Error("expected inPath to be false when found via npm prefix fallback")
	}
}

func TestResolveOpenClawWithPath_NotFound(t *testing.T) {
	binDir := t.TempDir()
	t.Setenv("PATH", binDir)

	mockExec := &mockExecutor{
		runFunc: func(ctx context.Context, name string, args []string, env map[string]string, stdin string, stdout, stderr io.Writer) error {
			if len(args) >= 2 && args[0] == "prefix" && args[1] == "-g" {
				return nil
			}
			return nil
		},
	}

	workflow := NewWorkflow(presets.Bundle{}, mockExec)

	_, _, err := workflow.resolveOpenClawWithPath(context.Background(), system.Info{OS: "linux"}, io.Discard)
	if err == nil {
		t.Fatal("expected error when openclaw not found")
	}
}

func TestResolveOpenClawExecutable_BackwardCompatibility(t *testing.T) {
	binDir := t.TempDir()
	openclawPath := filepath.Join(binDir, "openclaw")
	if err := os.WriteFile(openclawPath, []byte("#!/usr/bin/env sh\necho 'openclaw'\n"), 0o700); err != nil {
		t.Fatalf("WriteFile openclawPath: %v", err)
	}
	t.Setenv("PATH", binDir)

	workflow := NewWorkflow(presets.Bundle{}, &mockExecutor{})

	path, err := workflow.resolveOpenClawExecutable(context.Background(), system.Info{OS: "linux"}, io.Discard)
	if err != nil {
		t.Fatalf("resolveOpenClawExecutable() error = %v", err)
	}
	if path == "" {
		t.Fatal("expected path to be non-empty")
	}
}

func TestNpmGlobalBinDir_Unix(t *testing.T) {
	prefixDir := t.TempDir()
	binDir := filepath.Join(prefixDir, "bin")

	mockExec := &mockExecutor{
		runFunc: func(ctx context.Context, name string, args []string, env map[string]string, stdin string, stdout, stderr io.Writer) error {
			if len(args) >= 2 && args[0] == "prefix" && args[1] == "-g" {
				stdout.Write([]byte(prefixDir))
			}
			return nil
		},
	}

	workflow := NewWorkflow(presets.Bundle{}, mockExec)

	result, err := workflow.npmGlobalBinDir(context.Background(), system.Info{OS: "linux"}, io.Discard)
	if err != nil {
		t.Fatalf("npmGlobalBinDir() error = %v", err)
	}
	if result != binDir {
		t.Errorf("npmGlobalBinDir() = %q, want %q", result, binDir)
	}
}

func TestNpmGlobalBinDir_Windows(t *testing.T) {
	prefixDir := t.TempDir()

	mockExec := &mockExecutor{
		runFunc: func(ctx context.Context, name string, args []string, env map[string]string, stdin string, stdout, stderr io.Writer) error {
			if len(args) >= 2 && args[0] == "prefix" && args[1] == "-g" {
				stdout.Write([]byte(prefixDir))
			}
			return nil
		},
	}

	workflow := NewWorkflow(presets.Bundle{}, mockExec)

	result, err := workflow.npmGlobalBinDir(context.Background(), system.Info{OS: "windows"}, io.Discard)
	if err != nil {
		t.Fatalf("npmGlobalBinDir() error = %v", err)
	}
	if result != prefixDir {
		t.Errorf("npmGlobalBinDir() = %q, want %q", result, prefixDir)
	}
}
