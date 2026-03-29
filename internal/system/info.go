package system

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type Info struct {
	OS               string
	Arch             string
	HomeDir          string
	OpenClawHome     string
	ConfigPath       string
	BridgeConfigPath string
	StatePath        string
	RuntimeDir       string
	HasDocker        bool
	HasCompose       bool
	DockerHealthy    bool
	HasNode          bool
	HasNPM           bool
	HasOpenClaw      bool
	HasGit           bool
	HasCurl          bool
	PackageManager   string
	ExistingConfig   bool
	Elevated         bool
	IsTTY            bool
}

func Detect() (Info, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Info{}, err
	}

	openClawHome := filepath.Join(homeDir, ".openclaw")

	hasDocker := HasCommand("docker")
	info := Info{
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		HomeDir:          homeDir,
		OpenClawHome:     openClawHome,
		ConfigPath:       filepath.Join(openClawHome, "openclaw.json"),
		BridgeConfigPath: filepath.Join(openClawHome, "bridge.json"),
		StatePath:        filepath.Join(openClawHome, "install-state.json"),
		RuntimeDir:       filepath.Join(openClawHome, "runtime"),
		HasDocker:        hasDocker,
		HasCompose:       hasCompose(),
		DockerHealthy:    hasDocker && dockerDaemonHealthy(),
		HasNode:          HasCommand("node"),
		HasNPM:           HasCommand("npm"),
		HasOpenClaw:      HasCommand("openclaw"),
		HasGit:           HasCommand("git"),
		HasCurl:          HasCommand("curl"),
		PackageManager:   detectPackageManager(),
		ExistingConfig:   fileExists(filepath.Join(openClawHome, "openclaw.json")),
		Elevated:         isElevated(),
		IsTTY:            isTerminal(),
	}

	return info, nil
}

func HasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func hasCompose() bool {
	if HasCommand("docker") {
		if err := exec.Command("docker", "compose", "version").Run(); err == nil {
			return true
		}
	}
	return HasCommand("docker-compose")
}

func detectPackageManager() string {
	switch runtime.GOOS {
	case "darwin":
		if HasCommand("brew") {
			return "brew"
		}
	case "windows":
		if HasCommand("winget") {
			return "winget"
		}
	default:
		for _, name := range []string{"apt-get", "dnf", "yum"} {
			if HasCommand(name) {
				return name
			}
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dockerDaemonHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func isElevated() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	return os.Geteuid() == 0
}
