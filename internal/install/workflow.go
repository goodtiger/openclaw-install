package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/internal/output"
	"github.com/goodtiger/openclaw-install/internal/shared"
	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/presets"
)

type Mode string

const (
	ModeDocker Mode = "docker"
	ModeNative Mode = "native"
)

type Request struct {
	Mode        Mode
	Provider    config.ProviderConfig
	Channels    []config.ChannelSelection
	AppVersion  string
	SkipInstall bool
	SkipVerify  bool
	ResumeFrom  string
}

type Result struct {
	BackupFile       string
	ConfigPath       string
	BridgeConfigPath string
	StatePath        string
	RuntimeDir       string
	MirrorNames      map[string]string
	Warnings         []string
}

type DoctorReport struct {
	Info            system.Info
	RecommendedMode Mode
	MirrorNames     map[string]string
	Warnings        []string
}

type Executor interface {
	Run(ctx context.Context, cmd string, args []string, env map[string]string, dir string, stdout, stderr io.Writer) error
}

type RealExecutor struct{}

type Workflow struct {
	Presets    presets.Bundle
	Executor   Executor
	HTTPClient *http.Client
	Now        func() time.Time
	progressMu sync.RWMutex
	progress   *progressTracker
}

func NewWorkflow(bundle presets.Bundle, executor Executor) *Workflow {
	if executor == nil {
		executor = RealExecutor{}
	}
	return &Workflow{
		Presets:  bundle,
		Executor: executor,
		HTTPClient: &http.Client{
			Timeout: 3 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		},
		Now: time.Now,
	}
}

func (w *Workflow) LoadAndCheckInstallState(info system.Info) (config.InstallState, bool, error) {
	state, err := config.LoadInstallState(info.StatePath)
	if err != nil {
		return config.InstallState{}, false, fmt.Errorf("load install state: %w", err)
	}
	if state.Version == "" {
		return config.InstallState{}, false, nil
	}
	return state, !state.InstallComplete, nil
}

func (w *Workflow) shouldSkipStep(resumeFrom, step string) bool {
	if resumeFrom == "" {
		return false
	}
	stepOrder := map[string]int{
		progressStepPrepareWorkspace: 0,
		progressStepResolveMirrors:   1,
		progressStepWriteConfig:      2,
		progressStepGenerateAssets:   3,
		progressStepSaveState:        4,
		progressStepInstallDeps:      5,
		progressStepInstallRuntime:   6,
		progressStepConfigureChannel: 7,
		progressStepVerify:           8,
	}
	currentIdx, hasCurrent := stepOrder[step]
	resumeIdx, hasResume := stepOrder[resumeFrom]
	if !hasCurrent || !hasResume {
		return false
	}
	return currentIdx <= resumeIdx
}

func (w *Workflow) saveInstallStateWithStep(info system.Info, req Request, previousState config.InstallState, step string) error {
	mirrors, _ := w.ResolveMirrors(context.Background())
	state := config.InstallState{
		Version:           req.AppVersion,
		InstalledAt:       w.Now().UTC(),
		Mode:              req.Mode.String(),
		Platform:          info.OS + "/" + info.Arch,
		ManagedProviderID: req.Provider.ID,
		ManagedChannels:   channelIDs(req.Channels),
		MirrorNames:       mirrorNames(mirrors),
		RuntimeDir:        info.RuntimeDir,
		ConfigPath:        info.ConfigPath,
		BridgeConfigPath:  info.BridgeConfigPath,
		LastCompletedStep: step,
		InstallComplete:   false,
	}
	return config.SaveInstallState(info.StatePath, state)
}

func (w *Workflow) saveInstallStateComplete(info system.Info, req Request, previousState config.InstallState) error {
	mirrors, _ := w.ResolveMirrors(context.Background())
	state := config.InstallState{
		Version:           req.AppVersion,
		InstalledAt:       w.Now().UTC(),
		Mode:              req.Mode.String(),
		Platform:          info.OS + "/" + info.Arch,
		ManagedProviderID: req.Provider.ID,
		ManagedChannels:   channelIDs(req.Channels),
		MirrorNames:       mirrorNames(mirrors),
		RuntimeDir:        info.RuntimeDir,
		ConfigPath:        info.ConfigPath,
		BridgeConfigPath:  info.BridgeConfigPath,
		LastCompletedStep: "verify",
		InstallComplete:   true,
	}
	return config.SaveInstallState(info.StatePath, state)
}

func (RealExecutor) Run(ctx context.Context, cmd string, args []string, env map[string]string, dir string, stdout, stderr io.Writer) error {
	command := exec.CommandContext(ctx, cmd, args...)
	command.Dir = dir
	command.Stdout = stdout
	command.Stderr = stderr
	command.Env = append(os.Environ(), flattenEnv(env)...)
	return command.Run()
}

func (w *Workflow) beginProgress(out io.Writer, req Request) func() {
	w.progressMu.Lock()
	w.progress = newProgressTracker(out, installStepCount(req))
	w.progressMu.Unlock()
	return func() {
		w.progressMu.Lock()
		w.progress = nil
		w.progressMu.Unlock()
	}
}

func (w *Workflow) progressStep(title string) {
	if progress := w.currentProgress(); progress != nil {
		progress.Step(title)
	}
}

func (w *Workflow) progressDetailf(format string, args ...any) {
	if progress := w.currentProgress(); progress != nil {
		progress.Detailf(format, args...)
	}
}

func (w *Workflow) runCommand(ctx context.Context, cmd string, args []string, env map[string]string, dir string, stdout, stderr io.Writer) error {
	if progress := w.currentProgress(); progress != nil {
		progress.Command(cmd, args)
	}
	return w.Executor.Run(ctx, cmd, args, env, dir, stdout, stderr)
}

func (w *Workflow) currentProgress() *progressTracker {
	w.progressMu.RLock()
	defer w.progressMu.RUnlock()
	return w.progress
}

func (w *Workflow) Doctor(ctx context.Context, info system.Info) (DoctorReport, error) {
	mirrors, mirrorWarnings := w.ResolveMirrors(ctx)

	warnings := append([]string{}, mirrorWarnings...)
	if info.PackageManager == "" && !info.HasNode && !info.HasDocker {
		warnings = append(warnings, "未检测到受支持的包管理器，自动安装依赖可能失败")
	}
	if info.OS == "windows" && !info.HasDocker {
		warnings = append(warnings, "Windows 默认更推荐 Docker 模式；如果要使用 native，请先确保 Node.js/npm 可用")
	}

	if ghReachable, ghMsg := w.checkGitHubConnectivity(ctx); !ghReachable {
		warnings = append(warnings, ghMsg)
	}

	return DoctorReport{
		Info:            info,
		RecommendedMode: recommendedMode(info),
		MirrorNames:     mirrorNames(mirrors),
		Warnings:        warnings,
	}, nil
}

func (w *Workflow) checkGitHubConnectivity(ctx context.Context) (bool, string) {
	urls := []string{
		"https://api.github.com",
		"https://ghproxy.com/https://api.github.com",
	}
	for _, url := range urls {
		if err := w.probeURL(ctx, url); err == nil {
			return true, ""
		}
	}
	return false, "GitHub API 不可达，升级命令可能失败；已配置 ghproxy 镜像回退，但当前网络仍无法访问\n💡 设置代理: export HTTPS_PROXY=http://127.0.0.1:7890"
}

func (w *Workflow) Install(ctx context.Context, info system.Info, req Request, stdout, stderr io.Writer) (Result, error) {
	normalizedReq, err := req.Normalize(info)
	if err != nil {
		return Result{}, fmt.Errorf("normalize request: %w", err)
	}
	req = normalizedReq
	resetProgress := w.beginProgress(stdout, req)
	defer resetProgress()

	w.progressStep(progressStepPrepareWorkspace)
	if err := config.EnsureDir(info.OpenClawHome); err != nil {
		return Result{}, fmt.Errorf("create workspace dir %s: %w", info.OpenClawHome, err)
	}

	result := Result{
		ConfigPath:       info.ConfigPath,
		BridgeConfigPath: info.BridgeConfigPath,
		StatePath:        info.StatePath,
		RuntimeDir:       info.RuntimeDir,
	}

	backupFile, err := config.BackupIfExists(info.ConfigPath, filepath.Join(info.OpenClawHome, ".backups"), w.Now())
	if err != nil {
		return Result{}, fmt.Errorf("backup config: %w", err)
	}
	result.BackupFile = backupFile
	if backupFile != "" {
		w.progressDetailf("已备份现有配置到 %s", backupFile)
	}

	w.progressStep(progressStepResolveMirrors)
	mirrors, mirrorWarnings := w.ResolveMirrors(ctx)
	result.MirrorNames = mirrorNames(mirrors)
	result.Warnings = append(result.Warnings, mirrorWarnings...)
	if len(result.MirrorNames) == 0 {
		w.progressDetailf("未定义镜像分类，使用内置默认值")
	} else {
		for _, key := range shared.SortedStringKeys(result.MirrorNames) {
			w.progressDetailf("%s：%s", key, result.MirrorNames[key])
		}
	}

	previousState, err := config.LoadInstallState(info.StatePath)
	if err != nil {
		return Result{}, fmt.Errorf("load install state: %w", err)
	}

	if w.shouldSkipStep(req.ResumeFrom, progressStepSaveState) {
		w.progressStep(progressStepWriteConfig)
		w.progressStep(progressStepGenerateAssets)
		w.progressStep(progressStepSaveState)
		w.progressDetailf("跳过写入配置和生成运行时文件（已完成）")
	} else {
		assetWarnings, err := w.applyConfigAndAssets(ctx, info, req, previousState, mirrors, result.MirrorNames, stdout, stderr)
		if err != nil {
			return Result{}, fmt.Errorf("apply config and assets: %w", err)
		}
		result.Warnings = append(result.Warnings, assetWarnings...)
	}

	if !req.SkipInstall {
		w.progressStep(progressStepInstallDeps)
		if w.shouldSkipStep(req.ResumeFrom, progressStepInstallDeps) {
			w.progressDetailf("跳过 %s（已完成）", progressStepInstallDeps)
		} else {
			if err := w.installDependencies(ctx, info, req.Mode, stdout, stderr); err != nil {
				return result, fmt.Errorf("install dependencies: %w", err)
			}
		}

		w.progressStep(progressStepInstallRuntime)
		if w.shouldSkipStep(req.ResumeFrom, progressStepInstallRuntime) {
			w.progressDetailf("跳过 %s（已完成）", progressStepInstallRuntime)
		} else {
			if err := w.installOpenClaw(ctx, info, req.Mode, mirrors, stdout, stderr); err != nil {
				return result, fmt.Errorf("install OpenClaw runtime: %w", err)
			}
		}

		if err := w.saveInstallStateWithStep(info, req, previousState, "installRuntime"); err != nil {
			return result, fmt.Errorf("save install state: %w", err)
		}
	}

	w.progressStep(progressStepConfigureChannel)
	if w.shouldSkipStep(req.ResumeFrom, progressStepConfigureChannel) {
		w.progressDetailf("跳过 %s（已完成）", progressStepConfigureChannel)
	} else {
		if len(req.Channels) == 0 {
			w.progressDetailf("未启用任何通道")
		}
		channelWarnings, err := w.syncChannels(ctx, info, req, previousState, stdout, stderr)
		result.Warnings = append(result.Warnings, channelWarnings...)
		if err != nil {
			return result, fmt.Errorf("configure channels: %w", err)
		}
	}

	if err := w.saveInstallStateWithStep(info, req, previousState, "configureChannels"); err != nil {
		return result, fmt.Errorf("save install state: %w", err)
	}

	if !req.SkipVerify {
		w.progressStep(progressStepVerify)
		if w.shouldSkipStep(req.ResumeFrom, progressStepVerify) {
			w.progressDetailf("跳过 %s（已完成）", progressStepVerify)
		} else {
			verifyWarnings, err := w.verify(ctx, info, req, stdout, stderr)
			result.Warnings = append(result.Warnings, verifyWarnings...)
			if err != nil {
				return result, fmt.Errorf("verify installation: %w", err)
			}
		}
	}

	if err := w.saveInstallStateComplete(info, req, previousState); err != nil {
		return result, fmt.Errorf("save install state: %w", err)
	}

	return result, nil
}

// applyConfigAndAssets 构建并写入配置文件、生成运行时文件、保存安装状态。
func (w *Workflow) applyConfigAndAssets(ctx context.Context, info system.Info, req Request, previousState config.InstallState, mirrors MirrorSelection, mirrorNames map[string]string, stdout, stderr io.Writer) (warnings []string, err error) {
	input := config.ManagedConfigInput{
		InstallerVersion: req.AppVersion,
		Mode:             req.Mode.String(),
		GatewayBind:      gatewayBindForMode(req.Mode),
		BridgeHost:       bridgeHostForMode(req.Mode),
		Provider:         req.Provider,
		Channels:         req.Channels,
		ManagedAt:        w.Now(),
		MirrorNames:      mirrorNames,
	}

	existingConfig, err := config.LoadMap(info.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("load existing config %s: %w", info.ConfigPath, err)
	}
	managedConfig := config.BuildManagedConfig(input)
	finalConfig := config.ApplyManagedConfig(existingConfig, managedConfig, previousState)

	w.progressStep(progressStepWriteConfig)
	if err := config.SaveJSONAtomic(info.ConfigPath, finalConfig); err != nil {
		return nil, fmt.Errorf("save OpenClaw config %s: %w", info.ConfigPath, err)
	}
	w.progressDetailf("OpenClaw 配置 -> %s", info.ConfigPath)

	if err := config.SaveJSONAtomic(info.BridgeConfigPath, config.BuildBridgeConfig(input)); err != nil {
		return nil, fmt.Errorf("save Bridge config %s: %w", info.BridgeConfigPath, err)
	}
	w.progressDetailf("Bridge 配置 -> %s", info.BridgeConfigPath)

	w.progressStep(progressStepGenerateAssets)
	assetWarnings, err := w.writeAssets(ctx, info, req, previousState, mirrors, stdout, stderr)
	if err != nil {
		return nil, fmt.Errorf("write runtime assets: %w", err)
	}
	warnings = append(warnings, assetWarnings...)
	w.progressDetailf("运行时文件 -> %s", info.RuntimeDir)

	state := config.InstallState{
		Version:           req.AppVersion,
		InstalledAt:       w.Now().UTC(),
		Mode:              req.Mode.String(),
		Platform:          info.OS + "/" + info.Arch,
		ManagedProviderID: req.Provider.ID,
		ManagedChannels:   channelIDs(req.Channels),
		MirrorNames:       mirrorNames,
		RuntimeDir:        info.RuntimeDir,
		ConfigPath:        info.ConfigPath,
		BridgeConfigPath:  info.BridgeConfigPath,
	}

	w.progressStep(progressStepSaveState)
	if err := config.SaveInstallState(info.StatePath, state); err != nil {
		return nil, fmt.Errorf("save install state %s: %w", info.StatePath, err)
	}
	w.progressDetailf("安装状态 -> %s", info.StatePath)

	return warnings, nil
}

func (w *Workflow) Reconfigure(ctx context.Context, info system.Info, req Request, stdout, stderr io.Writer) (Result, error) {
	reconfigReq := req
	reconfigReq.SkipInstall = true
	return w.Install(ctx, info, reconfigReq, stdout, stderr)
}

// Normalize validates and normalizes the request, returning a copy with default
// values filled in. It may modify Provider.Type and Mode if they are empty.
func (r Request) Normalize(info system.Info) (Request, error) {
	normalized := r
	if normalized.Mode == "" {
		normalized.Mode = recommendedMode(info)
	}
	if normalized.AppVersion == "" {
		return Request{}, errors.New("缺少安装器版本号")
	}
	if strings.TrimSpace(normalized.Provider.ID) == "" {
		return Request{}, errors.New("缺少供应商 ID")
	}
	if strings.TrimSpace(normalized.Provider.Name) == "" {
		return Request{}, errors.New("缺少供应商名称")
	}
	if strings.TrimSpace(normalized.Provider.Type) == "" {
		normalized.Provider.Type = "openai-compatible"
	}
	if strings.TrimSpace(normalized.Provider.BaseURL) == "" {
		return Request{}, errors.New("缺少供应商 Base URL")
	}
	if strings.TrimSpace(normalized.Provider.PrimaryModel) == "" {
		return Request{}, errors.New("缺少主模型")
	}
	return normalized, nil
}

// Validate checks that all required fields are populated without modifying
// the receiver. Use this when you need to validate a request without applying
// default values or making any changes.
func (r Request) Validate(info system.Info) error {
	if r.AppVersion == "" {
		return errors.New("缺少安装器版本号")
	}
	if strings.TrimSpace(r.Provider.ID) == "" {
		return errors.New("缺少供应商 ID")
	}
	if strings.TrimSpace(r.Provider.Name) == "" {
		return errors.New("缺少供应商名称")
	}
	if strings.TrimSpace(r.Provider.BaseURL) == "" {
		return errors.New("缺少供应商 Base URL")
	}
	if strings.TrimSpace(r.Provider.PrimaryModel) == "" {
		return errors.New("缺少主模型")
	}
	return nil
}

func (w *Workflow) installDependencies(ctx context.Context, info system.Info, mode Mode, stdout, stderr io.Writer) error {
	switch mode {
	case ModeDocker:
		return w.ensureDocker(ctx, info, stdout, stderr)
	case ModeNative:
		return w.ensureNode(ctx, info, stdout, stderr)
	default:
		return output.NewFixablef("不支持的安装模式 %s", "请使用 docker 或 native 模式", mode)
	}
}

func (w *Workflow) installOpenClaw(ctx context.Context, info system.Info, mode Mode, mirrors MirrorSelection, stdout, stderr io.Writer) error {
	switch mode {
	case ModeDocker:
		return w.installDockerMode(ctx, info, stdout, stderr)
	case ModeNative:
		return w.installNativeMode(ctx, info, mirrors, stdout, stderr)
	default:
		return output.NewFixablef("不支持的安装模式 %s", "请使用 docker 或 native 模式", mode)
	}
}

func (w *Workflow) verify(ctx context.Context, info system.Info, req Request, stdout, stderr io.Writer) ([]string, error) {
	warnings := []string{}
	if _, err := config.LoadMap(info.ConfigPath); err != nil {
		return warnings, fmt.Errorf("校验配置文件 %s 失败: %w", info.ConfigPath, err)
	}
	if _, err := config.LoadBridgeConfig(info.BridgeConfigPath); err != nil {
		return warnings, fmt.Errorf("校验桥接配置 %s 失败: %w", info.BridgeConfigPath, err)
	}

	switch req.Mode {
	case ModeDocker:
		cmd, args, err := composeInvocation()
		if err != nil {
			return warnings, fmt.Errorf("invoke compose command: %w", err)
		}
		args = append(args, "-f", filepath.Join(info.RuntimeDir, "compose.yaml"), "config")
		if err := w.runCommand(ctx, cmd, args, nil, info.RuntimeDir, stdout, stderr); err != nil {
			return warnings, fmt.Errorf("docker compose 校验失败: %w", err)
		}
	}

	switch req.Mode {
	case ModeNative:
		openClawPath, err := w.resolveOpenClawExecutable(ctx, info, io.Discard)
		if err != nil {
			warnings = append(warnings, "当前环境还找不到 openclaw，可重开终端后再试")
		} else if err := w.runCommand(ctx, openClawPath, []string{"config", "validate"}, nil, "", stdout, stderr); err != nil {
			return warnings, fmt.Errorf("openclaw 配置校验失败: %w", err)
		}
	case ModeDocker:
		if err := w.runOpenClawCommand(ctx, info, req.Mode, []string{"config", "validate"}, stdout, stderr); err != nil {
			return warnings, fmt.Errorf("openclaw 配置校验失败: %w", err)
		}
	}

	if hasBridgeChannels(req.Channels) {
		warnings = append(warnings, "已在宿主机侧配置 bridge 服务；可用 `openclaw-install bridge serve --channel <name>` 或系统服务管理器检查状态")
	}
	if hasPluginChannels(req.Channels) {
		warnings = append(warnings, "插件型通道已通过 OpenClaw CLI 配置，可用 `openclaw channels list` 检查")
	}

	return warnings, nil
}

func (w *Workflow) ensureDocker(ctx context.Context, info system.Info, stdout, stderr io.Writer) error {
	if info.HasDocker && info.HasCompose {
		w.progressDetailf("Docker 和 docker compose 已可用")
		return nil
	}

	var dockerPackages []string
	switch info.PackageManager {
	case "apt-get":
		dockerPackages = []string{"docker.io", "docker-compose-plugin"}
	case "dnf":
		dockerPackages = []string{"docker", "docker-compose"}
	case "yum":
		dockerPackages = []string{"docker"}
	case "brew":
		if err := w.runCommand(ctx, "brew", []string{"install", "--cask", "docker"}, nil, "", stdout, stderr); err != nil {
			return fmt.Errorf("install docker via brew: %w", err)
		}
	case "winget":
		if err := w.runCommand(ctx, "winget", []string{"install", "-e", "--id", "Docker.DockerDesktop"}, nil, "", stdout, stderr); err != nil {
			return fmt.Errorf("install docker via winget: %w", err)
		}
	default:
		return output.NewFixable("未安装 Docker，且没有可用的包管理器用于自动安装",
			"手动安装 Docker Desktop: https://www.docker.com/products/docker-desktop/")
	}

	if info.PackageManager == "apt-get" || info.PackageManager == "dnf" || info.PackageManager == "yum" {
		if err := w.installPackages(ctx, info, dockerPackages, stdout, stderr); err != nil {
			return err
		}
	}

	if info.OS == "linux" && system.HasCommand("systemctl") {
		_ = w.runPrivileged(ctx, info, "systemctl", []string{"enable", "--now", "docker"}, nil, "", stdout, stderr)
	}

	return nil
}

// installPackages installs the provided packages using the appropriate package manager available in the system
func (w *Workflow) installPackages(ctx context.Context, info system.Info, packages []string, stdout, stderr io.Writer) error {
	switch info.PackageManager {
	case "apt-get":
		if err := w.runPrivileged(ctx, info, "apt-get", []string{"update"}, nil, "", stdout, stderr); err != nil {
			return fmt.Errorf("apt-get update: %w", err)
		}
		installArgs := append([]string{"install", "-y"}, packages...)
		if err := w.runPrivileged(ctx, info, "apt-get", installArgs, nil, "", stdout, stderr); err != nil {
			return fmt.Errorf("install packages %v via apt-get: %w", packages, err)
		}
	case "dnf":
		installArgs := append([]string{"install", "-y"}, packages...)
		if err := w.runPrivileged(ctx, info, "dnf", installArgs, nil, "", stdout, stderr); err != nil {
			return fmt.Errorf("install packages %v via dnf: %w", packages, err)
		}
	case "yum":
		installArgs := append([]string{"install", "-y"}, packages...)
		if err := w.runPrivileged(ctx, info, "yum", installArgs, nil, "", stdout, stderr); err != nil {
			return fmt.Errorf("install packages %v via yum: %w", packages, err)
		}
	case "brew":
		installArgs := append([]string{"install", "--cask"}, packages...)
		if err := w.runCommand(ctx, "brew", installArgs, nil, "", stdout, stderr); err != nil {
			return fmt.Errorf("install packages %v via brew: %w", packages, err)
		}
	case "winget":
		args := []string{"install", "-e", "--id"}
		args = append(args, packages...)
		if err := w.runCommand(ctx, "winget", args, nil, "", stdout, stderr); err != nil {
			return fmt.Errorf("install packages %v via winget: %w", packages, err)
		}
	default:
		return fmt.Errorf("unsupported package manager %s, cannot install packages %v", info.PackageManager, packages)
	}
	return nil
}

func (w *Workflow) ensureNode(ctx context.Context, info system.Info, stdout, stderr io.Writer) error {
	if info.HasNode && info.HasNPM {
		w.progressDetailf("Node.js 和 npm 已可用")
		return nil
	}

	var nodePackages []string
	switch info.PackageManager {
	case "apt-get":
		nodePackages = []string{"nodejs", "npm"}
	case "dnf":
		nodePackages = []string{"nodejs", "npm"}
	case "yum":
		nodePackages = []string{"nodejs", "npm"}
	case "brew":
		if err := w.runCommand(ctx, "brew", []string{"install", "node"}, nil, "", stdout, stderr); err != nil {
			return fmt.Errorf("install node via brew: %w", err)
		}
	case "winget":
		if err := w.runCommand(ctx, "winget", []string{"install", "-e", "--id", "OpenJS.NodeJS.LTS"}, nil, "", stdout, stderr); err != nil {
			return fmt.Errorf("install node via winget: %w", err)
		}
	default:
		return output.NewFixable("未安装 Node.js/npm，且没有可用的包管理器用于自动安装",
			"1) 使用 nvm 安装: curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash\n2) 或手动下载: https://nodejs.org")
	}

	if info.PackageManager == "apt-get" || info.PackageManager == "dnf" || info.PackageManager == "yum" {
		return w.installPackages(ctx, info, nodePackages, stdout, stderr)
	}

	return nil
}

func (w *Workflow) installDockerMode(ctx context.Context, info system.Info, stdout, stderr io.Writer) error {
	cmd, args, err := composeInvocation()
	if err != nil {
		return fmt.Errorf("get docker compose invocation details: %w", err)
	}
	args = append(args, "-f", filepath.Join(info.RuntimeDir, "compose.yaml"), "up", "-d", "--build")
	return w.runCommand(ctx, cmd, args, nil, info.RuntimeDir, stdout, stderr)
}

func (w *Workflow) installNativeMode(ctx context.Context, info system.Info, mirrors MirrorSelection, stdout, stderr io.Writer) error {
	openClawPath, inPath, err := w.resolveOpenClawWithPath(ctx, info, io.Discard)
	if err != nil {
		npmPath, npmErr := w.resolveNPMExecutable(info)
		if npmErr != nil {
			return npmErr
		}

		candidates := w.orderedMirrorCandidates("npm_registry", mirrors)
		if len(candidates) == 0 {
			candidates = []presets.MirrorCandidate{
				{
					Name:    "official",
					BaseURL: "https://registry.npmjs.org",
				},
			}
		}

		var lastInstallErr error
		for idx, candidate := range candidates {
			registryURL := strings.TrimSpace(candidate.BaseURL)
			if registryURL == "" {
				continue
			}
			if err := validateHTTPSURL(registryURL); err != nil {
				lastInstallErr = fmt.Errorf("npm 源 %s 无效: %w", mirrorCandidateLabel(candidate), err)
				if idx < len(candidates)-1 {
					w.progressDetailf("跳过无效 npm 源 %s", mirrorCandidateLabel(candidate))
				}
				continue
			}

			w.progressDetailf("尝试 npm 源 %s (%s)", mirrorCandidateLabel(candidate), registryURL)
			env := map[string]string{
				"NPM_CONFIG_REGISTRY": registryURL,
				"npm_config_registry": registryURL,
			}
			if goEnv := w.GoProxyEnv(mirrors); goEnv != nil {
				for k, v := range goEnv {
					env[k] = v
				}
			}
			instErr := w.runCommand(ctx, npmPath, []string{"install", "-g", "openclaw"}, env, "", stdout, stderr)
			if instErr == nil {
				if idx > 0 {
					w.progressDetailf("切换到 %s 后 npm 安装成功", mirrorCandidateLabel(candidate))
				}
				lastInstallErr = nil
				break
			}

			lastInstallErr = fmt.Errorf("install openclaw via npm using mirror %s (%s): %w", mirrorCandidateLabel(candidate), candidate.BaseURL, instErr)
			if idx < len(candidates)-1 {
				w.progressDetailf("使用 %s 安装失败，继续尝试下一个源", mirrorCandidateLabel(candidate))
			}
		}
		if lastInstallErr != nil {
			return lastInstallErr
		}

		openClawPath, inPath, err = w.resolveOpenClawWithPath(ctx, info, stderr)
		if err != nil {
			w.progressDetailf("安装完成，但当前仍找不到 openclaw，已跳过 gateway start")
			return nil
		}

		if !inPath {
			npmBinDir, binDirErr := w.npmGlobalBinDir(ctx, info, stderr)
			if binDirErr == nil && npmBinDir != "" {
				w.printPathWarning(npmBinDir, info.OS, stdout)
			}
		}
	} else {
		w.progressDetailf("OpenClaw 已安装，跳过 npm 全局安装")
	}

	return w.runCommand(ctx, openClawPath, []string{"gateway", "start"}, nil, "", stdout, stderr)
}

func (w *Workflow) printPathWarning(npmBinDir, osName string, stdout io.Writer) {
	fmt.Fprintln(stdout, "")
	fmt.Fprintf(stdout, "注意：openclaw 已安装到 %s，但该目录不在系统 PATH 中。\n", npmBinDir)
	fmt.Fprintln(stdout, "请将 "+npmBinDir+" 加入 PATH 以使用 openclaw 命令。")

	if osName == "windows" {
		fmt.Fprintln(stdout, "")
		fmt.Fprintf(stdout, "请将 %s 加入系统 PATH：\n", npmBinDir)
		fmt.Fprintf(stdout, "  setx PATH \"%%PATH%%;%s\"\n", npmBinDir)
	} else {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "临时使用（当前会话）:")
		fmt.Fprintf(stdout, "  export PATH=\"%s:$PATH\"\n", npmBinDir)
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "永久添加（添加到 shell 配置文件）:")
		shell := os.Getenv("SHELL")
		if shell != "" && strings.HasSuffix(shell, "bash") {
			fmt.Fprintf(stdout, "  echo 'export PATH=\"%s:$PATH\"' >> ~/.bashrc\n", npmBinDir)
		} else {
			fmt.Fprintf(stdout, "  echo 'export PATH=\"%s:$PATH\"' >> ~/.zshrc\n", npmBinDir)
		}
	}
}

func (w *Workflow) orderedMirrorCandidates(category string, selected MirrorSelection) []presets.MirrorCandidate {
	seen := map[string]struct{}{}
	ordered := []presets.MirrorCandidate{}

	if candidate, ok := selected[category]; ok {
		ordered = appendUniqueMirrorCandidate(ordered, seen, candidate)
	}

	for _, candidate := range w.Presets.Mirrors.Categories[category] {
		ordered = appendUniqueMirrorCandidate(ordered, seen, candidate)
	}

	return ordered
}

// GoProxyEnv returns GOPROXY and GOSUMDB environment variables based on the
// selected go_proxy mirror. Returns nil if no go_proxy mirror is available.
func (w *Workflow) GoProxyEnv(mirrors MirrorSelection) map[string]string {
	candidates := w.orderedMirrorCandidates("go_proxy", mirrors)
	if len(candidates) == 0 {
		return nil
	}
	// Use the first (best) candidate
	candidate := candidates[0]
	baseURL := strings.TrimSpace(candidate.BaseURL)
	if baseURL == "" {
		return nil
	}
	return map[string]string{
		"GOPROXY": baseURL + ",direct",
		"GOSUMDB": "sum.golang.google.cn",
	}
}

func appendUniqueMirrorCandidate(dst []presets.MirrorCandidate, seen map[string]struct{}, candidate presets.MirrorCandidate) []presets.MirrorCandidate {
	key := strings.TrimSpace(candidate.Name) + "|" + strings.TrimSpace(candidate.BaseURL)
	if _, ok := seen[key]; ok {
		return dst
	}
	seen[key] = struct{}{}
	return append(dst, candidate)
}

func mirrorCandidateLabel(candidate presets.MirrorCandidate) string {
	if strings.TrimSpace(candidate.Name) != "" {
		return candidate.Name
	}
	return candidate.BaseURL
}

func (w *Workflow) runPrivileged(ctx context.Context, info system.Info, cmd string, args []string, env map[string]string, dir string, stdout, stderr io.Writer) error {
	if info.OS == "windows" || info.Elevated || !system.HasCommand("sudo") {
		return w.runCommand(ctx, cmd, args, env, dir, stdout, stderr)
	}
	return w.runCommand(ctx, "sudo", append([]string{cmd}, args...), env, dir, stdout, stderr)
}

// RecommendedMode 根据当前系统环境返回建议的安装模式。
// 使用 DockerHealthy（通过 docker info 检测守护进程）而非仅检查 PATH。
func RecommendedMode(info system.Info) Mode {
	if info.OS == "windows" {
		return ModeDocker
	}
	if info.DockerHealthy && info.HasCompose {
		return ModeDocker
	}
	return ModeNative
}

// recommendedMode 内部别名，保留向后兼容。
func recommendedMode(info system.Info) Mode { return RecommendedMode(info) }

func mirrorNames(selection MirrorSelection) map[string]string {
	names := make(map[string]string, len(selection))
	for _, key := range shared.SortedStringKeys(selection) {
		names[key] = selection[key].Name
	}
	return names
}

func validateHTTPSURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("URL scheme 必须为 https: %s", rawURL)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return fmt.Errorf("URL host 不能为空: %s", rawURL)
	}
	return nil
}

func gatewayBindForMode(mode Mode) string {
	if mode == ModeDocker {
		return "lan"
	}
	return "loopback"
}

func bridgeHostForMode(mode Mode) string {
	if mode == ModeDocker {
		return "host.docker.internal"
	}
	return "127.0.0.1"
}

func composeInvocation() (string, []string, error) {
	if system.HasCommand("docker") {
		if err := exec.Command("docker", "compose", "version").Run(); err == nil {
			return "docker", []string{"compose"}, nil
		}
	}
	if system.HasCommand("docker-compose") {
		return "docker-compose", nil, nil
	}
	return "", nil, output.NewFixable("docker compose 不可用",
		"1) Docker Desktop 用户: 在 Preferences 中启用 Compose\n2) Linux 用户: 安装 docker-compose-plugin (sudo apt-get install docker-compose-plugin)")
}

func flattenEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(env))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}

// ChannelIDs 从 ChannelSelection 切片提取排序后的 ID 列表。
func ChannelIDs(channels []config.ChannelSelection) []string {
	out := make([]string, 0, len(channels))
	for _, channel := range channels {
		out = append(out, channel.ID)
	}
	sort.Strings(out)
	return out
}

// channelIDs 内部别名。
func channelIDs(channels []config.ChannelSelection) []string { return ChannelIDs(channels) }

func hasBridgeChannels(channels []config.ChannelSelection) bool {
	for _, channel := range channels {
		if shared.UsesBridgeProvisioner(channel.Provisioner) {
			return true
		}
	}
	return false
}

func hasPluginChannels(channels []config.ChannelSelection) bool {
	for _, channel := range channels {
		if !shared.UsesBridgeProvisioner(channel.Provisioner) {
			return true
		}
	}
	return false
}

func (m Mode) String() string {
	return string(m)
}

func scriptExtension() string {
	if runtime.GOOS == "windows" {
		return ".cmd"
	}
	return ".sh"
}
