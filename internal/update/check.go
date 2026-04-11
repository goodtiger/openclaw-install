package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const checkInterval = 24 * time.Hour

type updateState struct {
	LastCheckTime time.Time `json:"lastCheckTime"`
	LatestVersion string    `json:"latestVersion"`
}

// CheckForUpdates returns the latest version string if newer than current, empty string otherwise.
// Uses 24h cached state file to avoid frequent API calls. Returns empty string on any error.
func CheckForUpdates(currentVersion string) string {
	if !ShouldUseUpdateCheck() {
		return ""
	}

	statePath, err := stateFilePath()
	if err != nil {
		return ""
	}

	state, err := loadState(statePath)
	if err != nil {
		return ""
	}

	if !state.LastCheckTime.IsZero() && time.Since(state.LastCheckTime) < checkInterval {
		if isNewerVersion(state.LatestVersion, currentVersion) {
			return state.LatestVersion
		}
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	latest, err := fetchLatestVersion(ctx)
	if err != nil {
		return ""
	}

	_ = saveState(statePath, updateState{
		LastCheckTime: time.Now(),
		LatestVersion: latest,
	})

	if isNewerVersion(latest, currentVersion) {
		return latest
	}
	return ""
}

// ShouldUseUpdateCheck returns false in CI, non-TTY, or when OPENCLAW_NO_UPDATE_NOTIFIER=1.
func ShouldUseUpdateCheck() bool {
	if os.Getenv("OPENCLAW_NO_UPDATE_NOTIFIER") == "1" {
		return false
	}

	ciEnvVars := []string{
		"CI", "CONTINUOUS_INTEGRATION", "GITHUB_ACTIONS", "GITLAB_CI",
		"TRAVIS", "CIRCLECI", "JENKINS_URL", "BUILDKITE",
	}
	for _, envVar := range ciEnvVars {
		if os.Getenv(envVar) != "" {
			return false
		}
	}

	if !isTerminal(os.Stdout) {
		return false
	}

	return true
}

func stateFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".openclaw", "state", "update-check.json"), nil
}

func fetchLatestVersion(ctx context.Context) (string, error) {
	apiURLs := []string{
		"https://api.github.com/repos/goodtiger/openclaw-install/releases/latest",
		"https://ghproxy.com/https://api.github.com/repos/goodtiger/openclaw-install/releases/latest",
	}

	var lastErr error
	for _, apiURL := range apiURLs {
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("User-Agent", "openclaw-install/"+runtime.GOOS+"/"+runtime.GOARCH)

		client := &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("API returned %d", resp.StatusCode)
			continue
		}

		var release struct {
			TagName string `json:"tag_name"`
		}

		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&release); err != nil {
			resp.Body.Close()
			lastErr = err
			continue
		}
		resp.Body.Close()

		version := strings.TrimPrefix(release.TagName, "v")
		if version == "" {
			lastErr = fmt.Errorf("empty version tag")
			continue
		}

		return version, nil
	}

	return "", fmt.Errorf("all GitHub API attempts failed: %w", lastErr)
}

func loadState(path string) (updateState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return updateState{}, nil
		}
		return updateState{}, err
	}

	var state updateState
	if err := json.Unmarshal(data, &state); err != nil {
		return updateState{}, nil
	}
	return state, nil
}

func saveState(path string, state updateState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func isNewerVersion(latest, current string) bool {
	if latest == "" || current == "" {
		return false
	}

	latestParts := parseVersion(latest)
	currentParts := parseVersion(current)

	for i := 0; i < 3; i++ {
		if latestParts[i] > currentParts[i] {
			return true
		}
		if latestParts[i] < currentParts[i] {
			return false
		}
	}

	return false
}

func parseVersion(version string) [3]int {
	var parts [3]int
	components := strings.SplitN(version, ".", 3)

	for i := 0; i < 3 && i < len(components); i++ {
		var val int
		fmt.Sscanf(components[i], "%d", &val)
		parts[i] = val
	}

	return parts
}
