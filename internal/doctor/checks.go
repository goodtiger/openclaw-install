package doctor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/goodtiger/openclaw-install/internal/shared"
	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/presets"
)

type CheckStatus string

const (
	StatusPass CheckStatus = "pass"
	StatusWarn CheckStatus = "warn"
	StatusFail CheckStatus = "fail"
)

type CheckResult struct {
	Status  CheckStatus
	Message string
}

type Check struct {
	Name        string
	Description string
	IsDefault   bool
	Run         func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult
	Fix         func(ctx context.Context, info system.Info) error
}

func AllChecks() []Check {
	return []Check{
		{
			Name:        "docker",
			Description: "检测 Docker 二进制是否存在",
			IsDefault:   true,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				if info.HasDocker {
					return CheckResult{Status: StatusPass, Message: ""}
				}
				return CheckResult{Status: StatusFail, Message: "未检测到 docker 可执行文件"}
			},
			Fix: nil,
		},
		{
			Name:        "docker-compose",
			Description: "检测 docker compose 是否可用",
			IsDefault:   true,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				if info.HasCompose {
					return CheckResult{Status: StatusPass, Message: ""}
				}
				return CheckResult{Status: StatusFail, Message: "未检测到 docker compose"}
			},
			Fix: nil,
		},
		{
			Name:        "docker-daemon",
			Description: "检测 Docker 守护进程是否健康",
			IsDefault:   true,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				if !info.HasDocker {
					return CheckResult{Status: StatusPass, Message: ""}
				}
				if info.DockerHealthy {
					return CheckResult{Status: StatusPass, Message: ""}
				}
				return CheckResult{Status: StatusFail, Message: "Docker 守护进程未响应"}
			},
			Fix: func(ctx context.Context, info system.Info) error {
				if info.OS == "linux" && system.HasCommand("systemctl") {
					cmd := exec.CommandContext(ctx, "sudo", "systemctl", "start", "docker")
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					return cmd.Run()
				}
				return fmt.Errorf("无法自动启动 Docker 守护进程，请手动启动")
			},
		},
		{
			Name:        "node",
			Description: "检测 Node.js 是否已安装",
			IsDefault:   true,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				if info.HasNode {
					return CheckResult{Status: StatusPass, Message: ""}
				}
				return CheckResult{Status: StatusFail, Message: "未检测到 node"}
			},
			Fix: nil,
		},
		{
			Name:        "npm",
			Description: "检测 npm 是否已安装",
			IsDefault:   true,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				if info.HasNPM {
					return CheckResult{Status: StatusPass, Message: ""}
				}
				return CheckResult{Status: StatusFail, Message: "未检测到 npm"}
			},
			Fix: nil,
		},
		{
			Name:        "openclaw",
			Description: "检测 openclaw 是否已安装",
			IsDefault:   true,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				if info.HasOpenClaw {
					return CheckResult{Status: StatusPass, Message: ""}
				}
				return CheckResult{Status: StatusWarn, Message: "未检测到 openclaw（安装后将可用）"}
			},
			Fix: nil,
		},
		{
			Name:        "git",
			Description: "检测 Git 是否已安装",
			IsDefault:   true,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				if info.HasGit {
					return CheckResult{Status: StatusPass, Message: ""}
				}
				return CheckResult{Status: StatusFail, Message: "未检测到 git"}
			},
			Fix: nil,
		},
		{
			Name:        "curl",
			Description: "检测 curl 是否已安装",
			IsDefault:   true,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				if info.HasCurl {
					return CheckResult{Status: StatusPass, Message: ""}
				}
				return CheckResult{Status: StatusWarn, Message: "未检测到 curl"}
			},
			Fix: nil,
		},
		{
			Name:        "package-manager",
			Description: "检测系统包管理器是否可用",
			IsDefault:   true,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				if info.PackageManager != "" {
					return CheckResult{Status: StatusPass, Message: ""}
				}
				if info.HasNode || info.HasDocker {
					return CheckResult{Status: StatusPass, Message: ""}
				}
				return CheckResult{Status: StatusWarn, Message: "未检测到受支持的包管理器，自动安装依赖可能失败"}
			},
			Fix: nil,
		},
		{
			Name:        "github-connectivity",
			Description: "检测 GitHub API 连通性（影响升级功能）",
			IsDefault:   true,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				urls := []string{
					"https://api.github.com",
					"https://ghproxy.com/https://api.github.com",
				}
				for _, u := range urls {
					if probeURL(ctx, u) == nil {
						return CheckResult{Status: StatusPass, Message: ""}
					}
				}
				return CheckResult{
					Status:  StatusWarn,
					Message: "GitHub API 不可达，升级命令可能失败；已配置 ghproxy 镜像回退，但当前网络仍无法访问",
				}
			},
			Fix: nil,
		},
		{
			Name:        "mirror-resolution",
			Description: "检测镜像源是否可达并完成选择",
			IsDefault:   true,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				if len(bundle.Mirrors.Categories) == 0 {
					return CheckResult{Status: StatusPass, Message: "未定义镜像分类，使用内置默认值"}
				}
				warnings := resolveMirrorWarnings(ctx, bundle)
				if len(warnings) == 0 {
					return CheckResult{Status: StatusPass, Message: ""}
				}
				return CheckResult{Status: StatusWarn, Message: strings.Join(warnings, "; ")}
			},
			Fix: nil,
		},
		{
			Name:        "windows-docker-recommendation",
			Description: "Windows 平台下提示 Docker 模式建议",
			IsDefault:   true,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				if info.OS == "windows" && !info.HasDocker {
					return CheckResult{
						Status:  StatusWarn,
						Message: "Windows 默认更推荐 Docker 模式；如果要使用 native，请先确保 Node.js/npm 可用",
					}
				}
				return CheckResult{Status: StatusPass, Message: ""}
			},
			Fix: nil,
		},
		{
			Name:        "go-version",
			Description: "检测 Go 版本是否满足要求",
			IsDefault:   false,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				version := strings.TrimPrefix(runtime.Version(), "go")
				parts := strings.SplitN(version, ".", 3)
				if len(parts) < 2 {
					return CheckResult{Status: StatusWarn, Message: fmt.Sprintf("无法解析 Go 版本: %s", version)}
				}
				major := parts[0]
				minor := parts[1]
				if major == "1" {
					minorInt := 0
					fmt.Sscanf(minor, "%d", &minorInt)
					if minorInt >= 22 {
						return CheckResult{Status: StatusPass, Message: ""}
					}
				}
				return CheckResult{Status: StatusWarn, Message: fmt.Sprintf("Go 版本 %s 可能过旧，建议 1.22+", version)}
			},
			Fix: nil,
		},
		{
			Name:        "proxy-env",
			Description: "检测代理环境变量配置",
			IsDefault:   false,
			Run: func(ctx context.Context, info system.Info, bundle presets.Bundle) CheckResult {
				httpProxy := os.Getenv("HTTP_PROXY")
				if httpProxy == "" {
					httpProxy = os.Getenv("http_proxy")
				}
				httpsProxy := os.Getenv("HTTPS_PROXY")
				if httpsProxy == "" {
					httpsProxy = os.Getenv("https_proxy")
				}
				if httpProxy == "" && httpsProxy == "" {
					return CheckResult{Status: StatusWarn, Message: "未设置 HTTP_PROXY/HTTPS_PROXY 环境变量"}
				}
				msg := []string{}
				if httpProxy != "" {
					msg = append(msg, fmt.Sprintf("HTTP_PROXY=%s", httpProxy))
				}
				if httpsProxy != "" {
					msg = append(msg, fmt.Sprintf("HTTPS_PROXY=%s", httpsProxy))
				}
				return CheckResult{Status: StatusPass, Message: strings.Join(msg, ", ")}
			},
			Fix: nil,
		},
	}
}

func DefaultChecks() []Check {
	all := AllChecks()
	out := make([]Check, 0, len(all))
	for _, c := range all {
		if c.IsDefault {
			out = append(out, c)
		}
	}
	return out
}

func CheckByName(name string) (Check, bool) {
	for _, c := range AllChecks() {
		if c.Name == name {
			return c, true
		}
	}
	return Check{}, false
}

func CheckNames() []string {
	all := AllChecks()
	names := make([]string, len(all))
	for i, c := range all {
		names[i] = c.Name
	}
	return names
}

func probeURL(ctx context.Context, rawURL string) error {
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}
	return fmt.Errorf("探测 %s 返回 HTTP %d", rawURL, resp.StatusCode)
}

func resolveMirrorWarnings(ctx context.Context, bundle presets.Bundle) []string {
	warnings := []string{}

	for _, key := range shared.SortedStringKeys(bundle.Mirrors.Categories) {
		candidates := bundle.Mirrors.Categories[key]
		_, err := chooseMirror(ctx, key, candidates)
		if err != nil {
			warnings = append(warnings, err.Error())
		}
	}

	return warnings
}

func chooseMirror(ctx context.Context, category string, candidates []presets.MirrorCandidate) (presets.MirrorCandidate, error) {
	if len(candidates) == 0 {
		return presets.MirrorCandidate{}, fmt.Errorf("镜像分类 %s 没有候选项", category)
	}

	for _, candidate := range candidates {
		if candidate.ProbeURL == "" {
			return candidate, nil
		}
		if err := probeURL(ctx, candidate.ProbeURL); err == nil {
			return candidate, nil
		}
	}

	return candidates[0], fmt.Errorf("镜像分类 %s 探测失败，已回退到 %s", category, candidates[0].Name)
}
