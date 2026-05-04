package app

import (
	"os"
	"runtime"
	"syscall"
	"testing"
)

func TestBridgeServeSignalsAlwaysIncludeInterrupt(t *testing.T) {
	if !containsSignal(bridgeServeSignals(), os.Interrupt) {
		t.Fatalf("bridgeServeSignals() should include os.Interrupt")
	}
}

func TestBridgeServeSignalsIncludeSIGTERMOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM is not used for service shutdown on Windows")
	}

	if !containsSignal(bridgeServeSignals(), syscall.SIGTERM) {
		t.Fatalf("bridgeServeSignals() should include syscall.SIGTERM on Unix")
	}
}

func containsSignal(signals []os.Signal, want os.Signal) bool {
	for _, signal := range signals {
		if signal == want {
			return true
		}
	}
	return false
}
