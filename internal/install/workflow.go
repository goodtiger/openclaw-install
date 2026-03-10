package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/goodtiger/openclaw-install/internal/config"
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
	progress   *progressTracker
}

func NewWorkflow(bundle presets.Bundle, executor Executor) *Workflow {
	if executor == nil {
		executor = RealExecutor{}
	}
	return &Workflow{
		Presets:    bundle,
		Executor:   executor,
		HTTPClient: &http.Client{Timeout: 3 * time.Second},
		Now:        time.Now,
	}
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
	w.progress = newProgressTracker(out, installStepCount(req))
	return func() {
		w.progress = nil
	}
}

func (w *Workflow) progressStep(title string) {
	if w.progress != nil {
		w.progress.Step(title)
	}
}

func (w *Workflow) progressDetailf(format string, args ...any) {
	if w.progress != nil {
		w.progress.Detailf(format, args...)
	}
}

func (w *Workflow) runCommand(ctx context.Context, cmd string, args []string, env map[string]string, dir string, stdout, stderr io.Writer) error {
	if w.progress != nil {
		w.progress.Command(cmd, args)
	}
	return w.Executor.Run(ctx, cmd, args, env, dir, stdout, stderr)
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

	return DoctorReport{
		Info:            info,
		RecommendedMode: recommendedMode(info),
		MirrorNames:     mirrorNames(mirrors),
		Warnings:        warnings,
	}, nil
}

func (w *Workflow) Install(ctx context.Context, info system.Info, req Request, stdout, stderr io.Writer) (Result, error) {
	if err := req.Validate(info); err != nil {
		return Result{}, err
	}
	resetProgress := w.beginProgress(stdout, req)
	defer resetProgress()

	w.progressStep("准备工作目录")
	if err := config.EnsureDir(info.OpenClawHome); err != nil {
		return Result{}, err
	}

	result := Result{
		ConfigPath:       info.ConfigPath,
		BridgeConfigPath: info.BridgeConfigPath,
		StatePath:        info.StatePath,
		RuntimeDir:       info.RuntimeDir,
	}

	backupFile, err := config.BackupIfExists(info.ConfigPath, filepath.Join(info.OpenClawHome, ".backups"), w.Now())
	if err != nil {
		return Result{}, err
	}
	result.BackupFile = backupFile
	if backupFile != "" {
		w.progressDetailf("已备份现有配置到 %s", backupFile)
	}

	w.progressStep("解析镜像源")
	mirrors, mirrorWarnings := w.ResolveMirrors(ctx)
	result.MirrorNames = mirrorNames(mirrors)
	result.Warnings = append(result.Warnings, mirrorWarnings...)
	if len(result.MirrorNames) == 0 {
		w.progressDetailf("未定义镜像分类，使用内置默认值")
	} else {
		for _, key := range sortedStringMapKeys(result.MirrorNames) {
			w.progressDetailf("%s：%s", key, result.MirrorNames[key])
		}
	}

	previousState, err := config.LoadInstallState(info.StatePath)
	if err != nil {
		return Result{}, err
	}

	assetWarnings, err := w.applyConfigAndAssets(ctx, info, req, previousState, mirrors, result.MirrorNames, stdout, stderr)
	if err != nil {
		return Result{}, err
	}
	result.Warnings = append(result.Warnings, assetWarnings...)

	if !req.SkipInstall {
		w.progressStep("安装依赖")
		if err := w.installDependencies(ctx, info, req.Mode, stdout, stderr); err != nil {
			return result, err
		}
		w.progressStep("安装 OpenClaw 运行时")
		if err := w.installOpenClaw(ctx, info, req.Mode, mirrors, stdout, stderr); err != nil {
			return result, err
		}
	}

	w.progressStep("配置通道")
	if len(req.Channels) == 0 {
		w.progressDetailf("未启用任何通道")
	}
	channelWarnings, err := w.syncChannels(ctx, info, req, previousState, stdout, stderr)
	result.Warnings = append(result.Warnings, channelWarnings...)
	if err != nil {
		return result, err
	}

	if !req.SkipVerify {
		w.progressStep("验证安装结果")
		verifyWarnings, err := w.verify(ctx, info, req, stdout, stderr)
		result.Warnings = append(result.Warnings, verifyWarnings...)
		if err != nil {
			return result, err
		}
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
		return nil, err
	}
	managedConfig := config.BuildManagedConfig(input)
	finalConfig := config.ApplyManagedConfig(existingConfig, managedConfig, previousState)

	w.progressStep("写入配置文件")
	if err := config.SaveJSONAtomic(info.ConfigPath, finalConfig); err != nil {
		return nil, err
	}
	w.progressDetailf("OpenClaw 配置 -> %s", info.ConfigPath)

	if err := config.SaveJSONAtomic(info.BridgeConfigPath, config.BuildBridgeConfig(input)); err != nil {
		return nil, err
	}
	w.progressDetailf("Bridge 配置 -> %s", info.BridgeConfigPath)

	w.progressStep("生成运行时文件")
	assetWarnings, err := w.writeAssets(ctx, info, req, previousState, mirrors, stdout, stderr)
	if err != nil {
		return nil, err
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

	w.progressStep("保存安装状态")
	if err := config.SaveInstallState(info.StatePath, state); err != nil {
		return nil, err
	}
	w.progressDetailf("安装状态 -> %s", info.StatePath)

	return warnings, nil
}

func (w *Workflow) Reconfigure(ctx context.Context, info system.Info, req Request, stdout, stderr io.Writer) (Result, error) {
	reconfigReq := req
	reconfigReq.SkipInstall = true
	return w.Install(ctx, info, reconfigReq, stdout, stderr)
}

func (r *Request) Validate(info system.Info) error {
	if r.Mode == "" {
		r.Mode = recommendedMode(info)
	}
	if r.AppVersion == "" {
		return errors.New("缺少安装器版本号")
	}
	if strings.TrimSpace(r.Provider.ID) == "" {
		return errors.New("缺少供应商 ID")
	}
	if strings.TrimSpace(r.Provider.Name) == "" {
		return errors.New("缺少供应商名称")
	}
	if strings.TrimSpace(r.Provider.Type) == "" {
		r.Provider.Type = "openai-compatible"
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
		return fmt.Errorf("不支持的安装模式 %s", mode)
	}
}

func (w *Workflow) installOpenClaw(ctx context.Context, info system.Info, mode Mode, mirrors MirrorSelection, stdout, stderr io.Writer) error {
	switch mode {
	case ModeDocker:
		return w.installDockerMode(ctx, info, stdout, stderr)
	case ModeNative:
		return w.installNativeMode(ctx, info, mirrors, stdout, stderr)
	default:
		return fmt.Errorf("不支持的安装模式 %s", mode)
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
			return warnings, err
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

	switch info.PackageManager {
	case "apt-get":
		if err := w.runPrivileged(ctx, info, "apt-get", []string{"update"}, nil, "", stdout, stderr); err != nil {
			return err
		}
		if err := w.runPrivileged(ctx, info, "apt-get", []string{"install", "-y", "docker.io", "docker-compose-plugin"}, nil, "", stdout, stderr); err != nil {
			return err
		}
	case "dnf":
		if err := w.runPrivileged(ctx, info, "dnf", []string{"install", "-y", "docker", "docker-compose"}, nil, "", stdout, stderr); err != nil {
			return err
		}
	case "yum":
		if err := w.runPrivileged(ctx, info, "yum", []string{"install", "-y", "docker"}, nil, "", stdout, stderr); err != nil {
			return err
		}
	case "brew":
		if err := w.runCommand(ctx, "brew", []string{"install", "--cask", "docker"}, nil, "", stdout, stderr); err != nil {
			return err
		}
	case "winget":
		if err := w.runCommand(ctx, "winget", []string{"install", "-e", "--id", "Docker.DockerDesktop"}, nil, "", stdout, stderr); err != nil {
			return err
		}
	default:
		return errors.New("未安装 Docker，且没有可用的包管理器用于自动安装")
	}

	if info.OS == "linux" && system.HasCommand("systemctl") {
		_ = w.runPrivileged(ctx, info, "systemctl", []string{"enable", "--now", "docker"}, nil, "", stdout, stderr)
	}
	return nil
}

func (w *Workflow) ensureNode(ctx context.Context, info system.Info, stdout, stderr io.Writer) error {
	if info.HasNode && info.HasNPM {
		w.progressDetailf("Node.js 和 npm 已可用")
		return nil
	}

	switch info.PackageManager {
	case "apt-get":
		if err := w.runPrivileged(ctx, info, "apt-get", []string{"update"}, nil, "", stdout, stderr); err != nil {
			return err
		}
		if err := w.runPrivileged(ctx, info, "apt-get", []string{"install", "-y", "nodejs", "npm"}, nil, "", stdout, stderr); err != nil {
			return err
		}
	case "dnf":
		if err := w.runPrivileged(ctx, info, "dnf", []string{"install", "-y", "nodejs", "npm"}, nil, "", stdout, stderr); err != nil {
			return err
		}
	case "yum":
		if err := w.runPrivileged(ctx, info, "yum", []string{"install", "-y", "nodejs", "npm"}, nil, "", stdout, stderr); err != nil {
			return err
		}
	case "brew":
		if err := w.runCommand(ctx, "brew", []string{"install", "node"}, nil, "", stdout, stderr); err != nil {
			return err
		}
	case "winget":
		if err := w.runCommand(ctx, "winget", []string{"install", "-e", "--id", "OpenJS.NodeJS.LTS"}, nil, "", stdout, stderr); err != nil {
			return err
		}
	default:
		return errors.New("未安装 Node.js/npm，且没有可用的包管理器用于自动安装")
	}

	return nil
}

func (w *Workflow) installDockerMode(ctx context.Context, info system.Info, stdout, stderr io.Writer) error {
	cmd, args, err := composeInvocation()
	if err != nil {
		return err
	}
	args = append(args, "-f", filepath.Join(info.RuntimeDir, "compose.yaml"), "up", "-d", "--build")
	return w.runCommand(ctx, cmd, args, nil, info.RuntimeDir, stdout, stderr)
}

func (w *Workflow) installNativeMode(ctx context.Context, info system.Info, mirrors MirrorSelection, stdout, stderr io.Writer) error {
	openClawPath, err := w.resolveOpenClawExecutable(ctx, info, io.Discard)
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

		var installErr error
		for idx, candidate := range candidates {
			registryURL := strings.TrimSpace(candidate.BaseURL)
			if registryURL == "" {
				continue
			}

			w.progressDetailf("尝试 npm 源 %s (%s)", mirrorCandidateLabel(candidate), registryURL)
			env := map[string]string{
				"NPM_CONFIG_REGISTRY": registryURL,
				"npm_config_registry": registryURL,
			}
			installErr = w.runCommand(ctx, npmPath, []string{"install", "-g", "openclaw"}, env, "", stdout, stderr)
			if installErr == nil {
				if idx > 0 {
					w.progressDetailf("切换到 %s 后 npm 安装成功", mirrorCandidateLabel(candidate))
				}
				break
			}

			if idx < len(candidates)-1 {
				w.progressDetailf("使用 %s 安装失败，继续尝试下一个源", mirrorCandidateLabel(candidate))
			}
		}
		if installErr != nil {
			return installErr
		}

		openClawPath, err = w.resolveOpenClawExecutable(ctx, info, stderr)
		if err != nil {
			w.progressDetailf("安装完成，但当前仍找不到 openclaw，已跳过 gateway start")
			return nil
		}
	} else {
		w.progressDetailf("OpenClaw 已安装，跳过 npm 全局安装")
	}

	return w.runCommand(ctx, openClawPath, []string{"gateway", "start"}, nil, "", stdout, stderr)
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
func RecommendedMode(info system.Info) Mode {
	if info.OS == "windows" {
		return ModeDocker
	}
	if info.HasDocker && info.HasCompose {
		return ModeDocker
	}
	return ModeNative
}

// recommendedMode 内部别名，保留向后兼容。
func recommendedMode(info system.Info) Mode { return RecommendedMode(info) }

func mirrorNames(selection MirrorSelection) map[string]string {
	names := make(map[string]string, len(selection))
	keys := make([]string, 0, len(selection))
	for key := range selection {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		names[key] = selection[key].Name
	}
	return names
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
	return "", nil, errors.New("docker compose 不可用")
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
		if usesBridgeProvisioner(channel.Provisioner) {
			return true
		}
	}
	return false
}

func hasPluginChannels(channels []config.ChannelSelection) bool {
	for _, channel := range channels {
		if !usesBridgeProvisioner(channel.Provisioner) {
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
