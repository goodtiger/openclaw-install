package doctor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/presets"
)

func TestAllChecksReturnsNonEmpty(t *testing.T) {
	checks := AllChecks()
	if len(checks) == 0 {
		t.Fatal("AllChecks() returned empty slice")
	}
}

func TestDefaultChecksAreSubsetOfAll(t *testing.T) {
	all := AllChecks()
	defaults := DefaultChecks()

	allNames := make(map[string]bool)
	for _, c := range all {
		allNames[c.Name] = true
	}

	for _, c := range defaults {
		if !allNames[c.Name] {
			t.Errorf("default check %q not found in AllChecks", c.Name)
		}
		if !c.IsDefault {
			t.Errorf("default check %q has IsDefault=false", c.Name)
		}
	}
}

func TestCheckByName(t *testing.T) {
	all := AllChecks()
	for _, c := range all {
		found, ok := CheckByName(c.Name)
		if !ok {
			t.Errorf("CheckByName(%q) not found", c.Name)
		}
		if found.Name != c.Name {
			t.Errorf("CheckByName(%q) = %q, want %q", c.Name, found.Name, c.Name)
		}
	}

	_, ok := CheckByName("nonexistent")
	if ok {
		t.Error("CheckByName(\"nonexistent\") should return false")
	}
}

func TestCheckNames(t *testing.T) {
	names := CheckNames()
	all := AllChecks()
	if len(names) != len(all) {
		t.Errorf("CheckNames() returned %d names, want %d", len(names), len(all))
	}
}

func TestCheckNamesUnique(t *testing.T) {
	names := CheckNames()
	seen := make(map[string]bool)
	for _, name := range names {
		if seen[name] {
			t.Errorf("duplicate check name: %q", name)
		}
		seen[name] = true
	}
}

func TestRunChecksWithList(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	ctx := context.Background()
	info := system.Info{}
	bundle := presets.Bundle{}

	_, err := RunChecks(ctx, info, bundle, RunOptions{List: true}, &out, &errOut)
	if err != nil {
		t.Fatalf("RunChecks with --list returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "可用检查项") {
		t.Errorf("expected '可用检查项' in output, got:\n%s", output)
	}
}

func TestRunChecksDefault(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	ctx := context.Background()
	info := system.Info{}
	bundle := presets.Bundle{}

	report, err := RunChecks(ctx, info, bundle, RunOptions{}, &out, &errOut)
	if err != nil {
		t.Fatalf("RunChecks returned error: %v", err)
	}

	defaults := DefaultChecks()
	if len(report.Outcomes) != len(defaults) {
		t.Errorf("got %d outcomes, want %d (default checks)", len(report.Outcomes), len(defaults))
	}
}

func TestRunChecksAll(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	ctx := context.Background()
	info := system.Info{}
	bundle := presets.Bundle{}

	report, err := RunChecks(ctx, info, bundle, RunOptions{All: true}, &out, &errOut)
	if err != nil {
		t.Fatalf("RunChecks with --all returned error: %v", err)
	}

	all := AllChecks()
	if len(report.Outcomes) != len(all) {
		t.Errorf("got %d outcomes, want %d (all checks)", len(report.Outcomes), len(all))
	}
}

func TestRunChecksSpecific(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	ctx := context.Background()
	info := system.Info{}
	bundle := presets.Bundle{}

	report, err := RunChecks(ctx, info, bundle, RunOptions{CheckNames: []string{"docker", "git"}}, &out, &errOut)
	if err != nil {
		t.Fatalf("RunChecks with --check returned error: %v", err)
	}

	if len(report.Outcomes) != 2 {
		t.Errorf("got %d outcomes, want 2", len(report.Outcomes))
	}

	names := make(map[string]bool)
	for _, o := range report.Outcomes {
		names[o.Name] = true
	}
	if !names["docker"] || !names["git"] {
		t.Errorf("expected outcomes for docker and git, got: %v", names)
	}
}

func TestRunChecksNonexistent(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	ctx := context.Background()
	info := system.Info{}
	bundle := presets.Bundle{}

	report, err := RunChecks(ctx, info, bundle, RunOptions{CheckNames: []string{"nonexistent"}}, &out, &errOut)
	if err != nil {
		t.Fatalf("RunChecks with nonexistent check returned error: %v", err)
	}

	if len(report.Outcomes) != 0 {
		t.Errorf("got %d outcomes, want 0 for nonexistent check", len(report.Outcomes))
	}
}

func TestCheckStatusConstants(t *testing.T) {
	if StatusPass == "" {
		t.Error("StatusPass is empty")
	}
	if StatusWarn == "" {
		t.Error("StatusWarn is empty")
	}
	if StatusFail == "" {
		t.Error("StatusFail is empty")
	}
}

func TestCheckHasRunFunction(t *testing.T) {
	for _, c := range AllChecks() {
		if c.Run == nil {
			t.Errorf("check %q has nil Run function", c.Name)
		}
	}
}
