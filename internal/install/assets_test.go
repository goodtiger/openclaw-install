package install

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

	if _, err := os.Stat(servicePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected service file to be removed, stat err = %v", err)
	}

	if _, err := os.Stat(scriptPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected bridge script to be removed, stat err = %v", err)
	}
}

func TestWriteBridgeScriptQuotesChannelArgument(t *testing.T) {
	info := system.Info{RuntimeDir: t.TempDir(), BridgeConfigPath: "/tmp/bridge config.json"}

	scriptPath, err := writeBridgeScript(info, "/tmp/openclaw install", "feishu")
	if err != nil {
		t.Fatalf("writeBridgeScript() error = %v", err)
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile scriptPath: %v", err)
	}

	if !strings.Contains(string(content), `--channel "feishu"`) {
		t.Fatalf("expected quoted channel argument, got:\n%s", content)
	}
}

func TestWriteBridgeScriptRejectsInvalidChannelID(t *testing.T) {
	info := system.Info{RuntimeDir: t.TempDir(), BridgeConfigPath: "/tmp/bridge.json"}

	if _, err := writeBridgeScript(info, "/tmp/openclaw", "../bad;rm -rf"); err == nil {
		t.Fatal("expected invalid channel ID to be rejected")
	}
}

func joinArgs(args []string) string {
	return strings.Join(args, " ")
}

func TestWriteDockerAssets(t *testing.T) {
	// Test the default configuration
	info := system.Info{
		RuntimeDir:   t.TempDir(),
		OpenClawHome: "/home/user/.openclaw",
	}

	if err := writeDockerAssets(info, nil); err != nil {
		t.Fatalf("writeDockerAssets() error = %v", err)
	}

	// Check that files were created
	requiredFiles := []string{
		filepath.Join(info.RuntimeDir, "Dockerfile.openclaw"),
		filepath.Join(info.RuntimeDir, "compose.yaml"),
		filepath.Join(info.RuntimeDir, ".env"),
	}

	for _, file := range requiredFiles {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("expected file %s to exist, stat error = %v", file, err)
		}
	}

	// Check content of each file
	dockerfileContent, err := os.ReadFile(requiredFiles[0])
	if err != nil {
		t.Fatalf("failed to read Dockerfile: %v", err)
	}

	composeContent, err := os.ReadFile(requiredFiles[1])
	if err != nil {
		t.Fatalf("failed to read compose.yaml: %v", err)
	}

	envContent, err := os.ReadFile(requiredFiles[2])
	if err != nil {
		t.Fatalf("failed to read .env: %v", err)
	}

	if !strings.Contains(string(dockerfileContent), "FROM ${NODE_IMAGE}") {
		t.Errorf("Dockerfile missing FROM directive with NODE_IMAGE: %s", dockerfileContent)
	}

	if !strings.Contains(string(composeContent), "services:") {
		t.Errorf("compose.yaml missing services header: %s", composeContent)
	}

	if !strings.Contains(string(envContent), "OPENCLAW_HOME=/home/user/.openclaw") {
		t.Errorf("env file missing OPENCLAW_HOME: %s", envContent)
	}
}

func TestWriteDockerAssetsWithMirrors(t *testing.T) {
	// Test with mirrors configuration
	info := system.Info{
		RuntimeDir:   t.TempDir(),
		OpenClawHome: "/home/user/.openclaw",
	}

	mirrors := MirrorSelection{
		"docker_image": {
			Name:    "aliyun",
			BaseURL: "registry.aliyuncs.com/google_containers/node",
		},
		"npm_registry": {
			Name:    "taobao",
			BaseURL: "https://registry.npmmirror.com",
		},
	}

	if err := writeDockerAssets(info, mirrors); err != nil {
		t.Fatalf("writeDockerAssets() error = %v", err)
	}

	// Check content includes mirror values
	envPath := filepath.Join(info.RuntimeDir, ".env")
	envContent, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("failed to read .env: %v", err)
	}

	envStr := string(envContent)
	if !strings.Contains(envStr, "NODE_IMAGE=registry.aliyuncs.com/google_containers/node") {
		t.Errorf("env file missing mirror node image: %s", envStr)
	}

	if !strings.Contains(envStr, "NPM_REGISTRY=https://registry.npmmirror.com") {
		t.Errorf("env file missing mirror npm registry: %s", envStr)
	}
}

func TestSystemdQuote(t *testing.T) {
	tests := []struct {
		input       string
		expected    string
		description string
	}{
		{"/home/user", `"/home/user"`, "simple path"},
		{"/home/user/with spaces", `"/home/user/with spaces"`, "path with spaces"},
		{`/path/with"quotes`, `"/path/with\"quotes"`, "path with quotes"},
		{``, `""`, "empty string"},
		{`/some$path`, `"/some$path"`, "path with dollar sign"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := systemdQuote(tt.input)
			if result != tt.expected {
				t.Errorf("systemdQuote(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRegisterBridgeService(t *testing.T) {
	tests := []struct {
		osName              string
		serviceFunctionName string
	}{
		{"linux", "registerSystemdUserService"},
		{"darwin", "registerLaunchdService"},
		{"windows", "registerWindowsScheduledTask"},
		{"freebsd", "unknown OS"},
	}

	for _, tt := range tests {
		t.Run(tt.osName, func(t *testing.T) {
			homeDir := t.TempDir()
			runtimeDir := filepath.Join(homeDir, ".openclaw", "runtime")
			if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
				t.Fatalf("MkdirAll runtimeDir: %v", err)
			}

			scriptExt := ".sh"
			if tt.osName == "windows" {
				scriptExt = ".cmd"
			}
			scriptPath := filepath.Join(runtimeDir, "bridge-test-channel"+scriptExt)
			if err := os.WriteFile(scriptPath, []byte("@echo off\r\n"), 0o600); err != nil {
				t.Fatalf("WriteFile scriptPath: %v", err)
			}

			info := system.Info{
				OS:         tt.osName,
				HomeDir:    homeDir,
				RuntimeDir: runtimeDir,
			}

			executor := &recordingExecutor{}
			workflow := NewWorkflow(presets.Bundle{}, executor)

			warnings, err := workflow.registerBridgeService(context.Background(), info, "test-channel", scriptPath, io.Discard, io.Discard)

			if err != nil && tt.osName != "linux" && tt.osName != "darwin" {
				t.Errorf("registerBridgeService() error = %v, want no error for %s", err, tt.osName)
			}

			if tt.osName == "linux" || tt.osName == "darwin" {
				if err != nil {
					t.Errorf("registerBridgeService() error = %v for %s, want nil", err, tt.osName)
				}
			} else if tt.osName == "windows" {
				if len(warnings) == 0 {
					t.Error("registerBridgeService() expected warning for Windows (no schtasks in test), got none")
				} else if !strings.Contains(warnings[0], "未找到 schtasks") && !strings.Contains(warnings[0], "创建计划任务失败") {
					t.Errorf("registerBridgeService() wrong warning for Windows, got: %v", warnings[0])
				}
			} else {
				if len(warnings) == 0 {
					t.Error("registerBridgeService() expected warning for unknown OS, got none")
				} else if !strings.Contains(warnings[0], "当前宿主机系统暂不支持") {
					t.Errorf("registerBridgeService() wrong warning for unknown OS, got: %v", warnings[0])
				}
			}
		})
	}
}

func TestRegisterSystemdUserService(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping systemd-specific test on non-linux platform")
	}
	homeDir := t.TempDir()
	info := system.Info{
		OS:         "linux",
		HomeDir:    homeDir,
		RuntimeDir: filepath.Join(homeDir, ".openclaw", "runtime"),
	}

	// Create the systemd user directory beforehand since we know it's needed
	serviceDir := filepath.Join(homeDir, ".config", "systemd", "user")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("Failed to pre-create systemd user directory: %v", err)
	}

	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	// This will try to call systemctl commands but shouldn't fail to create the service file itself
	warnings, err := workflow.registerSystemdUserService(context.Background(), info, "test-channel", "/path/to/script", io.Discard, io.Discard)
	if err != nil {
		// Only consider as true error if the problem is not systemctl-related
		// systemctl failures should only produce warnings, not errors
		t.Fatalf("registerSystemdUserService returned error: %v", err)
	}

	servicePath := filepath.Join(serviceDir, "openclaw-bridge-test-channel.service")
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		t.Fatalf("Systemd service file at %s was not created, error: %v", servicePath, err)
	}

	// Read the service file content
	serviceContent, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("Failed to read service file: %v", err)
	}

	serviceStr := string(serviceContent)
	if !strings.Contains(serviceStr, "[Unit]") {
		t.Error("Service file missing [Unit] section")
	}

	if !strings.Contains(serviceStr, "[Service]") {
		t.Error("Service file missing [Service] section")
	}

	if !strings.Contains(serviceStr, "ExecStart="+systemdQuote("/path/to/script")) {
		t.Error("Service file missing correct ExecStart path")
	}

	// If systemctl is not available, we should get warnings about it
	if len(warnings) == 0 {
		t.Log("No systemctl warnings generated - systemctl might be available in test environment")
	}
}

func TestRegisterLaunchdService(t *testing.T) {
	homeDir := t.TempDir()
	info := system.Info{
		OS:         "darwin",
		HomeDir:    homeDir,
		RuntimeDir: filepath.Join(homeDir, ".openclaw", "runtime"),
	}

	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	_, err := workflow.registerLaunchdService(context.Background(), info, "test-channel", "/path/to/script", io.Discard, io.Discard)

	// Should not return error for missing launchctl, only warning
	if err != nil {
		t.Fatalf("registerLaunchdService() unexpected error = %v", err)
	}

	// Service file should be created even without launchctl
	launchAgentDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	plistPath := filepath.Join(launchAgentDir, "ai.openclaw.bridge.test-channel.plist")

	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		t.Fatalf("Expected plist file at %s, but it was not created", plistPath)
	}

	// Read the plist file content
	plistContent, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("Failed to read plist file: %v", err)
	}

	plistStr := string(plistContent)
	if !strings.Contains(plistStr, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>") {
		t.Error("Plist file missing XML declaration")
	}

	if !strings.Contains(plistStr, "ai.openclaw.bridge.test-channel") {
		t.Error("Plist file missing correct label")
	}

	if !strings.Contains(plistStr, "<string>/path/to/script</string>") {
		t.Error("Plist file missing correct ProgramArguments")
	}
}

func TestCleanupLaunchdService(t *testing.T) {
	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	homeDir := t.TempDir()
	info := system.Info{HomeDir: homeDir}

	// Create LaunchAgent file to test removal
	launchAgentDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentDir, 0o755); err != nil {
		t.Fatalf("Failed to create LaunchAgent dir: %v", err)
	}

	plistPath := filepath.Join(launchAgentDir, "ai.openclaw.bridge.test-channel.plist")
	if err := os.WriteFile(plistPath, []byte("<plist>test</plist>"), 0o600); err != nil {
		t.Fatalf("Failed to create plist file: %v", err)
	}

	if err := workflow.cleanupLaunchdService(context.Background(), info, "test-channel", io.Discard, io.Discard); err != nil {
		t.Fatalf("cleanupLaunchdService() error = %v", err)
	}

	// Verify plist file was removed
	if _, err := os.Stat(plistPath); !os.IsNotExist(err) {
		t.Fatalf("Expected service file to be removed, but it still exists, stat err = %v", err)
	}
}

func TestCleanupLaunchdServiceSkipsMissingFile(t *testing.T) {
	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	homeDir := t.TempDir()
	info := system.Info{HomeDir: homeDir}

	// Test cleanup of non-existent file should not error
	if err := workflow.cleanupLaunchdService(context.Background(), info, "missing-channel", io.Discard, io.Discard); err != nil {
		t.Fatalf("cleanupLaunchdService() error = %v, expected no error for missing file", err)
	}
}

func TestRegisterWindowsScheduledTask(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping Windows-specific test on non-Windows platform")
	}
	homeDir := t.TempDir()
	runtimeDir := filepath.Join(homeDir, ".openclaw", "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll runtimeDir: %v", err)
	}

	scriptPath := filepath.Join(runtimeDir, "bridge-test-channel.cmd")
	if err := os.WriteFile(scriptPath, []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatalf("WriteFile scriptPath: %v", err)
	}

	info := system.Info{
		OS:         "windows",
		HomeDir:    homeDir,
		RuntimeDir: runtimeDir,
	}

	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	warnings, err := workflow.registerWindowsScheduledTask(context.Background(), info, "test-channel", scriptPath, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("registerWindowsScheduledTask() error = %v", err)
	}

	if len(executor.commands) == 0 {
		t.Error("registerWindowsScheduledTask() expected schtasks command to be recorded")
	}

	found := false
	for _, cmd := range executor.commands {
		if strings.Contains(cmd, "schtasks") && strings.Contains(cmd, "/Create") && strings.Contains(cmd, "OpenClaw-Bridge-test-channel") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("registerWindowsScheduledTask() expected schtasks /Create with OpenClaw-Bridge-test-channel, commands = %#v", executor.commands)
	}

	_ = warnings
}

func TestCleanupWindowsScheduledTask(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping Windows-specific test on non-Windows platform")
	}
	homeDir := t.TempDir()
	info := system.Info{
		OS:      "windows",
		HomeDir: homeDir,
	}

	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	if err := workflow.cleanupWindowsScheduledTask(context.Background(), info, "test-channel", io.Discard, io.Discard); err != nil {
		t.Fatalf("cleanupWindowsScheduledTask() error = %v", err)
	}

	if len(executor.commands) == 0 {
		t.Error("cleanupWindowsScheduledTask() expected schtasks command to be recorded")
	}

	found := false
	for _, cmd := range executor.commands {
		if strings.Contains(cmd, "schtasks") && strings.Contains(cmd, "/Delete") && strings.Contains(cmd, "OpenClaw-Bridge-test-channel") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cleanupWindowsScheduledTask() expected schtasks /Delete with OpenClaw-Bridge-test-channel, commands = %#v", executor.commands)
	}
}

func TestCleanupWindowsScheduledTaskSkipsMissingTask(t *testing.T) {
	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	homeDir := t.TempDir()
	info := system.Info{HomeDir: homeDir}

	if err := workflow.cleanupWindowsScheduledTask(context.Background(), info, "missing-channel", io.Discard, io.Discard); err != nil {
		t.Fatalf("cleanupWindowsScheduledTask() error = %v, expected no error for missing task", err)
	}
}
