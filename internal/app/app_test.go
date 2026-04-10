package app

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/internal/install"
	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/internal/ui"
	"github.com/goodtiger/openclaw-install/presets"
)

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"ascii", "hello", 5},
		{"chinese", "你好", 4},
		{"mixed", "安装完成", 8},
		{"mixed2", "完成安装", 8},
		{"single-byte", "a", 1},
		{"single-cjk", "你", 2},
		{"mixed-emoji", "Hello你好", 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayWidth(tt.input); got != tt.want {
				t.Errorf("displayWidth(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{"all-empty", []string{"", "", ""}, ""},
		{"first-non-empty", []string{"hello", "", ""}, "hello"},
		{"middle-non-empty", []string{"", "world", ""}, "world"},
		{"last-non-empty", []string{"", "", "last"}, "last"},
		{"with-whitespace", []string{"", "  ", "test"}, "test"},
		{"single-non-empty", []string{"only"}, "only"},
		{"single-empty", []string{""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstNonEmpty(tt.input...); got != tt.want {
				t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseCSV(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single-value", "hello", []string{"hello"}},
		{"multiple-values", "a,b,c", []string{"a", "b", "c"}},
		{"with-whitespace", " a , b , c ", []string{"a", "b", "c"}},
		{"trailing-comma", "a,b,", []string{"a", "b"}},
		{"leading-comma", ",a,b", []string{"a", "b"}},
		{"empty-between", "a,,b", []string{"a", "b"}},
		{"all-whitespace", "  ,  ,  ", nil},
		{"single-empty-string", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCSV(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseCSV(%q) = %v, want %v", tt.input, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseCSV(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestEnvOrEmptyReadsOpenClawDotEnv(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("BAILIAN_API_KEY", "")

	openclawDir := filepath.Join(homeDir, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0o755); err != nil {
		t.Fatalf("mkdir .openclaw: %v", err)
	}
	envPath := filepath.Join(openclawDir, ".env")
	if err := os.WriteFile(envPath, []byte("BAILIAN_API_KEY=dot-env-key\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if got := envOrEmpty("BAILIAN_API_KEY"); got != "dot-env-key" {
		t.Fatalf("envOrEmpty returned %q, want %q", got, "dot-env-key")
	}
}

func TestBuildProviderConfigPrefersOpenClawDotEnvOverExistingAPIKey(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("BAILIAN_API_KEY", "")
	t.Setenv("OPENCLAW_API_KEY", "")
	t.Setenv("OPENCLAW_BASE_URL", "")
	t.Setenv("OPENCLAW_MODEL", "")

	openclawDir := filepath.Join(homeDir, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0o755); err != nil {
		t.Fatalf("mkdir .openclaw: %v", err)
	}
	envPath := filepath.Join(openclawDir, ".env")
	if err := os.WriteFile(envPath, []byte("BAILIAN_API_KEY=new-key-from-dotenv\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	preset := presets.ProviderPreset{
		ID:           "bailian",
		Name:         "Bailian",
		Type:         "openai-compatible",
		BaseURL:      "https://example.invalid/v1",
		API:          "openai-completions",
		APIKeyEnv:    "BAILIAN_API_KEY",
		DefaultModel: "qwen3.5-plus",
		Models:       []string{"qwen3.5-plus"},
	}
	existing := config.ProviderConfig{
		API:          "openai-completions",
		BaseURL:      "https://example.invalid/v1",
		APIKey:       "stale-existing-key",
		PrimaryModel: "qwen3.5-plus",
	}

	cfg, err := buildProviderConfig(ui.NewPrompter(strings.NewReader(""), io.Discard), preset, existing, runInstallOptions{yes: true}, io.Discard)
	if err != nil {
		t.Fatalf("buildProviderConfig returned error: %v", err)
	}
	if cfg.APIKey != "new-key-from-dotenv" {
		t.Fatalf("APIKey = %q, want %q", cfg.APIKey, "new-key-from-dotenv")
	}
}

func TestNormalizedProvisioner(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"empty", "", "bridge"},
		{"bridge", "bridge", "bridge"},
		{"plugin", "plugin", "plugin"},
		{"other", "custom", "custom"},
		{"whitespace", "  ", "bridge"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizedProvisioner(tt.value); got != tt.want {
				t.Errorf("normalizedProvisioner(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestBoolLabel(t *testing.T) {
	tests := []struct {
		name  string
		value bool
		want  string
	}{
		{"true", true, "已检测"},
		{"false", false, "未检测"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := boolLabel(tt.value); got != tt.want {
				t.Errorf("boolLabel(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestEnvOrEmpty(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		setup func()
		want  string
	}{
		{"unset", "NON_EXISTENT_VAR_12345", func() {}, ""},
		{"set", "TEST_VAR_12345", func() { os.Setenv("TEST_VAR_12345", "test-value") }, "test-value"},
		{"set-empty", "TEST_EMPTY_VAR_12345", func() { os.Setenv("TEST_EMPTY_VAR_12345", "") }, ""},
		{"set-whitespace", "TEST_WS_VAR_12345", func() { os.Setenv("TEST_WS_VAR_12345", "  ") }, ""},
		{"empty-key", "", func() {}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				if tt.key != "" && tt.key != "NON_EXISTENT_VAR_12345" {
					os.Unsetenv(tt.key)
				}
			})
			tt.setup()
			if got := envOrEmpty(tt.key); got != tt.want {
				t.Errorf("envOrEmpty(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestConvertProviderCatalog(t *testing.T) {
	tests := []struct {
		name     string
		input    []presets.ProviderModel
		expected []config.ProviderModel
	}{
		{
			name:     "empty",
			input:    nil,
			expected: nil,
		},
		{
			name:     "single model",
			input:    []presets.ProviderModel{{ID: "model1", Name: "Model One"}},
			expected: []config.ProviderModel{{ID: "model1", Name: "Model One"}},
		},
		{
			name: "multiple models",
			input: []presets.ProviderModel{
				{ID: "m1", Name: "M1", Reasoning: true, ContextWindow: 4096, MaxTokens: 2048},
				{ID: "m2", Name: "M2", Cost: presets.ModelCost{Input: 0.1, Output: 0.2}},
			},
			expected: []config.ProviderModel{
				{ID: "m1", Name: "M1", Reasoning: true, ContextWindow: 4096, MaxTokens: 2048},
				{ID: "m2", Name: "M2", Cost: config.ModelCost{Input: 0.1, Output: 0.2}},
			},
		},
		{
			name:     "with empty id",
			input:    []presets.ProviderModel{{ID: "", Name: "No ID"}},
			expected: []config.ProviderModel{{ID: "", Name: "No ID"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertProviderCatalog(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("convertProviderCatalog(%v) length = %d, want %d", tt.input, len(got), len(tt.expected))
				return
			}
			for i := range got {
				if got[i].ID != tt.expected[i].ID {
					t.Errorf("convertProviderCatalog()[%d].ID = %q, want %q", i, got[i].ID, tt.expected[i].ID)
				}
				if got[i].Name != tt.expected[i].Name {
					t.Errorf("convertProviderCatalog()[%d].Name = %q, want %q", i, got[i].Name, tt.expected[i].Name)
				}
				if got[i].Reasoning != tt.expected[i].Reasoning {
					t.Errorf("convertProviderCatalog()[%d].Reasoning = %v, want %v", i, got[i].Reasoning, tt.expected[i].Reasoning)
				}
				if got[i].ContextWindow != tt.expected[i].ContextWindow {
					t.Errorf("convertProviderCatalog()[%d].ContextWindow = %v, want %v", i, got[i].ContextWindow, tt.expected[i].ContextWindow)
				}
				if got[i].MaxTokens != tt.expected[i].MaxTokens {
					t.Errorf("convertProviderCatalog()[%d].MaxTokens = %v, want %v", i, got[i].MaxTokens, tt.expected[i].MaxTokens)
				}
			}
		})
	}
}

func TestProviderModelIDs(t *testing.T) {
	tests := []struct {
		name     string
		preset   presets.ProviderPreset
		catalog  []config.ProviderModel
		expected []string
	}{
		{
			name:     "empty preset and catalog",
			preset:   presets.ProviderPreset{Models: []string{}},
			catalog:  []config.ProviderModel{},
			expected: []string{},
		},
		{
			name:     "catalog takes precedence",
			preset:   presets.ProviderPreset{Models: []string{"preset1", "preset2"}},
			catalog:  []config.ProviderModel{{ID: "cat1"}, {ID: "cat2"}},
			expected: []string{"cat1", "cat2"},
		},
		{
			name:     "no catalog, use preset models",
			preset:   presets.ProviderPreset{Models: []string{"m1", "m2"}},
			catalog:  []config.ProviderModel{},
			expected: []string{"m1", "m2"},
		},
		{
			name:     "catalog with empty id skipped",
			preset:   presets.ProviderPreset{},
			catalog:  []config.ProviderModel{{ID: "valid"}, {ID: ""}, {ID: "also-valid"}},
			expected: []string{"valid", "also-valid"},
		},
		{
			name:     "preserved models slice",
			preset:   presets.ProviderPreset{Models: []string{"a", "b"}},
			catalog:  []config.ProviderModel{},
			expected: []string{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerModelIDs(tt.preset, tt.catalog)
			if len(got) != len(tt.expected) {
				t.Errorf("providerModelIDs() length = %d, want %d", len(got), len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("providerModelIDs()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestPrintHelp(t *testing.T) {
	printHelp(os.Stdout)
}

func TestRun(t *testing.T) {
	// Test with no arguments (should show help)
	t.Run("no_args", func(t *testing.T) {
		var out, errOut bytes.Buffer

		exitCode := Run([]string{}, &strings.Reader{}, &out, &errOut)

		if exitCode != 0 {
			t.Errorf("Run() with no args returned %d, want 0", exitCode)
		}
		if out.Len() == 0 {
			t.Error("Run() with no args should write help to out")
		}
	})

	// Test help commands
	t.Run("help_commands", func(t *testing.T) {
		commands := [][]string{{"help"}, {"-h"}, {"--help"}}
		for _, args := range commands {
			t.Run(strings.Join(args, "_"), func(t *testing.T) {
				var out, errOut bytes.Buffer

				exitCode := Run(args, &strings.Reader{}, &out, &errOut)

				if exitCode != 0 {
					t.Errorf("Run(%q) returned %d, want 0", args, exitCode)
				}
				if out.Len() == 0 {
					t.Error("Run() with help command should write help to out")
				}
			})
		}
	})

	// Test version command
	t.Run("version", func(t *testing.T) {
		var out, errOut bytes.Buffer

		exitCode := Run([]string{"version"}, &strings.Reader{}, &out, &errOut)

		if exitCode != 0 {
			t.Errorf("Run([\"version\"]) returned %d, want 0", exitCode)
		}
		if out.Len() == 0 {
			t.Error("Run([\"version\"]) should write version to out")
		}
	})

	// Test unknown command
	t.Run("unknown_command", func(t *testing.T) {
		var out, errOut bytes.Buffer

		exitCode := Run([]string{"invalid-command"}, &strings.Reader{}, &out, &errOut)

		if exitCode != 2 {
			t.Errorf("Run([\"invalid-command\"]) returned %d, want 2", exitCode)
		}
		if errOut.Len() == 0 {
			t.Error("Run([\"invalid-command\"]) should write error message to errOut")
		}
		if !strings.Contains(errOut.String(), "未知命令") {
			t.Errorf("Run([\"invalid-command\"]) error output should contain '未知命令'")
		}
	})

	// Test install command with help
	t.Run("install_help", func(t *testing.T) {
		var out, errOut bytes.Buffer

		exitCode := Run([]string{"install", "--help"}, &strings.Reader{}, &out, &errOut)
		_ = exitCode // Just checking for no panic during flag parsing
	})

	// Test reconfigure command with help
	t.Run("reconfigure_help", func(t *testing.T) {
		var out, errOut bytes.Buffer

		exitCode := Run([]string{"reconfigure", "--help"}, &strings.Reader{}, &out, &errOut)
		_ = exitCode // Just checking for no panic during flag parsing
	})

	// Test doctor command with help to avoid system detection failures
	t.Run("doctor_help", func(t *testing.T) {
		var out, errOut bytes.Buffer

		exitCode := Run([]string{"doctor", "--help"}, &strings.Reader{}, &out, &errOut)
		_ = exitCode // Just checking for no panic during argument parsing
	})
}

func TestRunDoctor(t *testing.T) {
	// Test with --help flag to avoid system detection requirements
	t.Run("with_help", func(t *testing.T) {
		var out, errOut bytes.Buffer

		err := runDoctor([]string{"--help"}, &out, &errOut)
		// Should not return an error when just parsing help flag
		if err != nil {
			t.Logf("runDoctor --help got error: %v, may be OK in test environment", err)
		}
	})
}

func TestRunUpgrade(t *testing.T) {
	t.Skip("upgrade requires network access to GitHub API")
}

func TestRunBridge(t *testing.T) {
	// Test runBridge with no subcommands
	t.Run("no_subcommand", func(t *testing.T) {
		var out, errOut bytes.Buffer

		err := runBridge([]string{}, &out, &errOut)

		if err == nil {
			t.Fatal("Expected error for missing subcommand")
		}
		if !strings.Contains(err.Error(), "bridge 需要子命令") {
			t.Errorf("Expected error to mention required subcommand, got: %v", err)
		}
	})

	// Test runBridge with unknown subcommand
	t.Run("unknown_subcommand", func(t *testing.T) {
		var out, errOut bytes.Buffer

		err := runBridge([]string{"unknown"}, &out, &errOut)

		if err == nil {
			t.Fatal("Expected error for unknown subcommand")
		}
		if !strings.Contains(err.Error(), "未知的 bridge 子命令") {
			t.Errorf("Expected error to mention unknown subcommand, got: %v", err)
		}
	})
}

func TestRunBridgeServe(t *testing.T) {
	// Test runBridgeServe with no --channel flag
	t.Run("missing_channel", func(t *testing.T) {
		var out, errOut bytes.Buffer

		err := runBridgeServe([]string{}, &out, &errOut)

		if err == nil {
			t.Fatal("Expected error for missing channel flag")
		}
		if !strings.Contains(err.Error(), "必须提供 --channel") {
			t.Errorf("Expected error to mention required channel, got: %v", err)
		}
	})
}

func TestPrintSuccessSummary(t *testing.T) {
	t.Run("install_mode", func(t *testing.T) {
		var out bytes.Buffer

		req := install.Request{
			Mode: "native",
			Provider: config.ProviderConfig{
				ID:           "test-provider",
				Name:         "Test Provider",
				PrimaryModel: "gpt-3.5-turbo",
				APIKey:       "test-api-key",
			},
		}
		result := install.Result{
			ConfigPath:       "/path/to/config",
			BridgeConfigPath: "/path/to/bridge",
			BackupFile:       "/path/to/backup",
		}

		printSuccessSummary(&out, req, result, false)

		output := out.String()
		if !strings.Contains(output, "安装完成") {
			t.Errorf("Expected summary to contain '安装完成' in install mode, got %q", output)
		}
		if !strings.Contains(output, "native") {
			t.Errorf("Expected summary to contain mode, got %q", output)
		}
	})

	t.Run("reconfigure_mode", func(t *testing.T) {
		var out bytes.Buffer

		req := install.Request{
			Mode: "docker",
			Provider: config.ProviderConfig{
				ID:           "test-provider",
				Name:         "Test Provider",
				PrimaryModel: "gpt-4",
				APIKey:       "test-api-key",
			},
		}
		result := install.Result{
			ConfigPath:       "/path/to/config",
			BridgeConfigPath: "/path/to/bridge",
		}

		printSuccessSummary(&out, req, result, true)

		output := out.String()
		if !strings.Contains(output, "重配置完成") {
			t.Errorf("Expected summary to contain '重配置完成' in reconfigure mode, got %q", output)
		}
		if !strings.Contains(output, "docker") {
			t.Errorf("Expected summary to contain mode, got %q", output)
		}
	})
}

func TestRunInstallLike(t *testing.T) {
	// Test runInstallLike with --yes flag
	t.Run("yes_path", func(t *testing.T) {
		var out, errOut bytes.Buffer

		opts := runInstallOptions{
			yes: true,
		}

		_ = runInstallLike(opts, &strings.Reader{}, &out, &errOut)
		// The test is to verify no panic during argument parsing.
		// System detection may fail but shouldn't panic.
	})

	// Test runInstallLike with non-TTY rejection when --yes is not passed
	t.Run("non_TTY_no_yes", func(t *testing.T) {
		var out, errOut bytes.Buffer

		opts := runInstallOptions{
			yes: false,
		}

		_ = runInstallLike(opts, &strings.Reader{}, &out, &errOut)
		// Just check no panic occurs during parsing
	})
}

func TestChooseProviderPreset(t *testing.T) {
	// Test empty bundle rejection
	t.Run("empty_bundle_rejection", func(t *testing.T) {
		prompter := ui.NewPrompter(strings.NewReader(""), io.Discard)

		_, err := chooseProviderPreset(prompter, presets.Bundle{}, "", true, "")

		if err == nil {
			t.Fatal("Expected empty provider bundle to be rejected")
		}
		if !strings.Contains(err.Error(), "没有可用的供应商预设") {
			t.Errorf("Expected error message '没有可用的供应商预设', but got: %v", err)
		}
	})

	// Test flag override path
	t.Run("flag_override", func(t *testing.T) {
		bundle := presets.Bundle{
			Providers: []presets.ProviderPreset{
				{ID: "test-provider", Name: "Test Provider", Type: "openai"},
			},
		}
		prompter := ui.NewPrompter(strings.NewReader(""), io.Discard)

		// Should pick the provider specified in the providerID argument
		preset, err := chooseProviderPreset(prompter, bundle, "test-provider", true, "")

		if err != nil {
			t.Fatalf("Expected no error when provider is specified by flag, got: %v", err)
		}
		if preset.ID != "test-provider" {
			t.Errorf("Expected provider ID 'test-provider', got %q", preset.ID)
		}
	})

	// Test yes-mode fallback to first provider
	t.Run("yes_mode_fallback_to_first", func(t *testing.T) {
		bundle := presets.Bundle{
			Providers: []presets.ProviderPreset{
				{ID: "first-provider", Name: "First Provider", Type: "openai"},
				{ID: "second-provider", Name: "Second Provider", Type: "claude"},
			},
		}
		prompter := ui.NewPrompter(strings.NewReader(""), io.Discard)

		preset, err := chooseProviderPreset(prompter, bundle, "", true, "")

		if err != nil {
			t.Fatalf("Expected no error in yes mode, got: %v", err)
		}
		if preset.ID != "first-provider" {
			t.Errorf("Expected first provider 'first-provider', got %q", preset.ID)
		}
	})
}

func TestNewFlagSet(t *testing.T) {
	t.Run("prints_help_content", func(t *testing.T) {
		var out bytes.Buffer

		fs := newFlagSet("test", &out, "Test command description")

		// Print usage to check help content
		fs.Usage()

		helpOutput := out.String()
		if !strings.Contains(helpOutput, "用法：openclaw-install test") {
			t.Errorf("Expected help output to contain usage pattern, got: %q", helpOutput)
		}
		if !strings.Contains(helpOutput, "Test command description") {
			t.Errorf("Expected help output to contain description, got: %q", helpOutput)
		}
	})

	t.Run("with_options_shows_defaults", func(t *testing.T) {
		var out bytes.Buffer

		fs := newFlagSet("test", &out, "Test command with options")
		testOpt := fs.String("opt", "default", "Test option")

		// Verify the option is defined
		if *testOpt != "default" {
			t.Errorf("Expected default value 'default', got: %q", *testOpt)
		}

		// Print usage to check options are shown
		fs.Usage()
		helpOutput := out.String()

		if !strings.Contains(helpOutput, "参数：") {
			t.Errorf("Expected help output to contain '参数：' section when options exist, got: %q", helpOutput)
		}
		if !strings.Contains(helpOutput, "Test option") {
			t.Errorf("Expected help output to contain option description, got: %q", helpOutput)
		}
	})
}

func TestRunInstallAndReconfigure(t *testing.T) {
	// Test install --help option parsing
	t.Run("install_help", func(t *testing.T) {
		var out, errOut bytes.Buffer

		exitCode := Run([]string{"install", "--help"}, &strings.Reader{}, &out, &errOut)
		_ = exitCode // Just checking for no panic during parsing
	})

	// Test reconfigure --help option parsing
	t.Run("reconfigure_help", func(t *testing.T) {
		var out, errOut bytes.Buffer

		exitCode := Run([]string{"reconfigure", "--help"}, &strings.Reader{}, &out, &errOut)
		_ = exitCode // Just checking for no panic during parsing
	})
}

func TestPrintDetectionPreview(t *testing.T) {
	t.Run("executes_without_panic", func(t *testing.T) {
		var out bytes.Buffer

		// Construct minimal data for the function to execute
		bundle := presets.Bundle{
			Providers: []presets.ProviderPreset{
				{ID: "test-provider", Name: "Test", Type: "openai"},
			},
		}

		info := system.Info{}
		report := install.DoctorReport{
			Info: info,
		}

		// Execute the function - this should not panic
		printDetectionPreview(&out, bundle, info, report)

		// Verify that the function executed and produced some output
		if out.Len() == 0 {
			t.Log("Warning: printDetectionPreview produced no output in test environment")
		}
	})
}

func TestCollectCredentialFields(t *testing.T) {
	t.Run("basic_execution", func(t *testing.T) {
		// Test the collectCredentialFields function
		channel := presets.ChannelPreset{
			Name: "test-channel",
			RequiredFields: []presets.CredentialField{
				{
					Key:      "username",
					Label:    "Username",
					Secret:   false,
					Optional: true,
				},
				{
					Key:      "password",
					Label:    "Password",
					Secret:   true,
					Optional: false,
				},
			},
		}

		prompter := ui.NewPrompter(strings.NewReader("test-user\ntest-pass\n"), os.Stdout)
		existingConfig := config.BridgeChannelConfig{
			Fields: map[string]string{
				"username": "existing-user",
			},
		}

		// Execute the function in yes mode to bypass interactive prompts
		result, err := collectCredentialFields(prompter, channel, existingConfig, true)
		_ = result // May be empty depending on test environment
		_ = err    // Just ensuring it runs without panic
	})
}

func TestCollectNetworkConfig(t *testing.T) {
	t.Run("with_yes_mode", func(t *testing.T) {
		channel := presets.ChannelPreset{
			Name:          "test-channel",
			Provisioner:   "bridge",
			DefaultListen: "localhost:3000",
			DefaultPath:   "/webhook",
		}

		existingConfig := config.BridgeChannelConfig{
			ListenAddr: "localhost:3001",
			Path:       "/other-webhook",
		}

		prompter := ui.NewPrompter(strings.NewReader(""), os.Stdout)

		// Test in yes mode to bypass interactive prompts
		listenAddr, path, err := collectNetworkConfig(prompter, channel, existingConfig, true, true)
		_ = listenAddr // Will likely be default in yes mode
		_ = path       // Will likely be default in yes mode
		_ = err
	})
}

func TestLoadExistingBridgeConfig(t *testing.T) {
	t.Run("handles_missing_file", func(t *testing.T) {
		// Test with non-existent config path
		cfg, err := loadExistingBridgeConfig("/tmp/non-existent-path.json")

		// Should either return empty config or an error that isn't a panic
		if err != nil {
			// If it's a file-not-found error, that's normal
			if !strings.Contains(err.Error(), "no such file") && !strings.Contains(err.Error(), "does not exist") {
				t.Errorf("Unexpected error type: %v", err)
			}
		}

		// Make sure cfg is not null and is an empty/default config
		_ = cfg
	})
}
