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
		warnings = append(warnings, "no supported package manager detected; automatic dependency installation may fail")
	}
	if info.OS == "windows" && !info.HasDocker {
		warnings = append(warnings, "Windows v1 only supports Docker mode, but Docker is not currently detected")
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

	w.progressStep("Preparing workspace")

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
		w.progressDetailf("Existing config backed up to %s", backupFile)
	}

	w.progressStep("Resolving mirrors")

	mirrors, mirrorWarnings := w.ResolveMirrors(ctx)
	result.MirrorNames = mirrorNames(mirrors)
	result.Warnings = append(result.Warnings, mirrorWarnings...)
	if len(result.MirrorNames) == 0 {
		w.progressDetailf("No mirror categories defined; using built-in defaults")
	} else {
		for _, key := range sortedStringMapKeys(result.MirrorNames) {
			w.progressDetailf("%s: %s", key, result.MirrorNames[key])
		}
	}

	previousState, err := config.LoadInstallState(info.StatePath)
	if err != nil {
		return Result{}, err
	}

	input := config.ManagedConfigInput{
		InstallerVersion: req.AppVersion,
		Mode:             req.Mode.String(),
		GatewayBind:      gatewayBindForMode(req.Mode),
		BridgeHost:       bridgeHostForMode(req.Mode),
		Provider:         req.Provider,
		Channels:         req.Channels,
		ManagedAt:        w.Now(),
		MirrorNames:      result.MirrorNames,
	}

	existingConfig, err := config.LoadMap(info.ConfigPath)
	if err != nil {
		return Result{}, err
	}
	managedConfig := config.BuildManagedConfig(input)
	finalConfig := config.ApplyManagedConfig(existingConfig, managedConfig, previousState)

	w.progressStep("Writing configuration files")
	if err := config.SaveJSONAtomic(info.ConfigPath, finalConfig); err != nil {
		return Result{}, err
	}
	w.progressDetailf("OpenClaw config -> %s", info.ConfigPath)

	if err := config.SaveJSONAtomic(info.BridgeConfigPath, config.BuildBridgeConfig(input)); err != nil {
		return Result{}, err
	}
	w.progressDetailf("Bridge config -> %s", info.BridgeConfigPath)

	w.progressStep("Generating runtime assets")
	assetWarnings, err := w.writeAssets(ctx, info, req, previousState, mirrors, stdout, stderr)
	if err != nil {
		return Result{}, err
	}
	result.Warnings = append(result.Warnings, assetWarnings...)
	w.progressDetailf("Runtime assets -> %s", info.RuntimeDir)

	state := config.InstallState{
		Version:           req.AppVersion,
		InstalledAt:       w.Now().UTC(),
		Mode:              req.Mode.String(),
		Platform:          info.OS + "/" + info.Arch,
		ManagedProviderID: req.Provider.ID,
		ManagedChannels:   channelIDs(req.Channels),
		MirrorNames:       result.MirrorNames,
		RuntimeDir:        info.RuntimeDir,
		ConfigPath:        info.ConfigPath,
		BridgeConfigPath:  info.BridgeConfigPath,
	}

	w.progressStep("Saving installer state")
	if err := config.SaveInstallState(info.StatePath, state); err != nil {
		return Result{}, err
	}
	w.progressDetailf("Install state -> %s", info.StatePath)

	if !req.SkipInstall {
		w.progressStep("Installing dependencies")
		if err := w.installDependencies(ctx, info, req.Mode, stdout, stderr); err != nil {
			return result, err
		}
		w.progressStep("Installing OpenClaw runtime")
		if err := w.installOpenClaw(ctx, info, req.Mode, mirrors, stdout, stderr); err != nil {
			return result, err
		}
	}

	w.progressStep("Configuring channels")
	if len(req.Channels) == 0 {
		w.progressDetailf("No channels selected")
	}
	channelWarnings, err := w.syncChannels(ctx, info, req, previousState, stdout, stderr)
	result.Warnings = append(result.Warnings, channelWarnings...)
	if err != nil {
		return result, err
	}

	if !req.SkipVerify {
		w.progressStep("Verifying installation")
		verifyWarnings, err := w.verify(ctx, info, req, stdout, stderr)
		result.Warnings = append(result.Warnings, verifyWarnings...)
		if err != nil {
			return result, err
		}
	}

	return result, nil
}

func (w *Workflow) Reconfigure(ctx context.Context, info system.Info, req Request, stdout, stderr io.Writer) (Result, error) {
	req.SkipInstall = true
	return w.Install(ctx, info, req, stdout, stderr)
}

func (r Request) Validate(info system.Info) error {
	if r.Mode == "" {
		r.Mode = recommendedMode(info)
	}
	if info.OS == "windows" && r.Mode == ModeNative {
		return errors.New("Windows v1 only supports Docker mode")
	}
	if r.AppVersion == "" {
		return errors.New("app version is required")
	}
	if strings.TrimSpace(r.Provider.ID) == "" {
		return errors.New("provider id is required")
	}
	if strings.TrimSpace(r.Provider.Name) == "" {
		return errors.New("provider name is required")
	}
	if strings.TrimSpace(r.Provider.Type) == "" {
		r.Provider.Type = "openai-compatible"
	}
	if strings.TrimSpace(r.Provider.BaseURL) == "" {
		return errors.New("provider base URL is required")
	}
	if strings.TrimSpace(r.Provider.PrimaryModel) == "" {
		return errors.New("primary model is required")
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
		return fmt.Errorf("unsupported mode %s", mode)
	}
}

func (w *Workflow) installOpenClaw(ctx context.Context, info system.Info, mode Mode, mirrors MirrorSelection, stdout, stderr io.Writer) error {
	switch mode {
	case ModeDocker:
		return w.installDockerMode(ctx, info, stdout, stderr)
	case ModeNative:
		return w.installNativeMode(ctx, info, mirrors, stdout, stderr)
	default:
		return fmt.Errorf("unsupported mode %s", mode)
	}
}

func (w *Workflow) verify(ctx context.Context, info system.Info, req Request, stdout, stderr io.Writer) ([]string, error) {
	warnings := []string{}
	if _, err := config.LoadMap(info.ConfigPath); err != nil {
		return warnings, fmt.Errorf("verify config %s: %w", info.ConfigPath, err)
	}
	if _, err := config.LoadBridgeConfig(info.BridgeConfigPath); err != nil {
		return warnings, fmt.Errorf("verify bridge config %s: %w", info.BridgeConfigPath, err)
	}

	switch req.Mode {
	case ModeDocker:
		cmd, args, err := composeInvocation()
		if err != nil {
			return warnings, err
		}
		args = append(args, "-f", filepath.Join(info.RuntimeDir, "compose.yaml"), "config")
		if err := w.runCommand(ctx, cmd, args, nil, info.RuntimeDir, stdout, stderr); err != nil {
			return warnings, fmt.Errorf("docker compose verify failed: %w", err)
		}
	}

	switch req.Mode {
	case ModeNative:
		if system.HasCommand("openclaw") {
			if err := w.runCommand(ctx, "openclaw", []string{"config", "validate"}, nil, "", stdout, stderr); err != nil {
				return warnings, fmt.Errorf("openclaw config validate failed: %w", err)
			}
		} else {
			warnings = append(warnings, "openclaw executable is not on PATH yet; restart the shell before retrying")
		}
	case ModeDocker:
		if err := w.runOpenClawCommand(ctx, info, req.Mode, []string{"config", "validate"}, stdout, stderr); err != nil {
			return warnings, fmt.Errorf("openclaw config validate failed: %w", err)
		}
	}

	if hasBridgeChannels(req.Channels) {
		warnings = append(warnings, "bridge services were configured on the host side; verify health with `openclaw-install bridge serve --channel <name>` or your service manager")
	}
	if hasPluginChannels(req.Channels) {
		warnings = append(warnings, "plugin-backed channels were configured through the OpenClaw CLI; check them with `openclaw channels list`")
	}

	return warnings, nil
}

func (w *Workflow) ensureDocker(ctx context.Context, info system.Info, stdout, stderr io.Writer) error {
	if info.HasDocker && info.HasCompose {
		w.progressDetailf("Docker and docker compose are already available")
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
		return errors.New("Docker is not installed and no supported package manager is available")
	}

	if info.OS == "linux" && system.HasCommand("systemctl") {
		_ = w.runPrivileged(ctx, info, "systemctl", []string{"enable", "--now", "docker"}, nil, "", stdout, stderr)
	}
	return nil
}

func (w *Workflow) ensureNode(ctx context.Context, info system.Info, stdout, stderr io.Writer) error {
	if info.HasNode && info.HasNPM {
		w.progressDetailf("Node.js and npm are already available")
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
		return errors.New("Node.js/npm is not installed and no supported package manager is available")
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
	env := map[string]string{
		"NPM_CONFIG_REGISTRY": mirrors["npm_registry"].BaseURL,
	}
	if !info.HasOpenClaw {
		if err := w.runCommand(ctx, "npm", []string{"install", "-g", "openclaw"}, env, "", stdout, stderr); err != nil {
			return err
		}
	} else {
		w.progressDetailf("OpenClaw is already installed; skipping npm global install")
	}
	if system.HasCommand("openclaw") {
		return w.runCommand(ctx, "openclaw", []string{"gateway", "start"}, nil, "", stdout, stderr)
	}
	w.progressDetailf("OpenClaw command is not on PATH yet; gateway start was skipped in this shell")
	return nil
}

func (w *Workflow) runPrivileged(ctx context.Context, info system.Info, cmd string, args []string, env map[string]string, dir string, stdout, stderr io.Writer) error {
	if info.OS == "windows" || info.Elevated || !system.HasCommand("sudo") {
		return w.runCommand(ctx, cmd, args, env, dir, stdout, stderr)
	}
	return w.runCommand(ctx, "sudo", append([]string{cmd}, args...), env, dir, stdout, stderr)
}

func recommendedMode(info system.Info) Mode {
	if info.OS == "windows" {
		return ModeDocker
	}
	if info.HasDocker && info.HasCompose {
		return ModeDocker
	}
	return ModeNative
}

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
	return "", nil, errors.New("docker compose is not available")
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

func channelIDs(channels []config.ChannelSelection) []string {
	out := make([]string, 0, len(channels))
	for _, channel := range channels {
		out = append(out, channel.ID)
	}
	sort.Strings(out)
	return out
}

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
