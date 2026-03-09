package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"

	"github.com/goodtiger/openclaw-install/internal/bridge"
	"github.com/goodtiger/openclaw-install/internal/config"
	"github.com/goodtiger/openclaw-install/internal/install"
	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/internal/ui"
	"github.com/goodtiger/openclaw-install/presets"
)

func Run(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		printHelp(out)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		printHelp(out)
		return 0
	case "version":
		fmt.Fprintln(out, Version)
		return 0
	case "doctor":
		if err := runDoctor(args[1:], out, errOut); err != nil {
			fmt.Fprintln(errOut, "doctor:", err)
			return 1
		}
		return 0
	case "install":
		if err := runInstall(args[1:], in, out, errOut); err != nil {
			fmt.Fprintln(errOut, "install:", err)
			return 1
		}
		return 0
	case "reconfigure":
		if err := runReconfigure(args[1:], in, out, errOut); err != nil {
			fmt.Fprintln(errOut, "reconfigure:", err)
			return 1
		}
		return 0
	case "bridge":
		if err := runBridge(args[1:], out, errOut); err != nil {
			fmt.Fprintln(errOut, "bridge:", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(errOut, "unknown command: %s\n\n", args[0])
		printHelp(errOut)
		return 2
	}
}

func runDoctor(args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return err
	}

	bundle, err := presets.Load()
	if err != nil {
		return err
	}

	info, err := system.Detect()
	if err != nil {
		return err
	}

	workflow := install.NewWorkflow(bundle, install.RealExecutor{})
	report, err := workflow.Doctor(context.Background(), info)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "System: %s/%s\n", report.Info.OS, report.Info.Arch)
	fmt.Fprintf(out, "OpenClaw home: %s\n", report.Info.OpenClawHome)
	fmt.Fprintf(out, "Config path: %s\n", report.Info.ConfigPath)
	fmt.Fprintf(out, "Package manager: %s\n", valueOrDefault(report.Info.PackageManager, "not detected"))
	fmt.Fprintf(out, "Recommended mode: %s\n", report.RecommendedMode)
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Detected tools:")
	fmt.Fprintf(out, "  docker: %t\n", report.Info.HasDocker)
	fmt.Fprintf(out, "  docker compose: %t\n", report.Info.HasCompose)
	fmt.Fprintf(out, "  node: %t\n", report.Info.HasNode)
	fmt.Fprintf(out, "  npm: %t\n", report.Info.HasNPM)
	fmt.Fprintf(out, "  openclaw: %t\n", report.Info.HasOpenClaw)
	fmt.Fprintf(out, "  git: %t\n", report.Info.HasGit)
	fmt.Fprintf(out, "  curl: %t\n", report.Info.HasCurl)
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Mirror selection:")
	keys := sortedKeys(report.MirrorNames)
	for _, key := range keys {
		fmt.Fprintf(out, "  %s: %s\n", key, report.MirrorNames[key])
	}
	if len(report.Warnings) > 0 {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "Warnings:")
		for _, warning := range report.Warnings {
			fmt.Fprintf(out, "  - %s\n", warning)
		}
	}
	return nil
}

func runInstall(args []string, in io.Reader, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(errOut)

	modeFlag := fs.String("mode", "", "install mode: docker or native")
	providerFlag := fs.String("provider", "", "provider preset id")
	baseURLFlag := fs.String("base-url", "", "provider base URL")
	apiKeyFlag := fs.String("api-key", "", "provider API key")
	primaryFlag := fs.String("primary-model", "", "primary model id")
	fallbackFlag := fs.String("fallback-models", "", "comma-separated fallback models")
	channelsFlag := fs.String("channels", "", "comma-separated channel ids")
	yesFlag := fs.Bool("yes", false, "accept defaults where possible")
	skipVerifyFlag := fs.Bool("skip-verify", false, "skip post-install verification")

	if err := fs.Parse(args); err != nil {
		return err
	}

	return runInstallLike(runInstallOptions{
		mode:         *modeFlag,
		providerID:   *providerFlag,
		baseURL:      *baseURLFlag,
		apiKey:       *apiKeyFlag,
		primaryModel: *primaryFlag,
		fallbacks:    *fallbackFlag,
		channels:     *channelsFlag,
		yes:          *yesFlag,
		skipVerify:   *skipVerifyFlag,
		reconfigure:  false,
	}, in, out, errOut)
}

func runReconfigure(args []string, in io.Reader, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("reconfigure", flag.ContinueOnError)
	fs.SetOutput(errOut)

	modeFlag := fs.String("mode", "", "installation mode to keep using")
	providerFlag := fs.String("provider", "", "provider preset id")
	baseURLFlag := fs.String("base-url", "", "provider base URL")
	apiKeyFlag := fs.String("api-key", "", "provider API key")
	primaryFlag := fs.String("primary-model", "", "primary model id")
	fallbackFlag := fs.String("fallback-models", "", "comma-separated fallback models")
	channelsFlag := fs.String("channels", "", "comma-separated channel ids")
	yesFlag := fs.Bool("yes", false, "accept defaults where possible")
	skipVerifyFlag := fs.Bool("skip-verify", false, "skip verification after rewrite")

	if err := fs.Parse(args); err != nil {
		return err
	}

	return runInstallLike(runInstallOptions{
		mode:         *modeFlag,
		providerID:   *providerFlag,
		baseURL:      *baseURLFlag,
		apiKey:       *apiKeyFlag,
		primaryModel: *primaryFlag,
		fallbacks:    *fallbackFlag,
		channels:     *channelsFlag,
		yes:          *yesFlag,
		skipVerify:   *skipVerifyFlag,
		reconfigure:  true,
	}, in, out, errOut)
}

type runInstallOptions struct {
	mode         string
	providerID   string
	baseURL      string
	apiKey       string
	primaryModel string
	fallbacks    string
	channels     string
	yes          bool
	skipVerify   bool
	reconfigure  bool
}

func runInstallLike(options runInstallOptions, in io.Reader, out, errOut io.Writer) error {
	bundle, err := presets.Load()
	if err != nil {
		return err
	}
	info, err := system.Detect()
	if err != nil {
		return err
	}

	prompter := ui.NewPrompter(in, out)
	defaultMode := install.Mode(options.mode)

	state, _ := config.LoadInstallState(info.StatePath)
	bridgeCfg, _ := loadExistingBridgeConfig(info.BridgeConfigPath)

	if defaultMode == "" {
		if state.Mode != "" {
			defaultMode = install.Mode(state.Mode)
		} else {
			defaultMode = recommendedMode(info)
		}
	}

	mode, err := chooseMode(prompter, info, options.yes, defaultMode)
	if err != nil {
		return err
	}

	providerPreset, err := chooseProviderPreset(prompter, bundle, options.providerID, options.yes, state.ManagedProviderID)
	if err != nil {
		return err
	}

	providerCfg, err := buildProviderConfig(prompter, providerPreset, bridgeCfg.Provider, options, out)
	if err != nil {
		return err
	}

	channelSelections, err := buildChannelSelections(prompter, bundle, bridgeCfg, state.ManagedChannels, options.channels, options.yes, out)
	if err != nil {
		return err
	}

	req := install.Request{
		Mode:        mode,
		Provider:    providerCfg,
		Channels:    channelSelections,
		AppVersion:  Version,
		SkipVerify:  options.skipVerify,
		SkipInstall: options.reconfigure,
	}

	if !options.yes {
		fmt.Fprintln(out, "")
		fmt.Fprintf(out, "Mode: %s\n", req.Mode)
		fmt.Fprintf(out, "Provider: %s (%s)\n", req.Provider.Name, req.Provider.ID)
		fmt.Fprintf(out, "Primary model: %s\n", req.Provider.PrimaryModel)
		if len(req.Channels) == 0 {
			fmt.Fprintln(out, "Channels: none")
		} else {
			fmt.Fprintf(out, "Channels: %s\n", strings.Join(channelIDs(req.Channels), ", "))
		}
		confirm, err := prompter.AskYesNo("Proceed?", true)
		if err != nil {
			return err
		}
		if !confirm {
			return errors.New("cancelled")
		}
	}

	workflow := install.NewWorkflow(bundle, install.RealExecutor{})
	ctx := context.Background()

	var result install.Result
	if options.reconfigure {
		result, err = workflow.Reconfigure(ctx, info, req, out, errOut)
	} else {
		result, err = workflow.Install(ctx, info, req, out, errOut)
	}
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "")
	if options.reconfigure {
		fmt.Fprintln(out, "Reconfiguration completed.")
	} else {
		fmt.Fprintln(out, "Installation completed.")
	}
	fmt.Fprintf(out, "Config: %s\n", result.ConfigPath)
	fmt.Fprintf(out, "Bridge config: %s\n", result.BridgeConfigPath)
	fmt.Fprintf(out, "State: %s\n", result.StatePath)
	fmt.Fprintf(out, "Runtime: %s\n", result.RuntimeDir)
	if result.BackupFile != "" {
		fmt.Fprintf(out, "Backup: %s\n", result.BackupFile)
	}
	if len(result.Warnings) > 0 {
		fmt.Fprintln(out, "Warnings:")
		for _, warning := range result.Warnings {
			fmt.Fprintf(out, "  - %s\n", warning)
		}
	}
	return nil
}

func runBridge(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return errors.New("bridge requires a subcommand; use `bridge serve`")
	}

	switch args[0] {
	case "serve":
		return runBridgeServe(args[1:], out, errOut)
	default:
		return fmt.Errorf("unknown bridge subcommand %q", args[0])
	}
}

func runBridgeServe(args []string, out, errOut io.Writer) error {
	info, err := system.Detect()
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("bridge serve", flag.ContinueOnError)
	fs.SetOutput(errOut)

	channelFlag := fs.String("channel", "", "channel id: qq, feishu, wecom")
	configPathFlag := fs.String("config", info.BridgeConfigPath, "path to bridge config JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*channelFlag) == "" {
		return errors.New("--channel is required")
	}

	cfg, err := config.LoadBridgeConfig(*configPathFlag)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	return bridge.Serve(ctx, cfg, *channelFlag, out)
}

func chooseMode(prompter *ui.Prompter, info system.Info, yes bool, defaultMode install.Mode) (install.Mode, error) {
	if info.OS == "windows" {
		return install.ModeDocker, nil
	}
	if yes {
		return defaultMode, nil
	}

	choice, err := prompter.AskChoice("Select install mode", []string{install.ModeDocker.String(), install.ModeNative.String()}, defaultMode.String())
	if err != nil {
		return "", err
	}
	return install.Mode(choice), nil
}

func chooseProviderPreset(prompter *ui.Prompter, bundle presets.Bundle, providerID string, yes bool, stateProviderID string) (presets.ProviderPreset, error) {
	if providerID == "" && stateProviderID != "" {
		providerID = stateProviderID
	}
	if providerID != "" {
		provider, ok := bundle.ProviderByID(providerID)
		if !ok {
			return presets.ProviderPreset{}, fmt.Errorf("unknown provider preset %q", providerID)
		}
		return provider, nil
	}
	if yes {
		return bundle.Providers[0], nil
	}

	options := make([]string, 0, len(bundle.Providers))
	labelToProvider := make(map[string]presets.ProviderPreset, len(bundle.Providers))
	for _, provider := range bundle.Providers {
		label := fmt.Sprintf("%s (%s)", provider.Name, provider.ID)
		options = append(options, label)
		labelToProvider[label] = provider
	}
	choice, err := prompter.AskChoice("Select a provider preset", options, options[0])
	if err != nil {
		return presets.ProviderPreset{}, err
	}
	return labelToProvider[choice], nil
}

func buildProviderConfig(prompter *ui.Prompter, preset presets.ProviderPreset, existing config.ProviderConfig, options runInstallOptions, out io.Writer) (config.ProviderConfig, error) {
	cfg := config.ProviderConfig{
		ID:   preset.ID,
		Name: preset.Name,
		Type: preset.Type,
	}

	baseURL := firstNonEmpty(options.baseURL, existing.BaseURL, preset.BaseURL)
	apiKey := firstNonEmpty(options.apiKey, existing.APIKey, os.Getenv(preset.APIKeyEnv))
	primaryModel := firstNonEmpty(options.primaryModel, existing.PrimaryModel)
	if primaryModel == "" && len(preset.Models) > 0 {
		primaryModel = preset.Models[0]
	}

	fallbackModels := parseCSV(options.fallbacks)
	if len(fallbackModels) == 0 && len(existing.FallbackModels) > 0 {
		fallbackModels = existing.FallbackModels
	}
	if len(fallbackModels) == 0 && len(preset.Models) > 1 {
		fallbackModels = append([]string{}, preset.Models[1:min(3, len(preset.Models))]...)
	}

	if !options.yes {
		fmt.Fprintln(out, "")
		fmt.Fprintf(out, "Provider preset: %s\n", preset.Name)
		if preset.Notes != "" {
			fmt.Fprintf(out, "  %s\n", preset.Notes)
		}
		var err error
		baseURL, err = prompter.AskString("Base URL", baseURL, false)
		if err != nil {
			return config.ProviderConfig{}, err
		}
		apiKey, err = prompter.AskString("API Key", apiKey, true)
		if err != nil {
			return config.ProviderConfig{}, err
		}
		primaryModel, err = prompter.AskString("Primary model", primaryModel, false)
		if err != nil {
			return config.ProviderConfig{}, err
		}
		fallbackCSV, err := prompter.AskString("Fallback models (comma-separated)", strings.Join(fallbackModels, ","), false)
		if err != nil {
			return config.ProviderConfig{}, err
		}
		fallbackModels = parseCSV(fallbackCSV)
	}

	cfg.BaseURL = baseURL
	cfg.APIKey = apiKey
	cfg.PrimaryModel = primaryModel
	cfg.FallbackModels = fallbackModels
	return cfg, nil
}

func buildChannelSelections(prompter *ui.Prompter, bundle presets.Bundle, existing config.BridgeConfig, stateChannels []string, channelsFlag string, yes bool, out io.Writer) ([]config.ChannelSelection, error) {
	selectedIDs := parseCSV(channelsFlag)
	useFlagSelection := len(selectedIDs) > 0
	selections := []config.ChannelSelection{}

	for _, preset := range bundle.Channels {
		defaultEnabled := slices.Contains(stateChannels, preset.ID)
		enabled := false

		switch {
		case useFlagSelection:
			enabled = slices.Contains(selectedIDs, preset.ID)
		case yes:
			enabled = defaultEnabled
		default:
			fmt.Fprintln(out, "")
			var err error
			enabled, err = prompter.AskYesNo("Enable "+preset.Name+"?", defaultEnabled)
			if err != nil {
				return nil, err
			}
		}

		if !enabled {
			continue
		}

		existingChannel := existing.Channels[preset.ID]
		listenAddr := firstNonEmpty(existingChannel.ListenAddr, preset.DefaultListen)
		path := firstNonEmpty(existingChannel.Path, preset.DefaultPath)

		if !yes || useFlagSelection {
			var err error
			listenAddr, err = prompter.AskString(preset.Name+" listen address", listenAddr, false)
			if err != nil {
				return nil, err
			}
			path, err = prompter.AskString(preset.Name+" callback path", path, false)
			if err != nil {
				return nil, err
			}
		}

		fields := make(map[string]string, len(preset.RequiredFields))
		for _, field := range preset.RequiredFields {
			defaultValue := existingChannel.Fields[field.Key]
			if yes && field.Optional {
				fields[field.Key] = defaultValue
				continue
			}
			value := defaultValue
			var err error
			value, err = prompter.AskString(field.Label, value, field.Secret)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(value) == "" && !field.Optional {
				return nil, fmt.Errorf("%s requires %s", preset.Name, field.Label)
			}
			fields[field.Key] = value
		}

		dmPolicy, _ := preset.Defaults["dmPolicy"].(string)
		groupPolicy, _ := preset.Defaults["groupPolicy"].(string)
		if existingChannel.DMPolicy != "" {
			dmPolicy = existingChannel.DMPolicy
		}
		if existingChannel.GroupPolicy != "" {
			groupPolicy = existingChannel.GroupPolicy
		}
		if !yes {
			var err error
			dmPolicy, err = prompter.AskString(preset.Name+" DM policy", dmPolicy, false)
			if err != nil {
				return nil, err
			}
			groupPolicy, err = prompter.AskString(preset.Name+" group policy", groupPolicy, false)
			if err != nil {
				return nil, err
			}
		}

		selections = append(selections, config.ChannelSelection{
			ID:          preset.ID,
			Name:        preset.Name,
			Driver:      preset.Driver,
			ListenAddr:  listenAddr,
			Path:        path,
			Fields:      fields,
			DMPolicy:    dmPolicy,
			GroupPolicy: groupPolicy,
		})
	}

	return selections, nil
}

func loadExistingBridgeConfig(path string) (config.BridgeConfig, error) {
	cfg, err := config.LoadBridgeConfig(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config.BridgeConfig{}, nil
		}
		return config.BridgeConfig{}, err
	}
	return cfg, nil
}

func parseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func channelIDs(channels []config.ChannelSelection) []string {
	out := make([]string, 0, len(channels))
	for _, channel := range channels {
		out = append(out, channel.ID)
	}
	return out
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, "openclaw-install")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  install       Interactive installation workflow")
	fmt.Fprintln(out, "  doctor        Inspect local environment and mirror reachability")
	fmt.Fprintln(out, "  reconfigure   Rewrite provider/channel config without reinstalling")
	fmt.Fprintln(out, "  bridge serve  Run a single bridge channel process")
	fmt.Fprintln(out, "  version       Print installer version")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Examples:")
	fmt.Fprintln(out, "  openclaw-install install")
	fmt.Fprintln(out, "  openclaw-install doctor")
	fmt.Fprintln(out, "  openclaw-install bridge serve --channel qq")
}

func recommendedMode(info system.Info) install.Mode {
	if info.OS == "windows" {
		return install.ModeDocker
	}
	if info.HasDocker && info.HasCompose {
		return install.ModeDocker
	}
	return install.ModeNative
}

func sortedKeys(input map[string]string) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
