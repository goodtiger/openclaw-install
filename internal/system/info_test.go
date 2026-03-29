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

func TestDetectReturnsDockerHealthyField(t *testing.T) {
	info, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	// DockerHealthy should be false when HasDocker is false
	if !info.HasDocker && info.DockerHealthy {
		t.Error("DockerHealthy should be false when HasDocker is false")
	}
}

func TestDetectReturnsTTYField(t *testing.T) {
	info, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	// In test context, stdin is usually not a terminal
	_ = info.IsTTY // Just ensure the field exists and doesn't panic
}

func TestIsTerminalDoesNotPanic(t *testing.T) {
	// Should not panic regardless of stdin state
	_ = isTerminal()
}
