package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func BackupIfExists(path, backupDir string, now time.Time) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	if err := EnsureDir(backupDir); err != nil {
		return "", err
	}

	backupName := fmt.Sprintf("%s.backup.%s", filepath.Base(path), now.Format("20060102_150405"))
	backupPath := filepath.Join(backupDir, backupName)

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return "", err
	}
	return backupPath, nil
}

func LoadMap(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}

	if len(strings.TrimSpace(string(content))) == 0 {
		return map[string]any{}, nil
	}

	var data map[string]any
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func LoadBridgeConfig(path string) (BridgeConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return BridgeConfig{}, err
	}
	var cfg BridgeConfig
	if err := json.Unmarshal(content, &cfg); err != nil {
		return BridgeConfig{}, err
	}
	return cfg, nil
}

func LoadInstallState(path string) (InstallState, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return InstallState{}, nil
	}
	if err != nil {
		return InstallState{}, err
	}
	var state InstallState
	if err := json.Unmarshal(content, &state); err != nil {
		return InstallState{}, err
	}
	return state, nil
}

func SaveJSONAtomic(path string, value any) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}

	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')

	tempFile := path + ".tmp"
	if err := os.WriteFile(tempFile, content, 0o600); err != nil {
		return err
	}
	return os.Rename(tempFile, path)
}

func SaveInstallState(path string, state InstallState) error {
	return SaveJSONAtomic(path, state)
}

func BuildManagedConfig(input ManagedConfigInput) map[string]any {
	providers := map[string]any{
		input.Provider.ID: map[string]any{
			"name":         input.Provider.Name,
			"type":         input.Provider.Type,
			"baseUrl":      input.Provider.BaseURL,
			"apiKey":       input.Provider.APIKey,
			"defaultModel": input.Provider.PrimaryModel,
		},
	}

	channels := map[string]any{}
	for _, channel := range input.Channels {
		channels[channel.ID] = map[string]any{
			"enabled":     true,
			"driver":      "bridge",
			"bridgeURL":   bridgeURL(input.BridgeHost, channel.ListenAddr, channel.Path),
			"listenAddr":  channel.ListenAddr,
			"path":        channel.Path,
			"dmPolicy":    valueOrDefault(channel.DMPolicy, "pairing"),
			"groupPolicy": valueOrDefault(channel.GroupPolicy, "allowlist"),
			"credentials": cloneStringMap(channel.Fields),
			"channelType": channel.Driver,
		}
	}

	fallbacks := make([]string, 0, len(input.Provider.FallbackModels))
	for _, model := range input.Provider.FallbackModels {
		fallbacks = append(fallbacks, joinModelID(input.Provider.ID, model))
	}

	return map[string]any{
		"meta": map[string]any{
			"installer": map[string]any{
				"name":          "openclaw-install",
				"version":       input.InstallerVersion,
				"mode":          input.Mode,
				"managedAt":     input.ManagedAt.UTC().Format(time.RFC3339),
				"managedKeys":   []string{"meta.installer", "gateway.bind", "gateway.port", "models.primary", "models.fallbacks", "models.providers", "channels"},
				"mirrorNames":   input.MirrorNames,
				"managedDriver": "bridge",
			},
		},
		"gateway": map[string]any{
			"port": DefaultGatewayPort,
			"bind": input.GatewayBind,
			"mode": "local",
		},
		"models": map[string]any{
			"mode":      "merge",
			"primary":   joinModelID(input.Provider.ID, input.Provider.PrimaryModel),
			"fallbacks": fallbacks,
			"providers": providers,
		},
		"channels": channels,
	}
}

func BuildBridgeConfig(input ManagedConfigInput) BridgeConfig {
	channels := make(map[string]BridgeChannelConfig, len(input.Channels))
	for _, channel := range input.Channels {
		channels[channel.ID] = BridgeChannelConfig{
			Enabled:     true,
			Driver:      channel.Driver,
			ListenAddr:  channel.ListenAddr,
			Path:        channel.Path,
			Fields:      cloneStringMap(channel.Fields),
			DMPolicy:    valueOrDefault(channel.DMPolicy, "pairing"),
			GroupPolicy: valueOrDefault(channel.GroupPolicy, "allowlist"),
		}
	}

	return BridgeConfig{
		Version:        1,
		SystemPrompt:   "You are an OpenClaw channel assistant. Reply clearly and briefly in Chinese unless the user asks otherwise.",
		TimeoutSeconds: 30,
		Provider:       input.Provider,
		Channels:       channels,
	}
}

func ApplyManagedConfig(existing, managed map[string]any, previous InstallState) map[string]any {
	base := cloneMap(existing)

	if previous.ManagedProviderID != "" {
		deleteNestedKey(base, []string{"models", "providers", previous.ManagedProviderID})
	}
	for _, channelID := range previous.ManagedChannels {
		deleteNestedKey(base, []string{"channels", channelID})
	}

	return MergeMaps(base, managed)
}

func MergeMaps(dst, src map[string]any) map[string]any {
	out := cloneMap(dst)
	for key, value := range src {
		srcMap, srcIsMap := asStringAnyMap(value)
		dstMap, dstIsMap := asStringAnyMap(out[key])
		if srcIsMap && dstIsMap {
			out[key] = MergeMaps(dstMap, srcMap)
			continue
		}
		out[key] = cloneValue(value)
	}
	return out
}

func cloneMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneValue(value)
	}
	return out
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneValue(item)
		}
		return out
	case []string:
		return slices.Clone(typed)
	case map[string]string:
		return cloneStringMap(typed)
	default:
		return typed
	}
}

func asStringAnyMap(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	return typed, ok
}

func deleteNestedKey(root map[string]any, path []string) {
	if len(path) == 0 {
		return
	}

	current := root
	for i := 0; i < len(path)-1; i++ {
		next, ok := current[path[i]].(map[string]any)
		if !ok {
			return
		}
		current = next
	}

	delete(current, path[len(path)-1])
}

func bridgeURL(bridgeHost, listenAddr, path string) string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		if path == "" {
			return "http://" + listenAddr
		}
		return "http://" + listenAddr + path
	}
	if bridgeHost != "" {
		host = bridgeHost
	}
	return fmt.Sprintf("http://%s:%s%s", host, port, path)
}

func joinModelID(providerID, model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	return providerID + "/" + model
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
