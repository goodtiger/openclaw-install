package install

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/goodtiger/openclaw-install/internal/system"
)

func (w *Workflow) resolveNPMExecutable(info system.Info) (string, error) {
	if path, err := exec.LookPath("npm"); err == nil {
		return path, nil
	}

	if info.OS == "windows" {
		for _, candidate := range windowsNodeCommandCandidates("npm") {
			if executableExists(candidate) {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("找不到 npm 命令")
}

func (w *Workflow) npmGlobalPrefix(ctx context.Context, info system.Info, stderr io.Writer) (string, error) {
	npmPath, err := w.resolveNPMExecutable(info)
	if err != nil {
		return "", err
	}

	if stderr == nil {
		stderr = io.Discard
	}

	var out bytes.Buffer
	if err := w.Executor.Run(ctx, npmPath, []string{"prefix", "-g"}, nil, "", &out, stderr); err != nil {
		return "", fmt.Errorf("执行 npm prefix -g 失败: %w", err)
	}

	prefix := strings.TrimSpace(out.String())
	if prefix == "" {
		return "", fmt.Errorf("npm prefix -g 没有返回目录")
	}
	return prefix, nil
}

func (w *Workflow) resolveOpenClawExecutable(ctx context.Context, info system.Info, stderr io.Writer) (string, error) {
	if path, err := exec.LookPath("openclaw"); err == nil {
		return path, nil
	}

	prefix, err := w.npmGlobalPrefix(ctx, info, stderr)
	if err != nil {
		return "", fmt.Errorf("找不到 openclaw 命令: %w", err)
	}

	candidates := openClawExecutableCandidates(prefix, info.OS)
	for _, candidate := range candidates {
		if executableExists(candidate) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("找不到 openclaw 命令")
}

func openClawExecutableCandidates(prefix, osName string) []string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}

	if osName == "windows" {
		return []string{
			filepath.Join(prefix, "openclaw.cmd"),
			filepath.Join(prefix, "openclaw"),
		}
	}

	return []string{
		filepath.Join(prefix, "bin", "openclaw"),
		filepath.Join(prefix, "openclaw"),
	}
}

func windowsNodeCommandCandidates(name string) []string {
	candidates := []string{}
	for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		candidates = append(candidates,
			filepath.Join(root, "nodejs", name+".cmd"),
			filepath.Join(root, "nodejs", name),
		)
	}
	return candidates
}

func executableExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
