package install

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/presets"
)

type recordingExecutor struct {
	commands []string
}

func (e *recordingExecutor) Run(ctx context.Context, cmd string, args []string, env map[string]string, dir string, stdout, stderr io.Writer) error {
	e.commands = append(e.commands, cmd+" "+joinArgs(args))
	return nil
}

func TestCleanupSystemdUserServiceSkipsMissingUnitFile(t *testing.T) {
	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	homeDir := t.TempDir()
	info := system.Info{HomeDir: homeDir}

	if err := workflow.cleanupSystemdUserService(context.Background(), info, "qq", io.Discard, io.Discard); err != nil {
		t.Fatalf("cleanupSystemdUserService() error = %v", err)
	}

	if len(executor.commands) != 0 {
		t.Fatalf("expected no commands for missing unit file, got %#v", executor.commands)
	}
}

func TestCleanupObsoleteChannelAssetsRemovesExistingBridgeArtifacts(t *testing.T) {
	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	homeDir := t.TempDir()
	runtimeDir := filepath.Join(homeDir, ".openclaw", "runtime")
	serviceDir := filepath.Join(homeDir, ".config", "systemd", "user")

	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll runtimeDir: %v", err)
	}
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll serviceDir: %v", err)
	}

	servicePath := filepath.Join(serviceDir, "openclaw-bridge-qq.service")
	if err := os.WriteFile(servicePath, []byte("[Unit]\nDescription=test\n"), 0o600); err != nil {
		t.Fatalf("WriteFile servicePath: %v", err)
	}

	scriptPath := filepath.Join(runtimeDir, "bridge-qq.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile scriptPath: %v", err)
	}

	info := system.Info{
		OS:         "linux",
		HomeDir:    homeDir,
		RuntimeDir: runtimeDir,
	}

	err := workflow.cleanupObsoleteChannelAssets(
		context.Background(),
		info,
		config.InstallState{
			ManagedChannels: []string{"qq"},
		},
		nil,
		io.Discard,
		io.Discard,
	)
	if err != nil {
		t.Fatalf("cleanupObsoleteChannelAssets() error = %v", err)
	}

	if _, err := os.Stat(servicePath); !os.IsNotExist(err) {
		t.Fatalf("expected service file to be removed, stat err = %v", err)
	}

	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Fatalf("expected bridge script to be removed, stat err = %v", err)
	}
}

func joinArgs(args []string) string {
	return strings.Join(args, " ")
}
