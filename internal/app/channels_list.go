package app

import (
	"fmt"
	"io"
	"os"

	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/presets"
)

func runChannelsList(args []string, out, errOut io.Writer) error {
	bundle, err := presets.Load()
	if err != nil {
		return fmt.Errorf("load presets: %w", err)
	}

	state, err := config.LoadInstallState(homeOpenclawPath("install-state.json"))
	if err != nil {
		return fmt.Errorf("load install state: %w", err)
	}

	stateChannelsSet := make(map[string]bool)
	for _, id := range state.ManagedChannels {
		stateChannelsSet[id] = true
	}

	fmt.Fprintln(out, "可用通道：")
	for _, preset := range bundle.Channels {
		enabled := stateChannelsSet[preset.ID]
		provisionerTag := "[bridge]"
		if preset.Provisioner == "openclaw-plugin" {
			provisionerTag = "[插件]"
		}
		enabledTag := ""
		if enabled {
			enabledTag = " [已启用]"
		}
		fmt.Fprintf(out, "  %-10s %-30s %s%s\n", preset.ID, preset.Name, provisionerTag, enabledTag)
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "注：[插件] = OpenClaw 插件，[bridge] = 独立 bridge 服务")

	return nil
}

func homeOpenclawPath(file string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.openclaw/" + file
	}
	return home + "/.openclaw/" + file
}
