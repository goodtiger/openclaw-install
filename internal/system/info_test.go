package system

import (
	"os"
	"runtime"
	"testing"
)

func TestFileExists(t *testing.T) {
	path := t.TempDir() + "/sample.txt"
	if fileExists(path) {
		t.Fatal("expected missing file to return false")
	}
	if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if !fileExists(path) {
		t.Fatal("expected existing file to return true")
	}
}

func TestIsElevatedMatchesPlatformExpectation(t *testing.T) {
	got := isElevated()
	if runtime.GOOS == "windows" {
		if got {
			t.Fatal("expected windows implementation to report false")
		}
		return
	}
	if got != (os.Geteuid() == 0) {
		t.Fatalf("isElevated() = %v, want %v", got, os.Geteuid() == 0)
	}
}
