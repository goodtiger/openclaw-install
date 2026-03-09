package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/internal/system"
)

func (w *Workflow) syncChannels(ctx context.Context, info system.Info, req Request, previous config.InstallState, stdout, stderr io.Writer) ([]string, error) {
	warnings := []string{}
	provisionedPluginChannels := false

	currentIDs := channelIDs(req.Channels)
	for _, channelID := range previous.ManagedChannels {
		if slices.Contains(currentIDs, channelID) {
			continue
		}
		if preset, ok := w.Presets.ChannelByID(channelID); ok && !usesBridgeProvisioner(preset.Provisioner) {
			warnings = append(warnings, fmt.Sprintf("%s was previously configured through an OpenClaw plugin; removal is not automated. Remove it manually if needed.", preset.Name))
		}
	}

	for _, channel := range req.Channels {
		if usesBridgeProvisioner(channel.Provisioner) {
			continue
		}

		if req.SkipInstall {
			if previous.Version == "" {
				warnings = append(warnings, fmt.Sprintf("%s plugin provisioning skipped because this looks like a config-only reconfigure without an existing OpenClaw install state.", channel.Name))
				continue
			}
			if shouldSkipPluginProvisioning(info, req.Mode) {
				warnings = append(warnings, fmt.Sprintf("%s plugin provisioning skipped because OpenClaw is not available yet; rerun install or reconfigure after OpenClaw is ready.", channel.Name))
				continue
			}
		}

		if err := w.provisionPluginChannel(ctx, info, req.Mode, channel, stdout, stderr); err != nil {
			return warnings, err
		}
		provisionedPluginChannels = true
		warnings = append(warnings, fmt.Sprintf("%s configured via OpenClaw plugin channel %s", channel.Name, valueOrDefault(channel.OpenClawChannel, channel.Driver)))
	}

	if provisionedPluginChannels {
		if err := w.restartOpenClaw(ctx, info, req.Mode, stdout, stderr); err != nil {
			warnings = append(warnings, fmt.Sprintf("plugin-backed channels were configured, but OpenClaw restart failed: %v", err))
		}
	}

	return warnings, nil
}

func (w *Workflow) provisionPluginChannel(ctx context.Context, info system.Info, mode Mode, channel config.ChannelSelection, stdout, stderr io.Writer) error {
	pluginPackage := strings.TrimSpace(channel.PluginPackage)
	if pluginPackage == "" {
		return fmt.Errorf("%s is missing plugin package metadata", channel.Name)
	}

	if err := w.runOpenClawCommand(ctx, info, mode, []string{"plugins", "install", pluginPackage}, stdout, stderr); err != nil {
		return fmt.Errorf("install plugin for %s: %w", channel.Name, err)
	}

	channelName := valueOrDefault(channel.OpenClawChannel, channel.Driver)
	args := []string{"channels", "add", "--channel", channelName}

	token := pluginChannelToken(channel)
	if token != "" {
		args = append(args, "--token", token)
	}

	if err := w.runOpenClawCommand(ctx, info, mode, args, stdout, stderr); err != nil {
		return fmt.Errorf("configure channel %s: %w", channel.Name, err)
	}

	return nil
}

func pluginChannelToken(channel config.ChannelSelection) string {
	if len(channel.TokenFields) == 0 {
		return ""
	}

	parts := make([]string, 0, len(channel.TokenFields))
	for _, field := range channel.TokenFields {
		value := strings.TrimSpace(channel.Fields[field])
		if value == "" {
			return ""
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, ":")
}

func (w *Workflow) runOpenClawCommand(ctx context.Context, info system.Info, mode Mode, args []string, stdout, stderr io.Writer) error {
	switch mode {
	case ModeNative:
		return w.Executor.Run(ctx, "openclaw", args, nil, "", stdout, stderr)
	case ModeDocker:
		if err := w.ensureDockerGatewayRunning(ctx, info, stdout, stderr); err != nil {
			return err
		}
		cmd, composeArgs, err := composeInvocation()
		if err != nil {
			return err
		}
		composeArgs = append(composeArgs, "-f", filepath.Join(info.RuntimeDir, "compose.yaml"), "exec", "-T", "openclaw", "openclaw")
		composeArgs = append(composeArgs, args...)
		return w.Executor.Run(ctx, cmd, composeArgs, nil, info.RuntimeDir, stdout, stderr)
	default:
		return fmt.Errorf("unsupported install mode %s", mode)
	}
}

func (w *Workflow) ensureDockerGatewayRunning(ctx context.Context, info system.Info, stdout, stderr io.Writer) error {
	cmd, composeArgs, err := composeInvocation()
	if err != nil {
		return err
	}
	composeArgs = append(composeArgs, "-f", filepath.Join(info.RuntimeDir, "compose.yaml"), "up", "-d")
	return w.Executor.Run(ctx, cmd, composeArgs, nil, info.RuntimeDir, stdout, stderr)
}

func (w *Workflow) restartOpenClaw(ctx context.Context, info system.Info, mode Mode, stdout, stderr io.Writer) error {
	switch mode {
	case ModeNative:
		if !system.HasCommand("openclaw") {
			return nil
		}
		return w.Executor.Run(ctx, "openclaw", []string{"gateway", "restart"}, nil, "", stdout, stderr)
	case ModeDocker:
		cmd, composeArgs, err := composeInvocation()
		if err != nil {
			return err
		}
		composeArgs = append(composeArgs, "-f", filepath.Join(info.RuntimeDir, "compose.yaml"), "restart", "openclaw")
		return w.Executor.Run(ctx, cmd, composeArgs, nil, info.RuntimeDir, stdout, stderr)
	default:
		return nil
	}
}

func usesBridgeProvisioner(provisioner string) bool {
	return valueOrDefault(strings.TrimSpace(provisioner), "bridge") == "bridge"
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func shouldSkipPluginProvisioning(info system.Info, mode Mode) bool {
	switch mode {
	case ModeNative:
		return !system.HasCommand("openclaw")
	case ModeDocker:
		_, err := os.Stat(filepath.Join(info.RuntimeDir, "compose.yaml"))
		return err != nil
	default:
		return true
	}
}
