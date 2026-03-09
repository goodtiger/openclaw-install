package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/internal/system"
)

func (w *Workflow) writeAssets(ctx context.Context, info system.Info, req Request, previous config.InstallState, mirrors MirrorSelection, stdout, stderr io.Writer) ([]string, error) {
	warnings := []string{}

	if err := config.EnsureDir(info.RuntimeDir); err != nil {
		return warnings, err
	}

	if err := w.cleanupObsoleteChannelAssets(ctx, info, previous, req.Channels, stdout, stderr); err != nil {
		warnings = append(warnings, err.Error())
	}

	switch req.Mode {
	case ModeDocker:
		if err := writeDockerAssets(info, mirrors); err != nil {
			return warnings, err
		}
	case ModeNative:
		if err := writeNativeAssets(info); err != nil {
			return warnings, err
		}
	default:
		return warnings, fmt.Errorf("unsupported mode %s", req.Mode)
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return warnings, err
	}

	for _, channel := range req.Channels {
		if !usesBridgeProvisioner(channel.Provisioner) {
			continue
		}
		scriptPath, err := writeBridgeScript(info, binaryPath, channel.ID)
		if err != nil {
			return warnings, err
		}
		channelWarnings, err := w.registerBridgeService(ctx, info, channel.ID, scriptPath, stdout, stderr)
		warnings = append(warnings, channelWarnings...)
		if err != nil {
			warnings = append(warnings, err.Error())
		}
	}

	return warnings, nil
}

func writeDockerAssets(info system.Info, mirrors MirrorSelection) error {
	if err := config.EnsureDir(info.RuntimeDir); err != nil {
		return err
	}

	nodeImage := "node:22-bullseye"
	if candidate, ok := mirrors["docker_image"]; ok && candidate.BaseURL != "" {
		nodeImage = candidate.BaseURL
	}

	npmRegistry := "https://registry.npmjs.org"
	if candidate, ok := mirrors["npm_registry"]; ok && candidate.BaseURL != "" {
		npmRegistry = candidate.BaseURL
	}

	dockerfile := `ARG NODE_IMAGE=node:22-bullseye
FROM ${NODE_IMAGE}
ARG NPM_REGISTRY=https://registry.npmjs.org
ENV NPM_CONFIG_REGISTRY=${NPM_REGISTRY}
RUN npm config set registry "${NPM_CONFIG_REGISTRY}" && npm install -g openclaw
EXPOSE 18789
WORKDIR /root/.openclaw
CMD ["sh", "-lc", "openclaw gateway start --foreground"]
`

	composeFile := `services:
  openclaw:
    build:
      context: .
      dockerfile: Dockerfile.openclaw
      args:
        NODE_IMAGE: ${NODE_IMAGE}
        NPM_REGISTRY: ${NPM_REGISTRY}
    ports:
      - "18789:18789"
    volumes:
      - ${OPENCLAW_HOME}:/root/.openclaw
    extra_hosts:
      - "host.docker.internal:host-gateway"
    restart: unless-stopped
`

	envFile := fmt.Sprintf("OPENCLAW_HOME=%s\nNODE_IMAGE=%s\nNPM_REGISTRY=%s\n", filepath.ToSlash(info.OpenClawHome), nodeImage, npmRegistry)

	if err := os.WriteFile(filepath.Join(info.RuntimeDir, "Dockerfile.openclaw"), []byte(dockerfile), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(info.RuntimeDir, "compose.yaml"), []byte(composeFile), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(info.RuntimeDir, ".env"), []byte(envFile), 0o600); err != nil {
		return err
	}
	return nil
}

func writeNativeAssets(info system.Info) error {
	if err := config.EnsureDir(info.RuntimeDir); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return os.WriteFile(filepath.Join(info.RuntimeDir, "run-openclaw.cmd"), []byte("@echo off\r\nopenclaw gateway start\r\n"), 0o600)
	}

	script := "#!/usr/bin/env sh\nset -eu\nopenclaw gateway start\n"
	path := filepath.Join(info.RuntimeDir, "run-openclaw.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func writeBridgeScript(info system.Info, binaryPath, channelID string) (string, error) {
	name := "bridge-" + channelID + scriptExtension()
	path := filepath.Join(info.RuntimeDir, name)
	if runtime.GOOS == "windows" {
		content := fmt.Sprintf("@echo off\r\n\"%s\" bridge serve --channel %s --config \"%s\"\r\n", binaryPath, channelID, info.BridgeConfigPath)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return "", err
		}
		return path, nil
	}

	content := fmt.Sprintf("#!/usr/bin/env sh\nset -eu\nexec \"%s\" bridge serve --channel %s --config \"%s\"\n", binaryPath, channelID, info.BridgeConfigPath)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return "", err
	}
	return path, nil
}

func (w *Workflow) registerBridgeService(ctx context.Context, info system.Info, channelID, scriptPath string, stdout, stderr io.Writer) ([]string, error) {
	switch info.OS {
	case "linux":
		return w.registerSystemdUserService(ctx, info, channelID, scriptPath, stdout, stderr)
	case "darwin":
		return w.registerLaunchdService(ctx, info, channelID, scriptPath, stdout, stderr)
	case "windows":
		return []string{"Windows bridge scripts were generated, but service registration is manual in v1"}, nil
	default:
		return []string{"unsupported host OS for automatic bridge service registration"}, nil
	}
}

func (w *Workflow) cleanupObsoleteChannelAssets(ctx context.Context, info system.Info, previous config.InstallState, current []config.ChannelSelection, stdout, stderr io.Writer) error {
	currentBridgeIDs := []string{}
	for _, channel := range current {
		if usesBridgeProvisioner(channel.Provisioner) {
			currentBridgeIDs = append(currentBridgeIDs, channel.ID)
		}
	}
	for _, channelID := range previous.ManagedChannels {
		if slices.Contains(currentBridgeIDs, channelID) {
			continue
		}

		switch info.OS {
		case "linux":
			if system.HasCommand("systemctl") {
				_ = w.Executor.Run(ctx, "systemctl", []string{"--user", "disable", "--now", "openclaw-bridge-" + channelID + ".service"}, nil, "", stdout, stderr)
			}
			_ = os.Remove(filepath.Join(info.HomeDir, ".config", "systemd", "user", "openclaw-bridge-"+channelID+".service"))
		case "darwin":
			plistPath := filepath.Join(info.HomeDir, "Library", "LaunchAgents", "ai.openclaw.bridge."+channelID+".plist")
			if system.HasCommand("launchctl") {
				_ = w.Executor.Run(ctx, "launchctl", []string{"unload", plistPath}, nil, "", stdout, stderr)
			}
			_ = os.Remove(plistPath)
		}

		_ = os.Remove(filepath.Join(info.RuntimeDir, "bridge-"+channelID+scriptExtension()))
	}

	return nil
}

func (w *Workflow) registerSystemdUserService(ctx context.Context, info system.Info, channelID, scriptPath string, stdout, stderr io.Writer) ([]string, error) {
	warnings := []string{}
	if !system.HasCommand("systemctl") {
		return []string{"systemctl not found; bridge service file was generated but not activated"}, nil
	}

	serviceDir := filepath.Join(info.HomeDir, ".config", "systemd", "user")
	if err := config.EnsureDir(serviceDir); err != nil {
		return warnings, err
	}
	serviceName := "openclaw-bridge-" + channelID + ".service"
	servicePath := filepath.Join(serviceDir, serviceName)

	content := fmt.Sprintf(`[Unit]
Description=OpenClaw Bridge (%s)

[Service]
Type=simple
ExecStart=%s
WorkingDirectory=%s
Restart=always
RestartSec=3
Environment=HOME=%s

[Install]
WantedBy=default.target
`, channelID, scriptPath, info.RuntimeDir, info.HomeDir)

	if err := os.WriteFile(servicePath, []byte(content), 0o600); err != nil {
		return warnings, err
	}

	if err := w.Executor.Run(ctx, "systemctl", []string{"--user", "daemon-reload"}, nil, "", stdout, stderr); err != nil {
		warnings = append(warnings, "failed to reload systemd user daemon; bridge service file is still available")
		return warnings, nil
	}
	if err := w.Executor.Run(ctx, "systemctl", []string{"--user", "enable", "--now", serviceName}, nil, "", stdout, stderr); err != nil {
		warnings = append(warnings, "failed to start systemd user service; start it manually with systemctl --user enable --now "+serviceName)
	}
	return warnings, nil
}

func (w *Workflow) registerLaunchdService(ctx context.Context, info system.Info, channelID, scriptPath string, stdout, stderr io.Writer) ([]string, error) {
	warnings := []string{}
	if !system.HasCommand("launchctl") {
		return []string{"launchctl not found; bridge plist was generated but not activated"}, nil
	}

	launchAgentDir := filepath.Join(info.HomeDir, "Library", "LaunchAgents")
	if err := config.EnsureDir(launchAgentDir); err != nil {
		return warnings, err
	}

	plistName := "ai.openclaw.bridge." + channelID + ".plist"
	plistPath := filepath.Join(launchAgentDir, plistName)
	logPath := filepath.Join(info.RuntimeDir, "bridge-"+channelID+".log")

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>WorkingDirectory</key>
  <string>%s</string>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, strings.TrimSuffix(plistName, ".plist"), scriptPath, info.RuntimeDir, logPath, logPath)

	if err := os.WriteFile(plistPath, []byte(plist), 0o600); err != nil {
		return warnings, err
	}

	_ = w.Executor.Run(ctx, "launchctl", []string{"unload", plistPath}, nil, "", stdout, stderr)
	if err := w.Executor.Run(ctx, "launchctl", []string{"load", plistPath}, nil, "", stdout, stderr); err != nil {
		warnings = append(warnings, "failed to load launchd agent; run `launchctl load "+plistPath+"` manually")
	}
	return warnings, nil
}
