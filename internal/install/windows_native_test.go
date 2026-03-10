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

type windowsNativeExecutor struct {
	commands                []string
	npmPrefix               string
	createOpenClawOnInstall bool
}

func (e *windowsNativeExecutor) Run(ctx context.Context, cmd string, args []string, env map[string]string, dir string, stdout, stderr io.Writer) error {
	e.commands = append(e.commands, cmd+" "+joinArgs(args))

	base := strings.ToLower(filepath.Base(cmd))
	switch {
	case (base == "npm.cmd" || base == "npm") && len(args) == 2 && args[0] == "prefix" && args[1] == "-g":
		_, _ = io.WriteString(stdout, e.npmPrefix)
	case (base == "npm.cmd" || base == "npm") && len(args) >= 3 && args[0] == "install" && args[1] == "-g" && args[2] == "openclaw":
		if e.createOpenClawOnInstall {
			if err := os.MkdirAll(e.npmPrefix, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(e.npmPrefix, "openclaw.cmd"), []byte("@echo off\r\n"), 0o600); err != nil {
				return err
			}
		}
	}

	return nil
}

func TestRequestValidateAllowsWindowsNative(t *testing.T) {
	req := Request{
		Mode: ModeNative,
		Provider: config.ProviderConfig{
			ID:           "bailian",
			Name:         "Alibaba Bailian Coding Plan",
			BaseURL:      "https://coding.dashscope.aliyuncs.com/v1",
			PrimaryModel: "qwen3.5-plus",
		},
		AppVersion: "0.1.5",
	}

	if err := req.Validate(system.Info{OS: "windows"}); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestResolveOpenClawExecutableUsesNPMPrefixOnWindows(t *testing.T) {
	rootDir, prefixDir := prepareWindowsNativeEnv(t)
	openClawPath := filepath.Join(prefixDir, "openclaw.cmd")
	if err := os.WriteFile(openClawPath, []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatalf("WriteFile openClawPath: %v", err)
	}

	executor := &windowsNativeExecutor{npmPrefix: prefixDir}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	got, err := workflow.resolveOpenClawExecutable(context.Background(), system.Info{OS: "windows"}, io.Discard)
	if err != nil {
		t.Fatalf("resolveOpenClawExecutable() error = %v", err)
	}
	if got != openClawPath {
		t.Fatalf("resolveOpenClawExecutable() = %q, want %q", got, openClawPath)
	}
	if !containsCommand(executor.commands, filepath.Join(rootDir, "Program Files", "nodejs", "npm.cmd")+" prefix -g") {
		t.Fatalf("expected npm prefix lookup, commands = %#v", executor.commands)
	}
}

func TestInstallNativeModeUsesResolvedExecutablesOnWindows(t *testing.T) {
	_, prefixDir := prepareWindowsNativeEnv(t)

	executor := &windowsNativeExecutor{
		npmPrefix:               prefixDir,
		createOpenClawOnInstall: true,
	}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	err := workflow.installNativeMode(context.Background(), system.Info{OS: "windows"}, MirrorSelection{
		"npm_registry": {
			Name:    "official",
			BaseURL: "https://registry.npmjs.org",
		},
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("installNativeMode() error = %v", err)
	}

	if !containsCommand(executor.commands, "npm.cmd install -g openclaw") {
		t.Fatalf("expected npm install command, commands = %#v", executor.commands)
	}
	if !containsCommand(executor.commands, filepath.Join(prefixDir, "openclaw.cmd")+" gateway start") {
		t.Fatalf("expected resolved openclaw gateway start command, commands = %#v", executor.commands)
	}
}

func TestRunOpenClawCommandNativeUsesResolvedPathOnWindows(t *testing.T) {
	_, prefixDir := prepareWindowsNativeEnv(t)
	openClawPath := filepath.Join(prefixDir, "openclaw.cmd")
	if err := os.WriteFile(openClawPath, []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatalf("WriteFile openClawPath: %v", err)
	}

	executor := &windowsNativeExecutor{npmPrefix: prefixDir}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	if err := workflow.runOpenClawCommand(context.Background(), system.Info{OS: "windows"}, ModeNative, []string{"channels", "list"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("runOpenClawCommand() error = %v", err)
	}
	if !containsCommand(executor.commands, openClawPath+" channels list") {
		t.Fatalf("expected resolved openclaw command, commands = %#v", executor.commands)
	}
	if workflow.shouldSkipPluginProvisioning(context.Background(), system.Info{OS: "windows"}, ModeNative) {
		t.Fatalf("shouldSkipPluginProvisioning() = true, want false")
	}
}

func TestVerifyNativeUsesResolvedOpenClawOnWindows(t *testing.T) {
	homeDir := t.TempDir()
	info := system.Info{
		OS:               "windows",
		HomeDir:          homeDir,
		OpenClawHome:     filepath.Join(homeDir, ".openclaw"),
		ConfigPath:       filepath.Join(homeDir, ".openclaw", "openclaw.json"),
		BridgeConfigPath: filepath.Join(homeDir, ".openclaw", "bridge.json"),
		StatePath:        filepath.Join(homeDir, ".openclaw", "install-state.json"),
		RuntimeDir:       filepath.Join(homeDir, ".openclaw", "runtime"),
	}
	if err := config.SaveJSONAtomic(info.ConfigPath, map[string]any{"gateway": map[string]any{"mode": "local"}}); err != nil {
		t.Fatalf("SaveJSONAtomic config: %v", err)
	}
	if err := config.SaveJSONAtomic(info.BridgeConfigPath, config.BridgeConfig{
		Version:        1,
		TimeoutSeconds: 30,
		Channels:       map[string]config.BridgeChannelConfig{},
	}); err != nil {
		t.Fatalf("SaveJSONAtomic bridge config: %v", err)
	}

	_, prefixDir := prepareWindowsNativeEnv(t)
	openClawPath := filepath.Join(prefixDir, "openclaw.cmd")
	if err := os.WriteFile(openClawPath, []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatalf("WriteFile openClawPath: %v", err)
	}

	executor := &windowsNativeExecutor{npmPrefix: prefixDir}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	if _, err := workflow.verify(context.Background(), info, Request{Mode: ModeNative}, io.Discard, io.Discard); err != nil {
		t.Fatalf("verify() error = %v", err)
	}
	if !containsCommand(executor.commands, openClawPath+" config validate") {
		t.Fatalf("expected resolved openclaw config validation, commands = %#v", executor.commands)
	}
}

func prepareWindowsNativeEnv(t *testing.T) (string, string) {
	t.Helper()

	rootDir := t.TempDir()
	programFilesDir := filepath.Join(rootDir, "Program Files")
	nodeDir := filepath.Join(programFilesDir, "nodejs")
	prefixDir := filepath.Join(rootDir, "AppData", "Roaming", "npm")

	if err := os.MkdirAll(nodeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll nodeDir: %v", err)
	}
	if err := os.MkdirAll(prefixDir, 0o755); err != nil {
		t.Fatalf("MkdirAll prefixDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nodeDir, "npm.cmd"), []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatalf("WriteFile npm.cmd: %v", err)
	}

	t.Setenv("PATH", "")
	t.Setenv("ProgramFiles", programFilesDir)
	t.Setenv("ProgramFiles(x86)", "")

	return rootDir, prefixDir
}

func containsCommand(commands []string, want string) bool {
	for _, command := range commands {
		if strings.Contains(command, want) {
			return true
		}
	}
	return false
}
