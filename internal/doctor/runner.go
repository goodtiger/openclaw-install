package doctor

import (
	"context"
	"fmt"
	"io"

	"github.com/goodtiger/openclaw-install/internal/shared"
	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/presets"
)

type RunOptions struct {
	CheckNames []string
	All        bool
	Fix        bool
	List       bool
}

type CheckOutcome struct {
	Name         string
	Description  string
	Status       CheckStatus
	Message      string
	FixAttempted bool
	FixError     error
}

type RunReport struct {
	Info        system.Info
	Outcomes    []CheckOutcome
	MirrorNames map[string]string
}

func RunChecks(ctx context.Context, info system.Info, bundle presets.Bundle, opts RunOptions, out, errOut io.Writer) (RunReport, error) {
	if opts.List {
		return printCheckList(out)
	}

	checks := selectChecks(opts)
	results := make([]CheckOutcome, 0, len(checks))

	for _, check := range checks {
		result := check.Run(ctx, info, bundle)
		outcome := CheckOutcome{
			Name:        check.Name,
			Description: check.Description,
			Status:      result.Status,
			Message:     result.Message,
		}

		if opts.Fix && (result.Status == StatusWarn || result.Status == StatusFail) && check.Fix != nil {
			outcome.FixAttempted = true
			if err := check.Fix(ctx, info); err != nil {
				outcome.FixError = err
				fmt.Fprintf(errOut, "  修复 %s 失败: %v\n", check.Name, err)
			} else {
				outcome.Status = StatusPass
				outcome.Message = ""
				fmt.Fprintf(out, "  已修复: %s\n", check.Name)
			}
		}

		results = append(results, outcome)
	}

	mirrorNames := resolveMirrorNames(ctx, bundle)

	return RunReport{
		Info:        info,
		Outcomes:    results,
		MirrorNames: mirrorNames,
	}, nil
}

func selectChecks(opts RunOptions) []Check {
	if opts.All {
		return AllChecks()
	}

	if len(opts.CheckNames) > 0 {
		selected := make([]Check, 0, len(opts.CheckNames))
		for _, name := range opts.CheckNames {
			if c, ok := CheckByName(name); ok {
				selected = append(selected, c)
			}
		}
		return selected
	}

	return DefaultChecks()
}

func printCheckList(out io.Writer) (RunReport, error) {
	fmt.Fprintln(out, "可用检查项：")
	fmt.Fprintln(out, "")
	for _, c := range AllChecks() {
		marker := "  "
		if c.IsDefault {
			marker = "* "
		}
		fmt.Fprintf(out, "%s%-30s %s\n", marker, c.Name, c.Description)
	}
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "* = 默认启用（无参数时运行）")
	fmt.Fprintln(out, "  使用 --all 运行所有检查项")
	return RunReport{}, nil
}

func resolveMirrorNames(ctx context.Context, bundle presets.Bundle) map[string]string {
	names := make(map[string]string)
	for _, key := range resolveMirrorCategories(bundle) {
		candidates := bundle.Mirrors.Categories[key]
		if len(candidates) > 0 {
			chosen, _ := chooseMirror(ctx, key, candidates)
			if chosen.Name != "" {
				names[key] = chosen.Name
			}
		}
	}
	return names
}

func resolveMirrorCategories(bundle presets.Bundle) []string {
	return shared.SortedStringKeys(bundle.Mirrors.Categories)
}
