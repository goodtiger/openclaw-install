package config

import "time"

const DefaultGatewayPort = 18789

type ProviderConfig struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	BaseURL        string          `json:"baseUrl"`
	APIKey         string          `json:"apiKey"`
	API            string          `json:"api,omitempty"`
	PrimaryModel   string          `json:"primaryModel"`
	FallbackModels []string        `json:"fallbackModels,omitempty"`
	Catalog        []ProviderModel `json:"catalog,omitempty"`
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

type ChannelSelection struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Driver          string            `json:"driver"`
	Provisioner     string            `json:"provisioner,omitempty"`
	ListenAddr      string            `json:"listenAddr"`
	Path            string            `json:"path"`
	Fields          map[string]string `json:"fields,omitempty"`
	PluginPackage   string            `json:"pluginPackage,omitempty"`
	OpenClawChannel string            `json:"openClawChannel,omitempty"`
	TokenFields     []string          `json:"tokenFields,omitempty"`
	DMPolicy        string            `json:"dmPolicy,omitempty"`
	GroupPolicy     string            `json:"groupPolicy,omitempty"`
}

type ManagedConfigInput struct {
	InstallerVersion string
	Mode             string
	GatewayBind      string
	BridgeHost       string
	Provider         ProviderConfig
	Channels         []ChannelSelection
	ManagedAt        time.Time
	MirrorNames      map[string]string
}

type BridgeConfig struct {
	Version        int                            `json:"version"`
	SystemPrompt   string                         `json:"systemPrompt"`
	TimeoutSeconds int                            `json:"timeoutSeconds"`
	Provider       ProviderConfig                 `json:"provider"`
	Channels       map[string]BridgeChannelConfig `json:"channels"`
}

type BridgeChannelConfig struct {
	Enabled         bool              `json:"enabled"`
	Driver          string            `json:"driver"`
	Provisioner     string            `json:"provisioner,omitempty"`
	ListenAddr      string            `json:"listenAddr"`
	Path            string            `json:"path"`
	Fields          map[string]string `json:"fields,omitempty"`
	PluginPackage   string            `json:"pluginPackage,omitempty"`
	OpenClawChannel string            `json:"openClawChannel,omitempty"`
	TokenFields     []string          `json:"tokenFields,omitempty"`
	DMPolicy        string            `json:"dmPolicy,omitempty"`
	GroupPolicy     string            `json:"groupPolicy,omitempty"`
}

type InstallState struct {
	Version           string            `json:"version"`
	InstalledAt       time.Time         `json:"installedAt"`
	Mode              string            `json:"mode"`
	Platform          string            `json:"platform"`
	ManagedProviderID string            `json:"managedProviderId"`
	ManagedChannels   []string          `json:"managedChannels"`
	MirrorNames       map[string]string `json:"mirrorNames,omitempty"`
	RuntimeDir        string            `json:"runtimeDir"`
	ConfigPath        string            `json:"configPath"`
	BridgeConfigPath  string            `json:"bridgeConfigPath"`
}
