package install

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

const (
	progressStepPrepareWorkspace = "准备工作目录"
	progressStepResolveMirrors   = "解析镜像源"
	progressStepWriteConfig      = "写入配置文件"
	progressStepGenerateAssets   = "生成运行时文件"
	progressStepSaveState        = "保存安装状态"
	progressStepInstallDeps      = "安装依赖"
	progressStepInstallRuntime   = "安装 OpenClaw 运行时"
	progressStepConfigureChannel = "配置通道"
	progressStepVerify           = "验证安装结果"
)

var baseProgressSteps = []string{
	progressStepPrepareWorkspace,
	progressStepResolveMirrors,
	progressStepWriteConfig,
	progressStepGenerateAssets,
	progressStepSaveState,
	progressStepConfigureChannel,
}

var installRuntimeProgressSteps = []string{
	progressStepInstallDeps,
	progressStepInstallRuntime,
}

var verifyProgressSteps = []string{
	progressStepVerify,
}

type progressTracker struct {
	mu      sync.Mutex
	out     io.Writer
	total   int
	current int
}

func newProgressTracker(out io.Writer, total int) *progressTracker {
	if out == nil || total <= 0 {
		return nil
	}
	return &progressTracker{
		out:   out,
		total: total,
	}
}

func installStepCount(req Request) int {
	total := len(baseProgressSteps)
	if !req.SkipInstall {
		total += len(installRuntimeProgressSteps)
	}
	if !req.SkipVerify {
		total += len(verifyProgressSteps)
	}
	return total
}

func (p *progressTracker) Step(title string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current++
	fmt.Fprintf(p.out, "\n[%d/%d] %s\n", p.current, p.total, title)
}

func (p *progressTracker) Detailf(format string, args ...any) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.out, "  - %s\n", fmt.Sprintf(format, args...))
}

func (p *progressTracker) Command(cmd string, args []string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.out, "  -> %s\n", formatCommand(cmd, args))
}

func formatCommand(cmd string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteCommandArg(cmd))
	for _, arg := range args {
		parts = append(parts, quoteCommandArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteCommandArg(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\n\"'") {
		return strconv.Quote(value)
	}
	return value
}
