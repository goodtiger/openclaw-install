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

func TestBuildManagedConfigBailianDefaultsAndSkipsPluginChannels(t *testing.T) {
	input := ManagedConfigInput{
		InstallerVersion: "0.1.0",
		Mode:             "native",
		GatewayBind:      "127.0.0.1",
		BridgeHost:       "127.0.0.1",
		ManagedAt:        time.Unix(1700000000, 0),
		MirrorNames:      map[string]string{"npm_registry": "npmmirror"},
		Provider: ProviderConfig{
			ID:           "bailian",
			Name:         "Alibaba Bailian Coding Plan",
			Type:         "openai-compatible",
			BaseURL:      "https://coding.dashscope.aliyuncs.com/v1",
			APIKey:       "YOUR_API_KEY",
			API:          "openai-completions",
			PrimaryModel: "qwen3.5-plus",
			Catalog: []ProviderModel{
				{
					ID:            "qwen3.5-plus",
					Name:          "qwen3.5-plus",
					Input:         []string{"text", "image"},
					ContextWindow: 1000000,
					MaxTokens:     65536,
				},
				{
					ID:            "qwen3-coder-plus",
					Name:          "qwen3-coder-plus",
					Input:         []string{"text"},
					ContextWindow: 1000000,
					MaxTokens:     65536,
				},
			},
		},
		Channels: []ChannelSelection{
			{
				ID:              "qq",
				Name:            "QQ (qqbot plugin)",
				Driver:          "qqbot",
				Provisioner:     "openclaw-plugin",
				PluginPackage:   "@sliverp/qqbot@latest",
				OpenClawChannel: "qqbot",
				TokenFields:     []string{"app_id", "app_secret"},
				Fields: map[string]string{
					"app_id":     "123",
					"app_secret": "456",
				},
			},
		},
	}

	managed := BuildManagedConfig(input)

	models := managed["models"].(map[string]any)
	providers := models["providers"].(map[string]any)
	bailian := providers["bailian"].(map[string]any)

	if bailian["baseUrl"] != "https://coding.dashscope.aliyuncs.com/v1" {
		t.Fatalf("unexpected bailian baseUrl: %#v", bailian["baseUrl"])
	}
	if bailian["api"] != "openai-completions" {
		t.Fatalf("unexpected bailian api: %#v", bailian["api"])
	}
	if len(bailian["models"].([]any)) != 2 {
		t.Fatalf("expected 2 bailian models, got %#v", bailian["models"])
	}

	agents := managed["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	model := defaults["model"].(map[string]any)
	if model["primary"] != "bailian/qwen3.5-plus" {
		t.Fatalf("unexpected primary model: %#v", model["primary"])
	}

	agentModels := defaults["models"].(map[string]any)
	if _, ok := agentModels["bailian/qwen3-coder-plus"]; !ok {
		t.Fatal("expected bailian/qwen3-coder-plus to be available in agents.defaults.models")
	}

	channels := managed["channels"].(map[string]any)
	if len(channels) != 0 {
		t.Fatalf("plugin-backed QQ should not be written into channels map, got %#v", channels)
	}
}
