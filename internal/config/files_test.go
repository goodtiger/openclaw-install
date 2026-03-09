package config

import (
	"testing"
	"time"
)

func TestApplyManagedConfigPreservesUnknownAndReplacesManagedEntries(t *testing.T) {
	existing := map[string]any{
		"custom": "keep-me",
		"channels": map[string]any{
			"telegram": map[string]any{"enabled": true},
			"qq":       map[string]any{"enabled": true, "legacy": true},
		},
		"models": map[string]any{
			"providers": map[string]any{
				"legacy-provider": map[string]any{"baseUrl": "http://legacy"},
				"other":           map[string]any{"baseUrl": "http://keep"},
			},
		},
	}

	input := ManagedConfigInput{
		InstallerVersion: "0.1.0",
		Mode:             "native",
		GatewayBind:      "127.0.0.1",
		BridgeHost:       "127.0.0.1",
		ManagedAt:        time.Unix(1700000000, 0),
		MirrorNames:      map[string]string{"npm_registry": "npmmirror"},
		Provider: ProviderConfig{
			ID:           "deepseek",
			Name:         "DeepSeek",
			Type:         "openai-compatible",
			BaseURL:      "https://api.deepseek.com/v1",
			APIKey:       "test-key",
			PrimaryModel: "deepseek-chat",
		},
		Channels: []ChannelSelection{
			{
				ID:         "feishu",
				Name:       "Feishu",
				Driver:     "feishu",
				ListenAddr: "127.0.0.1:19091",
				Path:       "/feishu/events",
				Fields: map[string]string{
					"app_id": "cli_xxx",
				},
			},
		},
	}

	previous := InstallState{
		ManagedProviderID: "legacy-provider",
		ManagedChannels:   []string{"qq"},
	}

	managed := BuildManagedConfig(input)
	merged := ApplyManagedConfig(existing, managed, previous)

	if merged["custom"] != "keep-me" {
		t.Fatalf("expected custom key to be preserved, got %#v", merged["custom"])
	}

	channels := merged["channels"].(map[string]any)
	if _, ok := channels["telegram"]; !ok {
		t.Fatal("expected unmanaged telegram channel to remain")
	}
	if _, ok := channels["qq"]; ok {
		t.Fatal("expected previously managed qq channel to be removed")
	}
	if _, ok := channels["feishu"]; !ok {
		t.Fatal("expected newly managed feishu channel to be added")
	}

	models := merged["models"].(map[string]any)
	providers := models["providers"].(map[string]any)
	if _, ok := providers["legacy-provider"]; ok {
		t.Fatal("expected previously managed provider to be removed")
	}
	if _, ok := providers["other"]; !ok {
		t.Fatal("expected unmanaged provider to be preserved")
	}
	if _, ok := providers["deepseek"]; !ok {
		t.Fatal("expected deepseek provider to be added")
	}
}
