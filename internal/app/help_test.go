package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelpPrintsChineseUsage(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantSnippet string
	}{
		{
			name:        "install",
			args:        []string{"install", "--help"},
			wantSnippet: "用法：openclaw-install install [参数]",
		},
		{
			name:        "doctor",
			args:        []string{"doctor", "--help"},
			wantSnippet: "用法：openclaw-install doctor [参数]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			var errOut bytes.Buffer

			code := Run(tc.args, strings.NewReader(""), &out, &errOut)
			if code != 0 {
				t.Fatalf("Run() code = %d, want 0; stderr:\n%s", code, errOut.String())
			}
			if !strings.Contains(errOut.String(), tc.wantSnippet) {
				t.Fatalf("expected help output to contain %q, got:\n%s", tc.wantSnippet, errOut.String())
			}
			if strings.Contains(errOut.String(), "执行失败") {
				t.Fatalf("help output should not be treated as an error, got:\n%s", errOut.String())
			}
		})
	}
}
