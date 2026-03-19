package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/goodtiger/openclaw-install/internal/shared"
)

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func BackupIfExists(path, backupDir string, now time.Time) (string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
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
	if errors.Is(err, os.ErrNotExist) {
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
	if errors.Is(err, os.ErrNotExist) {
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

	tempFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() {
		if tempPath != "" {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	tempPath = ""
	return nil
}

func SaveInstallState(path string, state InstallState) error {
	return SaveJSONAtomic(path, state)
}

func BuildManagedConfig(input ManagedConfigInput) map[string]any {
	providerEntry := map[string]any{
		"baseUrl": input.Provider.BaseURL,
		"apiKey":  shared.ValueOrDefault(input.Provider.APIKey, "YOUR_API_KEY"),
	}
	if strings.TrimSpace(input.Provider.API) != "" {
		providerEntry["api"] = input.Provider.API
	}
	if models := providerCatalogToMaps(providerCatalog(input.Provider)); len(models) > 0 {
		providerEntry["models"] = models
	}

	providers := map[string]any{input.Provider.ID: providerEntry}

	channels := map[string]any{}
	for _, channel := range input.Channels {
		if !usesBridgeProvisioner(channel.Provisioner) {
			continue
		}
		channels[channel.ID] = map[string]any{
			"enabled":     true,
			"driver":      "bridge",
			"bridgeURL":   bridgeURL(input.BridgeHost, channel.ListenAddr, channel.Path),
			"listenAddr":  channel.ListenAddr,
			"path":        channel.Path,
			"dmPolicy":    shared.ValueOrDefault(channel.DMPolicy, "pairing"),
			"groupPolicy": shared.ValueOrDefault(channel.GroupPolicy, "allowlist"),
			"credentials": cloneStringMap(channel.Fields),
			"channelType": channel.Driver,
		}
	}

	agentModels := map[string]any{}
	for _, model := range providerCatalog(input.Provider) {
		agentModels[joinModelID(input.Provider.ID, model.ID)] = map[string]any{}
	}
	if len(agentModels) == 0 && strings.TrimSpace(input.Provider.PrimaryModel) != "" {
		agentModels[joinModelID(input.Provider.ID, input.Provider.PrimaryModel)] = map[string]any{}
	}

	return map[string]any{
		"gateway": map[string]any{
			"port": DefaultGatewayPort,
			"bind": input.GatewayBind,
			"mode": "local",
		},
		"models": map[string]any{
			"mode":      "merge",
			"providers": providers,
		},
		"agents": map[string]any{
			"defaults": map[string]any{
				"model": map[string]any{
					"primary": joinModelID(input.Provider.ID, input.Provider.PrimaryModel),
				},
				"models": agentModels,
			},
		},
		"channels": channels,
	}
}

func BuildBridgeConfig(input ManagedConfigInput) BridgeConfig {
	channels := make(map[string]BridgeChannelConfig, len(input.Channels))
	for _, channel := range input.Channels {
		channels[channel.ID] = BridgeChannelConfig{
			Enabled:         true,
			Driver:          channel.Driver,
			Provisioner:     shared.ValueOrDefault(channel.Provisioner, "bridge"),
			ListenAddr:      channel.ListenAddr,
			Path:            channel.Path,
			Fields:          cloneStringMap(channel.Fields),
			PluginPackage:   channel.PluginPackage,
			OpenClawChannel: channel.OpenClawChannel,
			TokenFields:     slices.Clone(channel.TokenFields),
			DMPolicy:        shared.ValueOrDefault(channel.DMPolicy, "pairing"),
			GroupPolicy:     shared.ValueOrDefault(channel.GroupPolicy, "allowlist"),
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
	deleteNestedKey(base, []string{"meta", "installer"})

	if previous.ManagedProviderID != "" {
		deleteNestedKey(base, []string{"models", "providers", previous.ManagedProviderID})
		deleteNestedKey(base, []string{"models", "primary"})
		deleteNestedKey(base, []string{"models", "fallbacks"})
		deleteNestedKey(base, []string{"agents", "defaults", "model"})
		deleteNestedKey(base, []string{"agents", "defaults", "models"})
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

func providerCatalog(provider ProviderConfig) []ProviderModel {
	if len(provider.Catalog) > 0 {
		return provider.Catalog
	}

	seen := map[string]struct{}{}
	out := []ProviderModel{}
	for _, id := range append([]string{provider.PrimaryModel}, provider.FallbackModels...) {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, ProviderModel{ID: id, Name: id})
	}
	return out
}

func providerCatalogToMaps(models []ProviderModel) []any {
	if len(models) == 0 {
		return nil
	}

	out := make([]any, 0, len(models))
	for _, model := range models {
		entry := map[string]any{
			"id":        model.ID,
			"name":      shared.ValueOrDefault(model.Name, model.ID),
			"reasoning": model.Reasoning,
		}
		if len(model.Input) > 0 {
			entry["input"] = slices.Clone(model.Input)
		}
		entry["cost"] = map[string]any{
			"input":      model.Cost.Input,
			"output":     model.Cost.Output,
			"cacheRead":  model.Cost.CacheRead,
			"cacheWrite": model.Cost.CacheWrite,
		}
		if model.ContextWindow > 0 {
			entry["contextWindow"] = model.ContextWindow
		}
		if model.MaxTokens > 0 {
			entry["maxTokens"] = model.MaxTokens
		}
		out = append(out, entry)
	}
	return out
}

func usesBridgeProvisioner(provisioner string) bool {
	return shared.ValueOrDefault(provisioner, "bridge") == "bridge"
}
