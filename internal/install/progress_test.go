package install

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/presets"
)

func TestInstallReportsProgressSteps(t *testing.T) {
	workflow := NewWorkflow(presets.Bundle{}, &recordingExecutor{})

	homeDir := t.TempDir()
	info := system.Info{
		OS:               "linux",
		Arch:             "amd64",
		HomeDir:          homeDir,
		OpenClawHome:     filepath.Join(homeDir, ".openclaw"),
		ConfigPath:       filepath.Join(homeDir, ".openclaw", "openclaw.json"),
		BridgeConfigPath: filepath.Join(homeDir, ".openclaw", "bridge.json"),
		StatePath:        filepath.Join(homeDir, ".openclaw", "install-state.json"),
		RuntimeDir:       filepath.Join(homeDir, ".openclaw", "runtime"),
	}

	req := Request{
		Mode: ModeNative,
		Provider: config.ProviderConfig{
			ID:           "bailian",
			Name:         "Alibaba Bailian Coding Plan",
			BaseURL:      "https://coding.dashscope.aliyuncs.com/v1",
			PrimaryModel: "qwen3.5-plus",
		},
		AppVersion:  "0.1.1",
		SkipInstall: true,
		SkipVerify:  true,
	}

	var out bytes.Buffer
	if _, err := workflow.Install(context.Background(), info, req, &out, io.Discard); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	for _, want := range []string{
		"[1/6] 准备工作目录",
		"[2/6] 解析镜像源",
		"[3/6] 写入配置文件",
		"[4/6] 生成运行时文件",
		"[5/6] 保存安装状态",
		"[6/6] 配置通道",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected progress output to contain %q, got:\n%s", want, out.String())
		}
	}
}

func TestRunCommandReportsCurrentCommand(t *testing.T) {
	executor := &recordingExecutor{}
	workflow := NewWorkflow(presets.Bundle{}, executor)

	var out bytes.Buffer
	workflow.progress = newProgressTracker(&out, 1)
	workflow.progress.Step("安装依赖")

	if err := workflow.runCommand(context.Background(), "npm", []string{"install", "-g", "openclaw"}, nil, "", &out, io.Discard); err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}

	if !strings.Contains(out.String(), "  -> npm install -g openclaw") {
		t.Fatalf("expected command progress in output, got:\n%s", out.String())
	}

	if len(executor.commands) != 1 || executor.commands[0] != "npm install -g openclaw" {
		t.Fatalf("unexpected recorded commands: %#v", executor.commands)
	}
}
