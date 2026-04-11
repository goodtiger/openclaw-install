package app

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/internal/output"
)

func runValidate(args []string, out, errOut io.Writer) error {
	fs := newFlagSet("validate", errOut, "验证配置文件格式与有效性。")
	openclawFlag := fs.Bool("openclaw", false, "仅验证 openclaw.json")
	bridgeFlag := fs.Bool("bridge", false, "仅验证 bridge.json")
	stateFlag := fs.Bool("state", false, "仅验证 install-state.json")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "~"
	}
	openclawPath := home + "/.openclaw/openclaw.json"
	bridgePath := home + "/.openclaw/bridge.json"
	statePath := home + "/.openclaw/install-state.json"

	hasOpenclawFlag := *openclawFlag
	hasBridgeFlag := *bridgeFlag
	hasStateFlag := *stateFlag

	if !hasOpenclawFlag && !hasBridgeFlag && !hasStateFlag {
		hasOpenclawFlag, hasBridgeFlag, hasStateFlag = true, true, true
	}

	fmt.Fprintln(out, "配置文件验证：")

	var problemCount int

	if hasOpenclawFlag {
		errorMsg := validateOpenclawFile(openclawPath)
		if errorMsg != "" {
			fmt.Fprintf(out, "  openclaw.json:  ✗ %s\n", errorMsg)
			problemCount++
		} else {
			fmt.Fprintln(out, "  openclaw.json:  ✓ 有效")
		}
	}

	if hasBridgeFlag {
		errorMsg := validateBridgeFile(bridgePath)
		if errorMsg != "" {
			fmt.Fprintf(out, "  bridge.json:    ✗ %s\n", errorMsg)
			problemCount++
		} else {
			fmt.Fprintln(out, "  bridge.json:    ✓ 有效")
		}
	}

	if hasStateFlag {
		errorMsg := validateStateFile(statePath)
		if errorMsg != "" {
			fmt.Fprintf(out, "  install-state.json:  ✗ %s\n", errorMsg)
			problemCount++
		} else {
			fmt.Fprintln(out, "  install-state.json:  ✓ 有效")
		}
	}

	fmt.Fprintln(out, "")
	if problemCount > 0 {
		fmt.Fprintf(out, "发现 %d 个问题\n", problemCount)
		return nil
	}

	fmt.Fprintln(out, "验证通过！")
	return nil
}

func validateOpenclawFile(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return output.NewFixable("文件不存在", "运行 openclaw-install install 以生成配置文件").Error()
		}
		return fmt.Sprintf("读取文件失败: %v", err)
	}

	if len(strings.TrimSpace(string(content))) == 0 {
		return output.NewFixable("文件为空", "检查文件是否被意外清空，可运行 openclaw-install reconfigure 重新生成").Error()
	}

	var data map[string]any
	if err := json.Unmarshal(content, &data); err != nil {
		return output.NewFixablef("JSON 解析失败: %v", "检查 JSON 语法是否正确，可使用 jsonlint.com 在线校验", err).Error()
	}

	models, hasModels := data["models"].(map[string]any)
	if !hasModels {
		return output.NewFixable("缺少 models 配置", "在 openclaw.json 中添加 models 配置，或运行 openclaw-install reconfigure 重新生成").Error()
	}
	if _, hasProviders := models["providers"]; !hasProviders {
		return output.NewFixable("缺少 models.providers 配置", "在 openclaw.json 的 models 下添加 providers 配置，或运行 openclaw-install reconfigure").Error()
	}

	return ""
}

func validateBridgeFile(path string) string {
	_, err := config.LoadBridgeConfig(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return output.NewFixable("文件不存在", "运行 openclaw-install install 以生成 bridge 配置").Error()
		}
		errMsg := err.Error()
		if idx := strings.Index(errMsg, ": "); idx > 0 {
			return output.NewFixablef("%s", "检查 bridge.json 格式是否正确，或运行 openclaw-install reconfigure 重新生成", errMsg[idx+2:]).Error()
		}
		return output.NewFixablef("%s", "检查 bridge.json 格式是否正确，或运行 openclaw-install reconfigure 重新生成", errMsg).Error()
	}

	return ""
}

func validateStateFile(path string) string {
	_, err := config.LoadInstallState(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return output.NewFixable("文件不存在", "首次安装前无安装状态文件，属于正常现象").Error()
		}
		errMsg := err.Error()
		if idx := strings.Index(errMsg, ": "); idx > 0 {
			return output.NewFixablef("%s", "检查 install-state.json 格式是否正确，或删除后重新安装", errMsg[idx+2:]).Error()
		}
		return output.NewFixablef("%s", "检查 install-state.json 格式是否正确，或删除后重新安装", errMsg).Error()
	}

	return ""
}
