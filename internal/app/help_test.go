package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunInstallHelpReturnsZeroAndPrintsChineseUsage(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := Run([]string{"install", "--help"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr:\n%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "用法：openclaw-install install [参数]") {
		t.Fatalf("expected Chinese install help, got:\n%s", errOut.String())
	}
	if strings.Contains(errOut.String(), "执行失败") {
		t.Fatalf("help output should not be treated as an error, got:\n%s", errOut.String())
	}
}

func TestRunDoctorHelpReturnsZeroAndPrintsChineseUsage(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := Run([]string{"doctor", "--help"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr:\n%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "用法：openclaw-install doctor [参数]") {
		t.Fatalf("expected Chinese doctor help, got:\n%s", errOut.String())
	}
}
