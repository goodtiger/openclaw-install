package app

import (
	"fmt"
	"io"

	"github.com/goodtiger/openclaw-install/presets"
)

func runProvidersList(out io.Writer) error {
	bundle, err := presets.Load()
	if err != nil {
		return fmt.Errorf("load presets: %w", err)
	}

	fmt.Fprintln(out, "可用供应商：")
	for _, preset := range bundle.Providers {
		defaultModel := preset.DefaultModel
		if defaultModel == "" {
			if len(preset.Models) > 0 {
				defaultModel = preset.Models[0]
			} else {
				defaultModel = "-"
			}
		}
		isDefault := ""
		if preset.ID == "bailian" {
			isDefault = "[默认]"
		}
		fmt.Fprintf(out, "  %-15s %-25s %s  %s\n", preset.ID, preset.Name, isDefault, defaultModel)
	}

	return nil
}
