package main

import (
	"fmt"
	"os"

	"github.com/goodtiger/openclaw-install/internal/app"
	"github.com/goodtiger/openclaw-install/internal/update"
)

func main() {
	exitCode := app.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)

	if exitCode == 0 {
		latestVersion := update.CheckForUpdates(app.Version)
		if latestVersion != "" {
			fmt.Fprintf(os.Stderr, "\n💡 A new version is available: v%s → v%s\n", app.Version, latestVersion)
			fmt.Fprintf(os.Stderr, "   Run: openclaw-install upgrade\n\n")
		}
	}

	os.Exit(exitCode)
}
