package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestApplyManagedConfigPreservesUnknownAndReplacesManagedEntries(t *testing.T) {
	existing := map[string]any{
		"custom": "keep-me",
		"meta": map[string]any{
			"installer": map[string]any{
				"name":    "openclaw-install",
				"version": "0.1.0",
			},
		},
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
		GatewayBind:      "loopback",
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

	if meta, ok := merged["meta"].(map[string]any); ok {
		if _, ok := meta["installer"]; ok {
			t.Fatal("expected legacy meta.installer to be removed from merged config")
		}
	}
}

func TestBuildManagedConfigBailianDefaultsAndSkipsPluginChannels(t *testing.T) {
	input := ManagedConfigInput{
		InstallerVersion: "0.1.0",
		Mode:             "native",
		GatewayBind:      "loopback",
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

	gateway := managed["gateway"].(map[string]any)
	if gateway["bind"] != "loopback" {
		t.Fatalf("unexpected gateway bind: %#v", gateway["bind"])
	}

	agentModels := defaults["models"].(map[string]any)
	if _, ok := agentModels["bailian/qwen3-coder-plus"]; !ok {
		t.Fatal("expected bailian/qwen3-coder-plus to be available in agents.defaults.models")
	}

	if meta, ok := managed["meta"]; ok {
		t.Fatalf("unexpected meta block in managed config: %#v", meta)
	}

	channels := managed["channels"].(map[string]any)
	if len(channels) != 0 {
		t.Fatalf("plugin-backed QQ should not be written into channels map, got %#v", channels)
	}
}

func TestSaveJSONAtomicUsesUniqueTempFileAndCleansUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	if err := SaveJSONAtomic(path, map[string]any{"ok": true}); err != nil {
		t.Fatalf("SaveJSONAtomic() error = %v", err)
	}

	matches, err := filepath.Glob(path + ".tmp-*")
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected temp files to be cleaned up, got %#v", matches)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected target file to exist, stat err = %v", err)
	}
}

func TestBuildManagedConfigWeChatPluginWithLoginRequired(t *testing.T) {
	input := ManagedConfigInput{
		InstallerVersion: "0.1.0",
		Mode:             "native",
		GatewayBind:      "loopback",
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
				ID:              "wechat",
				Name:            "微信（个人微信 ClawBot 插件）",
				Driver:          "wechat",
				Provisioner:     "openclaw-plugin",
				PluginPackage:   "@tencent-weixin/openclaw-weixin@latest",
				OpenClawChannel: "openclaw-weixin",
				LoginRequired:   true,
				TokenFields:     []string{},
				Fields:          map[string]string{},
			},
		},
	}

	managed := BuildManagedConfig(input)

	channels := managed["channels"].(map[string]any)
	if len(channels) != 0 {
		t.Fatalf("plugin-backed WeChat should not be written into channels map, got %#v", channels)
	}

	plugins, ok := managed["plugins"].(map[string]any)
	if !ok {
		t.Fatal("expected plugins block to exist in managed config")
	}

	allow, ok := plugins["allow"].([]any)
	if !ok {
		t.Fatal("expected plugins.allow to be a list")
	}

	found := false
	for _, item := range allow {
		if item == "openclaw-weixin" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected plugins.allow to contain 'openclaw-weixin'")
	}
}

func TestBackupIfExists(t *testing.T) {
	tests := []struct {
		name           string
		fileExists     bool
		setupState     func(dir string) string // returns path to try backing up
		expectErr      bool
		expectedResult string
	}{
		{
			name:       "file not exists",
			fileExists: false,
			setupState: func(dir string) string {
				return filepath.Join(dir, "nonexistent.json")
			},
			expectErr:      false,
			expectedResult: "",
		},
		{
			name:       "file exists",
			fileExists: true,
			setupState: func(dir string) string {
				path := filepath.Join(dir, "config.json")
				err := os.WriteFile(path, []byte(`{"key":"value"}`), 0644)
				if err != nil {
					t.Fatal(err)
				}
				return path
			},
			expectErr:      false,
			expectedResult: "non-empty", // Will check if non-empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := tt.setupState(dir)
			
			backupDir := filepath.Join(dir, "backups")
			backup, err := BackupIfExists(path, backupDir, time.Now())

			if tt.expectErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			
			// Check the result against expectation
			if tt.expectedResult == "" {
				if backup != "" {
					t.Fatalf("expected empty backup path, got %q", backup)
				}
			} else if tt.expectedResult == "non-empty" {
				if backup == "" {
					t.Fatal("expected non-empty backup path, got empty")
				}
				
				// Verify backup content matches original 
				originalContent, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatal(readErr)
				}
				
				backupContent, readErr := os.ReadFile(backup)
				if readErr != nil {
					t.Fatal(readErr)
				}
				
				if string(originalContent) != string(backupContent) {
					t.Fatalf("backup content differs from original: %s vs %s", string(originalContent), string(backupContent))
				}
			}
		})
	}
	
	// Additional test for stat error case - create a protected directory with no permissions
	t.Run("stat error", func(t *testing.T) {
		// Create the main directory
		dir := t.TempDir()
		
		// Create a protected child directory with no permissions
		protectedDir := filepath.Join(dir, "protected")
		err := os.Mkdir(protectedDir, 0000) // No permissions - read/writable by user only
		if err != nil {
			t.Fatal(err)
		}
		
		// Try to operate on a file inside the protected directory
		protectedFilePath := filepath.Join(protectedDir, "config.json") 
		backupDir := filepath.Join(dir, "backups")
		
		// Try to back up this file - should fail at Stat
		_, err = BackupIfExists(protectedFilePath, backupDir, time.Now())
		
		if err == nil {
			t.Fatal("expected error for stat but got none")
		}
	})
}

func TestLoadMap(t *testing.T) {
	tests := []struct {
		name            string
		setupFile       func(dir string) string
		expectErr       bool
		expectedMap     map[string]any
	}{
		{
			name: "file doesn't exist",
			setupFile: func(dir string) string {
				return filepath.Join(dir, "nonexistent.json")
			},
			expectErr: false,
			expectedMap: map[string]any{},
		},
		{
			name: "empty file",
			setupFile: func(dir string) string {
				path := filepath.Join(dir, "empty.json")
				err := os.WriteFile(path, []byte(""), 0644)
				if err != nil {
					t.Fatal(err)
				}
				return path
			},
			expectErr: false,
			expectedMap: map[string]any{},
		},
		{
			name: "whitespace file",
			setupFile: func(dir string) string {
				path := filepath.Join(dir, "whitespace.json")
				err := os.WriteFile(path, []byte("   \n\t  "), 0644)
				if err != nil {
					t.Fatal(err)
				}
				return path
			},
			expectErr: false,
			expectedMap: map[string]any{},
		},
		{
			name: "valid JSON",
			setupFile: func(dir string) string {
				path := filepath.Join(dir, "valid.json")
				err := os.WriteFile(path, []byte(`{"name": "test", "count": 42}`), 0644)
				if err != nil {
					t.Fatal(err)
				}
				return path
			},
			expectErr: false,
			expectedMap: map[string]any{
				"name":  "test",
				"count": float64(42),
			},
		},
		{
			name: "invalid JSON",
			setupFile: func(dir string) string {
				path := filepath.Join(dir, "invalid.json")
				err := os.WriteFile(path, []byte(`{invalid json`), 0644)
				if err != nil {
					t.Fatal(err)
				}
				return path
			},
			expectErr: true,
			expectedMap: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := tt.setupFile(dir)
			
			resultMap, err := LoadMap(path)
			
			if tt.expectErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			
			if !reflect.DeepEqual(resultMap, tt.expectedMap) {
				t.Fatalf("expected map %+v, got %+v", tt.expectedMap, resultMap)
			}
		})
	}
}

func TestLoadBridgeConfig(t *testing.T) {
	tests := []struct {
		name          string
		setupFile     func(dir string) string
		expectErr     bool
		validator     func(cfg BridgeConfig) bool
	}{
		{
			name: "file doesn't exist",
			setupFile: func(dir string) string {
				return filepath.Join(dir, "nonexistent.json")
			},
			expectErr: true,
			validator: nil,
		},
		{
			name: "valid JSON",
			setupFile: func(dir string) string {
				cfg := BridgeConfig{
					Version:        1,
					SystemPrompt:   "You are an OpenClaw channel assistant. Reply clearly and briefly in Chinese unless the user asks otherwise.",
					TimeoutSeconds: 30,
					Provider: ProviderConfig{
						ID:           "test",
						Name:         "Test Provider",
						Type:         "openai-compatible",
						BaseURL:      "https://api.test.com",
						APIKey:       "secret-key",
						PrimaryModel: "gpt-4",
					},
					Channels: map[string]BridgeChannelConfig{
						"test-channel": {
							Enabled:     true,
							Driver:      "webhook",
							Provisioner: "bridge",
							ListenAddr:  "127.0.0.1:8080",
							Path:        "/webhook",
							Fields: map[string]string{
								"key": "value",
							},
						},
					},
				}
				content, err := json.Marshal(cfg)
				if err != nil {
					t.Fatal(err)
				}
				
				path := filepath.Join(dir, "valid.json")
				err = os.WriteFile(path, content, 0644)
				if err != nil {
					t.Fatal(err)
				}
				return path
			},
			expectErr: false,
			validator: func(cfg BridgeConfig) bool {
				return cfg.Version == 1 &&
				       cfg.SystemPrompt != "" && 
				       cfg.TimeoutSeconds == 30 &&
				       cfg.Provider.ID == "test" &&
				       len(cfg.Channels) == 1
			},
		},
		{
			name: "invalid JSON",
			setupFile: func(dir string) string {
				path := filepath.Join(dir, "invalid.json")
				err := os.WriteFile(path, []byte(`{invalid json`), 0644)
				if err != nil {
					t.Fatal(err)
				}
				return path
			},
			expectErr: true,
			validator: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := tt.setupFile(dir)
			
			resultCfg, err := LoadBridgeConfig(path)
			
			if tt.expectErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			
			if !tt.validator(resultCfg) {
				t.Fatalf("unexpected config: %+v", resultCfg)
			}
		})
	}
}

func TestLoadInstallState(t *testing.T) {
	tests := []struct {
		name          string
		setupFile     func(dir string) string
		expectErr     bool
		validateResult func(state InstallState) bool
	}{
		{
			name: "file doesn't exist",
			setupFile: func(dir string) string {
				return filepath.Join(dir, "nonexistent.json")
			},
			expectErr: false,
			validateResult: func(state InstallState) bool {
				return reflect.DeepEqual(state, InstallState{})
			},
		},
		{
			name: "valid JSON",
			setupFile: func(dir string) string {
				state := InstallState{
					Version:           "1.0.0",
					InstalledAt:       time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
					Mode:              "native",
					Platform:          "linux",
					ManagedProviderID: "test-provider",
					ManagedChannels:   []string{"ch1", "ch2"},
				}
				content, err := json.Marshal(state)
				if err != nil {
					t.Fatal(err)
				}
				
				path := filepath.Join(dir, "valid.json")
				err = os.WriteFile(path, content, 0644)
				if err != nil {
					t.Fatal(err)
				}
				return path
			},
			expectErr: false,
			validateResult: func(state InstallState) bool {
				return state.Version == "1.0.0" &&
				       state.Mode == "native" &&
				       state.Platform == "linux" &&
				       state.ManagedProviderID == "test-provider" &&
				       reflect.DeepEqual(state.ManagedChannels, []string{"ch1", "ch2"})
			},
		},
		{
			name: "invalid JSON",
			setupFile: func(dir string) string {
				path := filepath.Join(dir, "invalid.json")
				err := os.WriteFile(path, []byte(`{invalid json`), 0644)
				if err != nil {
					t.Fatal(err)
				}
				return path
			},
			expectErr: true,
			validateResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := tt.setupFile(dir)
			
			resultState, err := LoadInstallState(path)
			
			if tt.expectErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			
			if !tt.validateResult(resultState) {
				t.Fatalf("loaded state does not match expected: expected validation failure")
			}
		})
	}
}

func TestSaveInstallState(t *testing.T) {
	t.Run("saves state to file correctly", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "install-state.json")
		
		state := InstallState{
			Version:           "1.0.0",
			InstalledAt:       time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			Mode:              "native",
			Platform:          "linux",
			ManagedProviderID: "test-provider",
			ManagedChannels:   []string{"ch1", "ch2"},
		}
		
		err := SaveInstallState(path, state)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Fatal("saved file does not exist")
		}
		
		loadedState, err := LoadInstallState(path)
		if err != nil {
			t.Fatalf("failed to load saved state: %v", err)
		}
		
		if loadedState.Version != state.Version ||
			loadedState.Mode != state.Mode ||
			loadedState.Platform != state.Platform ||
			loadedState.ManagedProviderID != state.ManagedProviderID ||
			!reflect.DeepEqual(loadedState.ManagedChannels, state.ManagedChannels) {
			t.Fatalf("loaded state does not match saved state: expected %+v, got %+v", state, loadedState)
		}
	})
}

func TestBuildBridgeConfig(t *testing.T) {
	t.Run("builds config from input with channels", func(t *testing.T) {
		input := ManagedConfigInput{
			Provider: ProviderConfig{
				ID:           "test",
				Name:         "Test Provider",
				Type:         "openai-compatible",
				BaseURL:      "https://api.test.com",
				APIKey:       "secret-key",
				PrimaryModel: "gpt-4",
			},
			Channels: []ChannelSelection{
				{
					ID:         "test-channel",
					Name:       "Test Channel",
					Driver:     "webhook",
					Provisioner: "bridge",
					ListenAddr: "127.0.0.1:8080",
					Path:       "/webhook",
					Fields: map[string]string{
						"key": "value",
					},
				},
			},
		}
		
		result := BuildBridgeConfig(input)
		
		if result.Version != 1 {
			t.Errorf("expected version 1, got %d", result.Version)
		}
		
		if result.SystemPrompt == "" {
			t.Error("expected system prompt to be set")
		}
		
		if result.TimeoutSeconds != 30 {
			t.Errorf("expected timeout 30, got %d", result.TimeoutSeconds)
		}
		
		if result.Provider.ID != "test" {
			t.Errorf("expected provider ID 'test', got '%s'", result.Provider.ID)
		}
		
		if len(result.Channels) != 1 {
			t.Fatalf("expected 1 channel, got %d", len(result.Channels))
		}
		
		channel, exists := result.Channels["test-channel"]
		if !exists {
			t.Fatal("expected test-channel to exist in bridge config")
		}
		
		if !channel.Enabled {
			t.Error("expected channel to be enabled")
		}
		
		if channel.Driver != "webhook" {
			t.Errorf("expected driver 'webhook', got '%s'", channel.Driver)
		}
		
		if channel.Provisioner != "bridge" {
			t.Errorf("expected provisioner 'bridge', got '%s'", channel.Provisioner)
		}
		
		if channel.ListenAddr != "127.0.0.1:8080" {
			t.Errorf("expected listen address '127.0.0.1:8080', got '%s'", channel.ListenAddr)
		}
		
		if channel.Path != "/webhook" {
			t.Errorf("expected path '/webhook', got '%s'", channel.Path)
		}
		
		if len(channel.Fields) != 1 {
			t.Errorf("expected 1 field, got %d", len(channel.Fields))
		}
		
		if value, exists := channel.Fields["key"]; !exists || value != "value" {
			t.Errorf("expected field 'key' to have value 'value', got '%s'", value)
		}
	})
}

func TestBridgeURL(t *testing.T) {
	tests := []struct {
		name         string
		bridgeHost   string
		listenAddr   string
		path         string
		expectedURL  string
	}{
		{
			name:        "valid host:port with path",
			bridgeHost:  "",
			listenAddr:  "localhost:8080",
			path:        "/webhook",
			expectedURL: "http://localhost:8080/webhook",
		},
		{
			name:        "valid host:port without path",
			bridgeHost:  "",
			listenAddr:  "localhost:8080",
			path:        "",
			expectedURL: "http://localhost:8080",
		},
		{
			name:        "no port fallback",
			bridgeHost:  "",
			listenAddr:  "localhost",
			path:        "",
			expectedURL: "http://localhost",
		},
		{
			name:        "no port with path fallback",
			bridgeHost:  "",
			listenAddr:  "localhost",
			path:        "/webhook",
			expectedURL: "http://localhost/webhook",
		},
		{
			name:        "with bridge host override",
			bridgeHost:  "127.0.0.1",
			listenAddr:  "localhost:8080",
			path:        "/webhook",
			expectedURL: "http://127.0.0.1:8080/webhook",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bridgeURL(tt.bridgeHost, tt.listenAddr, tt.path)
			
			if result != tt.expectedURL {
				t.Errorf("expected %s, got %s", tt.expectedURL, result)
			}
		})
	}
}

func TestJoinModelID(t *testing.T) {
	tests := []struct {
		name          string
		providerID    string
		model         string
		expected      string
	}{
		{
			name:       "normal case",
			providerID: "openai",
			model:      "gpt-4",
			expected:   "openai/gpt-4",
		},
		{
			name:       "empty model",
			providerID: "openai",
			model:      "",
			expected:   "",
		},
		{
			name:       "whitespace model",
			providerID: "openai",
			model:      "   ",
			expected:   "",
		},
		{
			name:       "trimmed model",
			providerID: "openai",
			model:      " gpt-4 ",
			expected:   "openai/gpt-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinModelID(tt.providerID, tt.model)
			
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestCloneStringMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]string
	}{
		{
			name:     "nil map",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: nil,
		},
		{
			name:     "populated map",
			input:    map[string]string{"key1": "value1", "key2": "value2"},
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cloneStringMap(tt.input)
			
			if tt.expected == nil { 
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if result == nil || !reflect.DeepEqual(result, tt.expected) {
					t.Errorf("expected %v, got %v", tt.expected, result)
				}
			}
		})
	}
}
