package config

import "time"

const DefaultGatewayPort = 18789

type ProviderConfig struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	BaseURL        string   `json:"baseUrl"`
	APIKey         string   `json:"apiKey"`
	PrimaryModel   string   `json:"primaryModel"`
	FallbackModels []string `json:"fallbackModels,omitempty"`
}

type ChannelSelection struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Driver      string            `json:"driver"`
	ListenAddr  string            `json:"listenAddr"`
	Path        string            `json:"path"`
	Fields      map[string]string `json:"fields,omitempty"`
	DMPolicy    string            `json:"dmPolicy,omitempty"`
	GroupPolicy string            `json:"groupPolicy,omitempty"`
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
	Enabled     bool              `json:"enabled"`
	Driver      string            `json:"driver"`
	ListenAddr  string            `json:"listenAddr"`
	Path        string            `json:"path"`
	Fields      map[string]string `json:"fields,omitempty"`
	DMPolicy    string            `json:"dmPolicy,omitempty"`
	GroupPolicy string            `json:"groupPolicy,omitempty"`
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
