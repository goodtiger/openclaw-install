package presets

import (
	"embed"
	"encoding/json"
	"fmt"
)

type MirrorManifest struct {
	Categories map[string][]MirrorCandidate `json:"categories"`
}

type MirrorCandidate struct {
	Name        string            `json:"name"`
	BaseURL     string            `json:"base_url"`
	ProbeURL    string            `json:"probe_url"`
	ImagePrefix string            `json:"image_prefix,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

type ModelCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

type ProviderModel struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Reasoning     bool      `json:"reasoning"`
	Input         []string  `json:"input,omitempty"`
	Cost          ModelCost `json:"cost,omitempty"`
	ContextWindow int       `json:"contextWindow,omitempty"`
	MaxTokens     int       `json:"maxTokens,omitempty"`
}

type ProviderPreset struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	BaseURL      string          `json:"base_url"`
	API          string          `json:"api,omitempty"`
	APIKeyEnv    string          `json:"api_key_env"`
	DefaultModel string          `json:"default_model,omitempty"`
	Models       []string        `json:"models"`
	Catalog      []ProviderModel `json:"catalog,omitempty"`
	Notes        string          `json:"notes"`
}

type CredentialField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Secret   bool   `json:"secret"`
	Optional bool   `json:"optional,omitempty"`
}

type ChannelPreset struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Driver          string            `json:"driver"`
	Provisioner     string            `json:"provisioner,omitempty"`
	DefaultEnabled  bool              `json:"default_enabled,omitempty"`
	DefaultListen   string            `json:"default_listen"`
	DefaultPath     string            `json:"default_path"`
	PluginPackage   string            `json:"plugin_package,omitempty"`
	OpenClawChannel string            `json:"openclaw_channel,omitempty"`
	TokenFields     []string          `json:"token_fields,omitempty"`
	Notes           string            `json:"notes"`
	RequiredFields  []CredentialField `json:"required_fields"`
	Defaults        map[string]any    `json:"defaults"`
}

type Bundle struct {
	Mirrors   MirrorManifest
	Providers []ProviderPreset
	Channels  []ChannelPreset
}

type providerFile struct {
	Providers []ProviderPreset `json:"providers"`
}

type channelFile struct {
	Channels []ChannelPreset `json:"channels"`
}

//go:embed *.yaml
var files embed.FS

func Load() (Bundle, error) {
	var bundle Bundle
	if err := loadJSONFile("mirrors.yaml", &bundle.Mirrors); err != nil {
		return Bundle{}, err
	}

	var providers providerFile
	if err := loadJSONFile("providers.yaml", &providers); err != nil {
		return Bundle{}, err
	}
	bundle.Providers = providers.Providers

	var channels channelFile
	if err := loadJSONFile("channels.yaml", &channels); err != nil {
		return Bundle{}, err
	}
	bundle.Channels = channels.Channels

	return bundle, nil
}

func (b Bundle) ProviderByID(id string) (ProviderPreset, bool) {
	for _, provider := range b.Providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return ProviderPreset{}, false
}

func (b Bundle) ChannelByID(id string) (ChannelPreset, bool) {
	for _, channel := range b.Channels {
		if channel.ID == id {
			return channel, true
		}
	}
	return ChannelPreset{}, false
}

func loadJSONFile(name string, dst any) error {
	content, err := files.ReadFile(name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	if err := json.Unmarshal(content, dst); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	return nil
}
