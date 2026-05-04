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

	warnings, err := workflow.provisionPluginChannel(context.Background(), info, ModeNative, channel, false, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("provisionPluginChannel() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
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

func TestSyncChannelsReconcilesManagedPluginChannel(t *testing.T) {
	openClawPath := writeFakeOpenClawBinary(t)

	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{
		Channels: []presets.ChannelPreset{
			{
				ID:              "qq",
				Name:            "QQ（qqbot 插件）",
				Driver:          "qqbot",
				Provisioner:     "openclaw-plugin",
				PluginPackage:   "@sliverp/qqbot@latest",
				OpenClawChannel: "qqbot",
			},
		},
	}, executor)

	req := Request{
		Mode:        ModeNative,
		SkipInstall: true,
		Channels: []config.ChannelSelection{
			{
				ID:              "qq",
				Name:            "QQ（qqbot 插件）",
				Driver:          "qqbot",
				Provisioner:     "openclaw-plugin",
				PluginPackage:   "@sliverp/qqbot@latest",
				OpenClawChannel: "qqbot",
				TokenFields:     []string{"app_id", "app_secret"},
				Fields: map[string]string{
					"app_id":     "123456",
					"app_secret": "topsecret",
				},
			},
		},
	}
	previous := config.InstallState{
		Version:         "0.1.0",
		ManagedChannels: []string{"qq"},
	}

	warnings, err := workflow.syncChannels(context.Background(), system.Info{OS: "linux"}, req, previous, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("syncChannels() error = %v", err)
	}

	removeIdx := commandIndex(executor.commands, openClawPath+" channels remove --channel qqbot")
	addIdx := commandIndex(executor.commands, openClawPath+" channels add --channel qqbot --token 123456:topsecret")
	restartIdx := commandIndex(executor.commands, openClawPath+" gateway restart")

	if commandIndex(executor.commands, openClawPath+" plugins install @sliverp/qqbot@latest") == -1 {
		t.Fatalf("expected plugin install command, commands = %#v", executor.commands)
	}
	if removeIdx == -1 {
		t.Fatalf("expected plugin channel reset command, commands = %#v", executor.commands)
	}
	if addIdx == -1 {
		t.Fatalf("expected plugin channel add command, commands = %#v", executor.commands)
	}
	if restartIdx == -1 {
		t.Fatalf("expected OpenClaw restart command, commands = %#v", executor.commands)
	}
	if removeIdx > addIdx {
		t.Fatalf("expected channel removal before re-add, commands = %#v", executor.commands)
	}
	if addIdx > restartIdx {
		t.Fatalf("expected restart after channel add, commands = %#v", executor.commands)
	}
	if !containsWarning(warnings, "QQ（qqbot 插件） 已通过 OpenClaw 插件通道 qqbot 完成配置") {
		t.Fatalf("expected success warning, warnings = %#v", warnings)
	}
}

func TestSyncChannelsRemovesDeselectedManagedPluginChannel(t *testing.T) {
	openClawPath := writeFakeOpenClawBinary(t)

	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{
		Channels: []presets.ChannelPreset{
			{
				ID:              "qq",
				Name:            "QQ（qqbot 插件）",
				Driver:          "qqbot",
				Provisioner:     "openclaw-plugin",
				PluginPackage:   "@sliverp/qqbot@latest",
				OpenClawChannel: "qqbot",
			},
		},
	}, executor)

	req := Request{
		Mode:        ModeNative,
		SkipInstall: true,
	}
	previous := config.InstallState{
		Version:         "0.1.0",
		ManagedChannels: []string{"qq"},
	}

	warnings, err := workflow.syncChannels(context.Background(), system.Info{OS: "linux"}, req, previous, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("syncChannels() error = %v", err)
	}

	if !containsCommand(executor.commands, openClawPath+" channels remove --channel qqbot") {
		t.Fatalf("expected plugin channel removal command, commands = %#v", executor.commands)
	}
	if containsCommand(executor.commands, openClawPath+" channels add --channel qqbot") {
		t.Fatalf("did not expect plugin channel add command, commands = %#v", executor.commands)
	}
	if !containsCommand(executor.commands, openClawPath+" gateway restart") {
		t.Fatalf("expected OpenClaw restart after channel removal, commands = %#v", executor.commands)
	}
	if !containsWarning(warnings, "QQ（qqbot 插件） 已从 OpenClaw 通道配置中移除") {
		t.Fatalf("expected removal warning, warnings = %#v", warnings)
	}
}

func writeFakeOpenClawBinary(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	openClawPath := filepath.Join(binDir, "openclaw")
	if err := os.WriteFile(openClawPath, []byte("#!/usr/bin/env sh\n"), 0o700); err != nil {
		t.Fatalf("WriteFile openClawPath: %v", err)
	}
	t.Setenv("PATH", binDir)
	return openClawPath
}

func commandIndex(commands []string, want string) int {
	for idx, command := range commands {
		if strings.Contains(command, want) {
			return idx
		}
	}
	return -1
}

func containsWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, want) {
			return true
		}
	}
	return false
}
