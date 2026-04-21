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

func TestProvisionPluginChannelConfigSetWritesCorrectCommands(t *testing.T) {
	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	binDir := t.TempDir()
	openclawBinary := filepath.Join(binDir, "openclaw")
	if err := os.WriteFile(openclawBinary, []byte("#!/usr/bin/env sh\necho ok\n"), 0o700); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("PATH", binDir)

	info := system.Info{
		OS:         "darwin",
		HomeDir:    t.TempDir(),
		RuntimeDir: t.TempDir(),
	}

	channel := config.ChannelSelection{
		ID:              "dingtalk",
		Name:            "钉钉（DingTalk 插件）",
		Driver:          "dingtalk",
		Provisioner:     "openclaw-plugin",
		PluginPackage:   "@soimy/dingtalk@latest",
		OpenClawChannel: "dingtalk",
		ConfigMethod:    "config_set",
		Fields: map[string]string{
			"clientId":     "ding-test-id",
			"clientSecret": "test-secret-value",
		},
		RequiredFields: []config.CredentialField{
			{Key: "clientId", Label: "Client ID", Secret: false, EnvKey: "OPENCLAW_DINGTALK_CLIENT_ID"},
			{Key: "clientSecret", Label: "Client Secret", Secret: true, EnvKey: "OPENCLAW_DINGTALK_CLIENT_SECRET"},
		},
		DMPolicy:    "open",
		GroupPolicy: "allowlist",
	}

	err := workflow.provisionPluginChannel(context.Background(), info, ModeNative, channel, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("provisionPluginChannel() error = %v", err)
	}

	// Verify commands were issued in the correct order
	cmds := executor.commands
	if len(cmds) == 0 {
		t.Fatal("expected commands to be recorded, got none")
	}

	allCmds := strings.Join(cmds, "|")

	// Check plugin install
	if !strings.Contains(allCmds, "plugins install") {
		t.Errorf("expected 'plugins install' command, got: %v", cmds)
	}

	// Check channel enabled
	if !strings.Contains(allCmds, "config set channels.dingtalk.enabled true") {
		t.Errorf("expected 'config set channels.dingtalk.enabled true', got: %v", cmds)
	}

	// Check credential fields
	if !strings.Contains(allCmds, "config set channels.dingtalk.clientId ding-test-id") {
		t.Errorf("expected clientId config, got: %v", cmds)
	}
	if !strings.Contains(allCmds, "config set channels.dingtalk.clientSecret test-secret-value") {
		t.Errorf("expected clientSecret config, got: %v", cmds)
	}

	// Check policies
	if !strings.Contains(allCmds, "config set channels.dingtalk.dmPolicy open") {
		t.Errorf("expected dmPolicy config, got: %v", cmds)
	}
	if !strings.Contains(allCmds, "config set channels.dingtalk.groupPolicy allowlist") {
		t.Errorf("expected groupPolicy config, got: %v", cmds)
	}
}
