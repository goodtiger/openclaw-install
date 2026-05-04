package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/internal/shared"
	"github.com/goodtiger/openclaw-install/internal/system"
)

var channelIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

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
		return warnings, fmt.Errorf("不支持的安装模式 %s", req.Mode)
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return warnings, err
	}

	for _, channel := range req.Channels {
		if !shared.UsesBridgeProvisioner(channel.Provisioner) {
			continue
		}
		if err := validateChannelID(channel.ID); err != nil {
			return warnings, err
		}
		w.progressDetailf("为 %s 准备 bridge 运行文件", channel.Name)
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
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY
ENV NPM_CONFIG_REGISTRY=${NPM_REGISTRY}
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV NO_PROXY=${NO_PROXY}
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
        HTTP_PROXY: ${HTTP_PROXY:-}
        HTTPS_PROXY: ${HTTPS_PROXY:-}
        NO_PROXY: ${NO_PROXY:-}
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
		script := "@echo off\r\nsetlocal\r\nset \"OPENCLAW_CMD=openclaw\"\r\nwhere openclaw >nul 2>nul\r\nif errorlevel 1 (\r\n  if exist \"%APPDATA%\\npm\\openclaw.cmd\" set \"OPENCLAW_CMD=%APPDATA%\\npm\\openclaw.cmd\"\r\n)\r\ncall \"%OPENCLAW_CMD%\" gateway start\r\n"
		return os.WriteFile(filepath.Join(info.RuntimeDir, "run-openclaw.cmd"), []byte(script), 0o600)
	}

	script := "#!/usr/bin/env sh\nset -eu\nopenclaw gateway start\n"
	path := filepath.Join(info.RuntimeDir, "run-openclaw.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func writeBridgeScript(info system.Info, binaryPath, channelID string) (string, error) {
	if err := validateChannelID(channelID); err != nil {
		return "", err
	}
	name := "bridge-" + channelID + scriptExtension()
	path := filepath.Join(info.RuntimeDir, name)
	if runtime.GOOS == "windows" {
		content := fmt.Sprintf("@echo off\r\n\"%s\" bridge serve --channel \"%s\" --config \"%s\"\r\n", binaryPath, channelID, info.BridgeConfigPath)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return "", err
		}
		return path, nil
	}

	content := fmt.Sprintf("#!/usr/bin/env sh\nset -eu\nexec \"%s\" bridge serve --channel \"%s\" --config \"%s\"\n", binaryPath, channelID, info.BridgeConfigPath)
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
		return w.registerWindowsScheduledTask(ctx, info, channelID, scriptPath, stdout, stderr)
	default:
		return []string{"当前宿主机系统暂不支持自动注册 bridge 服务"}, nil
	}
}

func (w *Workflow) cleanupObsoleteChannelAssets(ctx context.Context, info system.Info, previous config.InstallState, current []config.ChannelSelection, stdout, stderr io.Writer) error {
	currentBridgeIDs := []string{}
	for _, channel := range current {
		if shared.UsesBridgeProvisioner(channel.Provisioner) {
			if err := validateChannelID(channel.ID); err != nil {
				return err
			}
			currentBridgeIDs = append(currentBridgeIDs, channel.ID)
		}
	}
	for _, channelID := range previous.ManagedChannels {
		if err := validateChannelID(channelID); err != nil {
			return err
		}
		if slices.Contains(currentBridgeIDs, channelID) {
			continue
		}

		switch info.OS {
		case "linux":
			if err := w.cleanupSystemdUserService(ctx, info, channelID, stdout, stderr); err != nil {
				return err
			}
		case "darwin":
			if err := w.cleanupLaunchdService(ctx, info, channelID, stdout, stderr); err != nil {
				return err
			}
		case "windows":
			if err := w.cleanupWindowsScheduledTask(ctx, info, channelID, stdout, stderr); err != nil {
				return err
			}
		}

		scriptPath := filepath.Join(info.RuntimeDir, "bridge-"+channelID+scriptExtension())
		if err := os.Remove(scriptPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	return nil
}

func (w *Workflow) cleanupSystemdUserService(ctx context.Context, info system.Info, channelID string, stdout, stderr io.Writer) error {
	if err := validateChannelID(channelID); err != nil {
		return err
	}
	serviceName := "openclaw-bridge-" + channelID + ".service"
	servicePath := filepath.Join(info.HomeDir, ".config", "systemd", "user", serviceName)

	if _, err := os.Stat(servicePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if system.HasCommand("systemctl") {
		_ = w.runCommand(ctx, "systemctl", []string{"--user", "disable", "--now", serviceName}, nil, "", stdout, stderr)
	}

	if err := os.Remove(servicePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if system.HasCommand("systemctl") {
		_ = w.runCommand(ctx, "systemctl", []string{"--user", "daemon-reload"}, nil, "", stdout, stderr)
	}

	return nil
}

func (w *Workflow) cleanupLaunchdService(ctx context.Context, info system.Info, channelID string, stdout, stderr io.Writer) error {
	if err := validateChannelID(channelID); err != nil {
		return err
	}
	plistPath := filepath.Join(info.HomeDir, "Library", "LaunchAgents", "ai.openclaw.bridge."+channelID+".plist")

	if _, err := os.Stat(plistPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if system.HasCommand("launchctl") {
		_ = w.runCommand(ctx, "launchctl", []string{"unload", plistPath}, nil, "", stdout, stderr)
	}

	if err := os.Remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

func (w *Workflow) registerSystemdUserService(ctx context.Context, info system.Info, channelID, scriptPath string, stdout, stderr io.Writer) ([]string, error) {
	warnings := []string{}
	if err := validateChannelID(channelID); err != nil {
		return warnings, err
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
Environment=%s

[Install]
WantedBy=default.target
`, channelID, systemdQuote(scriptPath), systemdQuote(info.RuntimeDir), systemdQuote("HOME="+info.HomeDir))

	if err := os.WriteFile(servicePath, []byte(content), 0o600); err != nil {
		return warnings, err
	}

	if !system.HasCommand("systemctl") {
		return []string{"未找到 systemctl；bridge 服务文件已生成，但尚未激活"}, nil
	}

	if err := w.runCommand(ctx, "systemctl", []string{"--user", "daemon-reload"}, nil, "", stdout, stderr); err != nil {
		warnings = append(warnings, "重新加载 systemd user daemon 失败，但 bridge 服务文件已经生成")
		return warnings, nil
	}
	if err := w.runCommand(ctx, "systemctl", []string{"--user", "enable", "--now", serviceName}, nil, "", stdout, stderr); err != nil {
		warnings = append(warnings, "启动 systemd 用户服务失败；可手动执行 systemctl --user enable --now "+serviceName)
	}
	return warnings, nil
}

func (w *Workflow) registerLaunchdService(ctx context.Context, info system.Info, channelID, scriptPath string, stdout, stderr io.Writer) ([]string, error) {
	warnings := []string{}
	if err := validateChannelID(channelID); err != nil {
		return warnings, err
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

	if !system.HasCommand("launchctl") {
		return []string{"未找到 launchctl；bridge plist 已生成，但尚未激活"}, nil
	}

	_ = w.runCommand(ctx, "launchctl", []string{"unload", plistPath}, nil, "", stdout, stderr)
	if err := w.runCommand(ctx, "launchctl", []string{"load", plistPath}, nil, "", stdout, stderr); err != nil {
		warnings = append(warnings, "加载 launchd agent 失败；可手动执行 `launchctl load "+plistPath+"`")
	}
	return warnings, nil
}

func (w *Workflow) registerWindowsScheduledTask(ctx context.Context, info system.Info, channelID, scriptPath string, stdout, stderr io.Writer) ([]string, error) {
	warnings := []string{}
	if err := validateChannelID(channelID); err != nil {
		return warnings, err
	}
	if !system.HasCommand("schtasks") {
		return []string{"未找到 schtasks；bridge 启动脚本已生成，但尚未注册计划任务"}, nil
	}

	taskName := "OpenClaw-Bridge-" + channelID
	cmdPath := filepath.Join(info.RuntimeDir, "bridge-"+channelID+".cmd")

	if _, err := os.Stat(cmdPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return warnings, fmt.Errorf("bridge 脚本不存在: %s", cmdPath)
		}
		return warnings, err
	}

	if err := w.runCommand(ctx, "schtasks", []string{
		"/Create",
		"/TN", taskName,
		"/TR", `start /B "" "` + cmdPath + `"`,
		"/SC", "ONLOGON",
		"/RL", "LIMITED",
		"/F",
	}, nil, "", stdout, stderr); err != nil {
		warnings = append(warnings, "创建计划任务失败；可手动执行 schtasks /Create /TN \""+taskName+"\" /TR \"..."+`" /SC ONLOGON /RL LIMITED`)
	}
	return warnings, nil
}

func (w *Workflow) cleanupWindowsScheduledTask(ctx context.Context, info system.Info, channelID string, stdout, stderr io.Writer) error {
	if err := validateChannelID(channelID); err != nil {
		return err
	}

	taskName := "OpenClaw-Bridge-" + channelID

	if system.HasCommand("schtasks") {
		_ = w.runCommand(ctx, "schtasks", []string{"/Delete", "/TN", taskName, "/F"}, nil, "", stdout, stderr)
	}

	return nil
}

func validateChannelID(channelID string) error {
	if !channelIDPattern.MatchString(channelID) {
		return fmt.Errorf("无效的 channel ID %q：仅允许字母、数字、下划线和连字符", channelID)
	}
	return nil
}

func systemdQuote(value string) string {
	return strconv.Quote(value)
}
