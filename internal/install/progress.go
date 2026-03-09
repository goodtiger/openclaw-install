package install

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type progressTracker struct {
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
	total := 6
	if !req.SkipInstall {
		total += 2
	}
	if !req.SkipVerify {
		total++
	}
	return total
}

func (p *progressTracker) Step(title string) {
	if p == nil {
		return
	}
	p.current++
	fmt.Fprintf(p.out, "\n[%d/%d] %s\n", p.current, p.total, title)
}

func (p *progressTracker) Detailf(format string, args ...any) {
	if p == nil {
		return
	}
	fmt.Fprintf(p.out, "  - %s\n", fmt.Sprintf(format, args...))
}

func (p *progressTracker) Command(cmd string, args []string) {
	if p == nil {
		return
	}
	fmt.Fprintf(p.out, "  -> %s\n", formatCommand(cmd, args))
}

func sortedStringMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
