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
			warnings = append(warnings, fmt.Sprintf("%s 之前是通过 OpenClaw 插件配置的，当前不会自动移除；如有需要请手动删除。", preset.Name))
		}
	}

	for _, channel := range req.Channels {
		if usesBridgeProvisioner(channel.Provisioner) {
			w.progressDetailf("%s 使用宿主机 bridge 方式配置", channel.Name)
			continue
		}

		if req.SkipInstall {
			if previous.Version == "" {
				message := fmt.Sprintf("%s 的插件配置已跳过：当前像是仅改配置，且没有发现已有的 OpenClaw 安装状态。", channel.Name)
				warnings = append(warnings, message)
				w.progressDetailf(message)
				continue
			}
			if w.shouldSkipPluginProvisioning(ctx, info, req.Mode) {
				message := fmt.Sprintf("%s 的插件配置已跳过：OpenClaw 还不可用，请在 OpenClaw 就绪后重新运行 install 或 reconfigure。", channel.Name)
				warnings = append(warnings, message)
				w.progressDetailf(message)
				continue
			}
		}

		if err := w.provisionPluginChannel(ctx, info, req.Mode, channel, stdout, stderr); err != nil {
			return warnings, err
		}
		provisionedPluginChannels = true
		warnings = append(warnings, fmt.Sprintf("%s 已通过 OpenClaw 插件通道 %s 完成配置", channel.Name, valueOrDefault(channel.OpenClawChannel, channel.Driver)))
	}

	if provisionedPluginChannels {
		if err := w.restartOpenClaw(ctx, info, req.Mode, stdout, stderr); err != nil {
			warnings = append(warnings, fmt.Sprintf("插件型通道已配置，但重启 OpenClaw 失败：%v", err))
		}
	}

	return warnings, nil
}

func (w *Workflow) provisionPluginChannel(ctx context.Context, info system.Info, mode Mode, channel config.ChannelSelection, stdout, stderr io.Writer) error {
	pluginPackage := strings.TrimSpace(channel.PluginPackage)
	if pluginPackage == "" {
		return fmt.Errorf("%s 缺少插件包元数据", channel.Name)
	}
	w.progressDetailf("为 %s 安装插件 %s", channel.Name, pluginPackage)

	if err := w.runOpenClawCommand(ctx, info, mode, []string{"plugins", "install", pluginPackage}, stdout, stderr); err != nil {
		return fmt.Errorf("为 %s 安装插件失败: %w", channel.Name, err)
	}

	channelName := valueOrDefault(channel.OpenClawChannel, channel.Driver)
	args := []string{"channels", "add", "--channel", channelName}

	token := pluginChannelToken(channel)
	if token != "" {
		args = append(args, "--token", token)
	}
	w.progressDetailf("添加 OpenClaw 通道 %s", channelName)

	if err := w.runOpenClawCommand(ctx, info, mode, args, stdout, stderr); err != nil {
		return fmt.Errorf("配置通道 %s 失败: %w", channel.Name, err)
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
		openClawPath, err := w.resolveOpenClawExecutable(ctx, info, stderr)
		if err != nil {
			return err
		}
		return w.runCommand(ctx, openClawPath, args, nil, "", stdout, stderr)
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
		return w.runCommand(ctx, cmd, composeArgs, nil, info.RuntimeDir, stdout, stderr)
	default:
		return fmt.Errorf("不支持的安装模式 %s", mode)
	}
}

func (w *Workflow) ensureDockerGatewayRunning(ctx context.Context, info system.Info, stdout, stderr io.Writer) error {
	cmd, composeArgs, err := composeInvocation()
	if err != nil {
		return err
	}
	composeArgs = append(composeArgs, "-f", filepath.Join(info.RuntimeDir, "compose.yaml"), "up", "-d")
	return w.runCommand(ctx, cmd, composeArgs, nil, info.RuntimeDir, stdout, stderr)
}

func (w *Workflow) restartOpenClaw(ctx context.Context, info system.Info, mode Mode, stdout, stderr io.Writer) error {
	switch mode {
	case ModeNative:
		openClawPath, err := w.resolveOpenClawExecutable(ctx, info, io.Discard)
		if err != nil {
			return nil
		}
		return w.runCommand(ctx, openClawPath, []string{"gateway", "restart"}, nil, "", stdout, stderr)
	case ModeDocker:
		cmd, composeArgs, err := composeInvocation()
		if err != nil {
			return err
		}
		composeArgs = append(composeArgs, "-f", filepath.Join(info.RuntimeDir, "compose.yaml"), "restart", "openclaw")
		return w.runCommand(ctx, cmd, composeArgs, nil, info.RuntimeDir, stdout, stderr)
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

func (w *Workflow) shouldSkipPluginProvisioning(ctx context.Context, info system.Info, mode Mode) bool {
	switch mode {
	case ModeNative:
		_, err := w.resolveOpenClawExecutable(ctx, info, io.Discard)
		return err != nil
	case ModeDocker:
		_, err := os.Stat(filepath.Join(info.RuntimeDir, "compose.yaml"))
		return err != nil
	default:
		return true
	}
}
