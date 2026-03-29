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
	"github.com/goodtiger/openclaw-install/internal/shared"
	"github.com/goodtiger/openclaw-install/internal/system"
	"github.com/goodtiger/openclaw-install/internal/ui"
	"github.com/goodtiger/openclaw-install/presets"
)

// placeholderAPIKey 是配置文件中 API Key 的占位符值。
// 写入配置前应校验是否仍为该值并提示用户填写。
const placeholderAPIKey = "YOUR_API_KEY"

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
			fmt.Fprintln(errOut, "doctor 执行失败：", err)
			return 1
		}
		return 0
	case "install":
		if err := runInstall(args[1:], in, out, errOut); err != nil {
			fmt.Fprintln(errOut, "install 执行失败：", err)
			return 1
		}
		return 0
	case "reconfigure":
		if err := runReconfigure(args[1:], in, out, errOut); err != nil {
			fmt.Fprintln(errOut, "reconfigure 执行失败：", err)
			return 1
		}
		return 0
	case "upgrade":
		if err := runUpgrade(args[1:], out, errOut); err != nil {
			fmt.Fprintln(errOut, "upgrade 执行失败：", err)
			return 1
		}
		return 0
	case "bridge":
		if err := runBridge(args[1:], out, errOut); err != nil {
			fmt.Fprintln(errOut, "bridge 执行失败：", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(errOut, "未知命令：%s\n\n", args[0])
		printHelp(errOut)
		return 2
	}
}

func runDoctor(args []string, out, errOut io.Writer) error {
	fs := newFlagSet("doctor", errOut, "检查本机环境、依赖检测结果与镜像可达性。")
	previewFlag := fs.Bool("preview", false, "预览自动检测的配置值及来源")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
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

	fmt.Fprintf(out, "系统：%s/%s\n", report.Info.OS, report.Info.Arch)
	fmt.Fprintf(out, "OpenClaw 目录：%s\n", report.Info.OpenClawHome)
	fmt.Fprintf(out, "配置路径：%s\n", report.Info.ConfigPath)
	fmt.Fprintf(out, "包管理器：%s\n", shared.ValueOrDefault(report.Info.PackageManager, "未检测到"))
	fmt.Fprintf(out, "推荐模式：%s\n", report.RecommendedMode)
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "已检测工具：")
	fmt.Fprintf(out, "  docker: %s\n", boolLabel(report.Info.HasDocker))
	if report.Info.HasDocker {
		fmt.Fprintf(out, "  docker daemon: %s\n", boolLabel(report.Info.DockerHealthy))
	}
	fmt.Fprintf(out, "  docker compose: %s\n", boolLabel(report.Info.HasCompose))
	fmt.Fprintf(out, "  node: %s\n", boolLabel(report.Info.HasNode))
	fmt.Fprintf(out, "  npm: %s\n", boolLabel(report.Info.HasNPM))
	fmt.Fprintf(out, "  openclaw: %s\n", boolLabel(report.Info.HasOpenClaw))
	fmt.Fprintf(out, "  git: %s\n", boolLabel(report.Info.HasGit))
	fmt.Fprintf(out, "  curl: %s\n", boolLabel(report.Info.HasCurl))
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "镜像选择：")
	for _, key := range shared.SortedStringKeys(report.MirrorNames) {
		fmt.Fprintf(out, "  %s：%s\n", key, report.MirrorNames[key])
	}

	if *previewFlag {
		fmt.Fprintln(out, "")
		printDetectionPreview(out, bundle, info, report)
	}

	if len(report.Warnings) > 0 {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "警告：")
		for _, warning := range report.Warnings {
			fmt.Fprintf(out, "  - %s\n", warning)
		}
	}
	return nil
}

// printDetectionPreview shows what the auto-detect engine would resolve for each config value.
func printDetectionPreview(out io.Writer, bundle presets.Bundle, info system.Info, report install.DoctorReport) {
	fmt.Fprintln(out, "检测结果：")

	// Mode
	fmt.Fprintf(out, "  ├─ 安装模式：    %-16s", report.RecommendedMode)
	if report.Info.DockerHealthy && report.Info.HasCompose {
		fmt.Fprintln(out, "(检测到 Docker 守护进程)")
	} else {
		fmt.Fprintln(out, "(Docker 不可用，使用 native 模式)")
	}

	// Provider
	providerResolved := shared.ResolveWithSource(
		shared.ResolvedValue{Value: os.Getenv("OPENCLAW_PROVIDER"), Source: "env:OPENCLAW_PROVIDER"},
	)
	if providerResolved.Value == "" {
		for _, p := range bundle.Providers {
			if p.APIKeyEnv == "" {
				continue
			}
			if v := strings.TrimSpace(os.Getenv(p.APIKeyEnv)); v != "" {
				providerResolved = shared.ResolvedValue{Value: p.ID, Source: fmt.Sprintf("env:%s (推断)", p.APIKeyEnv)}
				break
			}
		}
	}
	if providerResolved.Value != "" {
		provider, ok := bundle.ProviderByID(providerResolved.Value)
		name := providerResolved.Value
		if ok {
			name = provider.Name
		}
		fmt.Fprintf(out, "  ├─ 供应商：      %-16s(%s)\n", name, providerResolved.Source)
	} else {
		fmt.Fprintln(out, "  ├─ 供应商：      未检测到          (需手动选择)")
	}

	// API Key
	apiKeyResolved := shared.ResolveWithSource(
		shared.ResolvedValue{Value: os.Getenv("OPENCLAW_API_KEY"), Source: "env:OPENCLAW_API_KEY"},
	)
	if apiKeyResolved.Value == "" && providerResolved.Value != "" {
		if provider, ok := bundle.ProviderByID(providerResolved.Value); ok && provider.APIKeyEnv != "" {
			apiKeyResolved = shared.ResolveWithSource(
				shared.ResolvedValue{Value: os.Getenv(provider.APIKeyEnv), Source: fmt.Sprintf("env:%s", provider.APIKeyEnv)},
			)
		}
	}
	if apiKeyResolved.Value != "" {
		fmt.Fprintf(out, "  ├─ API Key：     %-16s(%s)\n", shared.MaskSecret(apiKeyResolved.Value), apiKeyResolved.Source)
	} else {
		fmt.Fprintln(out, "  ├─ API Key：     未检测到          (需手动输入)")
	}

	// Model
	modelResolved := shared.ResolveWithSource(
		shared.ResolvedValue{Value: os.Getenv("OPENCLAW_MODEL"), Source: "env:OPENCLAW_MODEL"},
	)
	if modelResolved.Value == "" && providerResolved.Value != "" {
		if provider, ok := bundle.ProviderByID(providerResolved.Value); ok && provider.DefaultModel != "" {
			modelResolved = shared.ResolvedValue{Value: provider.DefaultModel, Source: "默认"}
		}
	}
	if modelResolved.Value != "" {
		fmt.Fprintf(out, "  ├─ 主模型：      %-16s(%s)\n", modelResolved.Value, modelResolved.Source)
	} else {
		fmt.Fprintln(out, "  ├─ 主模型：      未检测到          (需手动选择)")
	}

	fmt.Fprintln(out, "  └─ 通道：        无自动检测        (需手动配置)")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "运行 openclaw-install install 以使用这些设置安装。")
}

func runInstall(args []string, in io.Reader, out, errOut io.Writer) error {
	fs := newFlagSet("install", errOut, "执行交互式安装流程。")

	modeFlag := fs.String("mode", "", "安装模式：docker 或 native")
	providerFlag := fs.String("provider", "", "供应商预设 ID")
	baseURLFlag := fs.String("base-url", "", "供应商 Base URL")
	apiKeyFlag := fs.String("api-key", "", "供应商 API Key")
	primaryFlag := fs.String("primary-model", "", "主模型 ID")
	fallbackFlag := fs.String("fallback-models", "", "候选模型列表，逗号分隔")
	channelsFlag := fs.String("channels", "", "通道 ID 列表，逗号分隔")
	yesFlag := fs.Bool("yes", false, "尽量直接接受默认值")
	skipVerifyFlag := fs.Bool("skip-verify", false, "跳过安装后的校验")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
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
	fs := newFlagSet("reconfigure", errOut, "不重新安装 OpenClaw，仅重写 provider/channel 配置。")

	modeFlag := fs.String("mode", "", "继续使用的安装模式")
	providerFlag := fs.String("provider", "", "供应商预设 ID")
	baseURLFlag := fs.String("base-url", "", "供应商 Base URL")
	apiKeyFlag := fs.String("api-key", "", "供应商 API Key")
	primaryFlag := fs.String("primary-model", "", "主模型 ID")
	fallbackFlag := fs.String("fallback-models", "", "候选模型列表，逗号分隔")
	channelsFlag := fs.String("channels", "", "通道 ID 列表，逗号分隔")
	yesFlag := fs.Bool("yes", false, "尽量直接接受默认值")
	skipVerifyFlag := fs.Bool("skip-verify", false, "跳过重写配置后的校验")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
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

	// Non-TTY safety: require --yes when stdin is not a terminal.
	if !info.IsTTY && !options.yes {
		return errors.New("非交互式终端中运行，请使用 --yes 参数跳过提示")
	}

	prompter := ui.NewPrompter(in, out)
	defaultMode := install.Mode(options.mode)

	state, err := config.LoadInstallState(info.StatePath)
	if err != nil {
		return err
	}
	bridgeCfg, err := loadExistingBridgeConfig(info.BridgeConfigPath)
	if err != nil {
		return err
	}

	if defaultMode == "" {
		if envMode := strings.TrimSpace(os.Getenv("OPENCLAW_MODE")); envMode != "" {
			defaultMode = install.Mode(envMode)
		} else if state.Mode != "" {
			defaultMode = install.Mode(state.Mode)
		} else {
			defaultMode = install.RecommendedMode(info)
		}
	}

	mode, err := chooseMode(prompter, options.yes, defaultMode)
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
		fmt.Fprintf(out, "安装模式：%s\n", req.Mode)
		fmt.Fprintf(out, "供应商：%s (%s)\n", req.Provider.Name, req.Provider.ID)
		fmt.Fprintf(out, "主模型：%s\n", req.Provider.PrimaryModel)
		if len(req.Channels) == 0 {
			fmt.Fprintln(out, "通道：未启用")
		} else {
			fmt.Fprintf(out, "通道：%s\n", strings.Join(install.ChannelIDs(req.Channels), ", "))
		}
		confirm, err := prompter.AskYesNo("确认继续吗？", true)
		if err != nil {
			return err
		}
		if !confirm {
			return errors.New("已取消")
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
	printSuccessSummary(out, req, result, options.reconfigure)
	return nil
}

// printSuccessSummary prints a visually distinct success box after install/reconfigure.
func printSuccessSummary(out io.Writer, req install.Request, result install.Result, reconfigure bool) {
	border := "╔══════════════════════════════════════════════════════════╗"
	bottom := "╚══════════════════════════════════════════════════════════╝"
	sep := "╟──────────────────────────────────────────────────────────╢"

	title := "安装完成"
	if reconfigure {
		title = "重配置完成"
	}

	fmt.Fprintln(out, border)
	fmt.Fprintf(out, "║  %s%s║\n", title, strings.Repeat(" ", 56-len([]rune(title))*2))
	fmt.Fprintln(out, sep)
	fmt.Fprintf(out, "║  安装模式：%-47s║\n", req.Mode)
	fmt.Fprintf(out, "║  供应商：  %-47s║\n", fmt.Sprintf("%s (%s)", req.Provider.Name, req.Provider.ID))
	fmt.Fprintf(out, "║  主模型：  %-47s║\n", req.Provider.PrimaryModel)
	fmt.Fprintf(out, "║  API Key： %-47s║\n", shared.MaskSecret(req.Provider.APIKey))
	if len(req.Channels) == 0 {
		fmt.Fprintf(out, "║  通道：    %-47s║\n", "未启用")
	} else {
		fmt.Fprintf(out, "║  通道：    %-47s║\n", strings.Join(install.ChannelIDs(req.Channels), ", "))
	}
	fmt.Fprintln(out, sep)
	fmt.Fprintf(out, "║  配置文件：%-47s║\n", result.ConfigPath)
	fmt.Fprintf(out, "║  桥接配置：%-47s║\n", result.BridgeConfigPath)
	if result.BackupFile != "" {
		fmt.Fprintf(out, "║  备份文件：%-47s║\n", result.BackupFile)
	}
	fmt.Fprintln(out, sep)
	fmt.Fprintf(out, "║  试试看：  %-47s║\n", "openclaw chat \"你好\"")
	fmt.Fprintln(out, bottom)

	if len(result.Warnings) > 0 {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "警告：")
		for _, warning := range result.Warnings {
			fmt.Fprintf(out, "  - %s\n", warning)
		}
	}
}

func runBridge(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return errors.New("bridge 需要子命令，可使用 `bridge serve`")
	}

	switch args[0] {
	case "serve":
		return runBridgeServe(args[1:], out, errOut)
	default:
		return fmt.Errorf("未知的 bridge 子命令 %q", args[0])
	}
}

func runBridgeServe(args []string, out, errOut io.Writer) error {
	info, err := system.Detect()
	if err != nil {
		return err
	}

	fs := newFlagSet("bridge serve", errOut, "启动单个 bridge 通道进程。")

	channelFlag := fs.String("channel", "", "bridge 通道 ID：feishu、wecom")
	configPathFlag := fs.String("config", info.BridgeConfigPath, "bridge 配置 JSON 路径")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*channelFlag) == "" {
		return errors.New("必须提供 --channel")
	}

	cfg, err := config.LoadBridgeConfig(*configPathFlag)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	return bridge.Serve(ctx, cfg, *channelFlag, out)
}

func chooseMode(prompter *ui.Prompter, yes bool, defaultMode install.Mode) (install.Mode, error) {
	if yes {
		return defaultMode, nil
	}

	choice, err := prompter.AskChoice("请选择安装模式", []string{install.ModeDocker.String(), install.ModeNative.String()}, defaultMode.String())
	if err != nil {
		return "", err
	}
	return install.Mode(choice), nil
}

func chooseProviderPreset(prompter *ui.Prompter, bundle presets.Bundle, providerID string, yes bool, stateProviderID string) (presets.ProviderPreset, error) {
	if len(bundle.Providers) == 0 {
		return presets.ProviderPreset{}, errors.New("没有可用的供应商预设")
	}
	// 1. Explicit flag
	if providerID == "" && stateProviderID != "" {
		providerID = stateProviderID
	}
	// 2. OPENCLAW_PROVIDER env var
	if providerID == "" {
		providerID = strings.TrimSpace(os.Getenv("OPENCLAW_PROVIDER"))
	}
	if providerID != "" {
		provider, ok := bundle.ProviderByID(providerID)
		if !ok {
			return presets.ProviderPreset{}, fmt.Errorf("未知的供应商预设 %q", providerID)
		}
		return provider, nil
	}
	// 3. Infer from provider-specific API key env vars (first-match-wins in preset order)
	for _, provider := range bundle.Providers {
		if provider.APIKeyEnv == "" {
			continue
		}
		if v := strings.TrimSpace(os.Getenv(provider.APIKeyEnv)); v != "" {
			return provider, nil
		}
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
	choice, err := prompter.AskChoice("请选择供应商预设", options, options[0])
	if err != nil {
		return presets.ProviderPreset{}, err
	}
	return labelToProvider[choice], nil
}

func buildProviderConfig(prompter *ui.Prompter, preset presets.ProviderPreset, existing config.ProviderConfig, options runInstallOptions, out io.Writer) (config.ProviderConfig, error) {
	cfg := config.ProviderConfig{
		ID:      preset.ID,
		Name:    preset.Name,
		Type:    preset.Type,
		API:     firstNonEmpty(existing.API, preset.API),
		Catalog: convertProviderCatalog(preset.Catalog),
	}

	baseURL := firstNonEmpty(options.baseURL, os.Getenv("OPENCLAW_BASE_URL"), existing.BaseURL, preset.BaseURL)
	apiKey := firstNonEmpty(options.apiKey, os.Getenv("OPENCLAW_API_KEY"), os.Getenv(preset.APIKeyEnv), existing.APIKey, placeholderAPIKey)
	primaryModel := firstNonEmpty(options.primaryModel, os.Getenv("OPENCLAW_MODEL"), existing.PrimaryModel, preset.DefaultModel)
	if primaryModel == "" {
		modelIDs := providerModelIDs(preset, cfg.Catalog)
		if len(modelIDs) > 0 {
			primaryModel = modelIDs[0]
		}
	}

	fallbackModels := parseCSV(options.fallbacks)
	if len(fallbackModels) == 0 && len(existing.FallbackModels) > 0 {
		fallbackModels = existing.FallbackModels
	}
	if len(fallbackModels) == 0 {
		modelIDs := providerModelIDs(preset, cfg.Catalog)
		for _, modelID := range modelIDs {
			if modelID == primaryModel {
				continue
			}
			fallbackModels = append(fallbackModels, modelID)
		}
	}

	if !options.yes {
		fmt.Fprintln(out, "")
		fmt.Fprintf(out, "供应商预设：%s\n", preset.Name)
		if preset.Notes != "" {
			fmt.Fprintf(out, "  %s\n", preset.Notes)
		}
		var err error
		baseURL, err = prompter.AskString("Base URL（接口地址）", baseURL, false)
		if err != nil {
			return config.ProviderConfig{}, err
		}
		apiKey, err = prompter.AskString("API Key", apiKey, true)
		if err != nil {
			return config.ProviderConfig{}, err
		}
		primaryModel, err = prompter.AskString("主模型", primaryModel, false)
		if err != nil {
			return config.ProviderConfig{}, err
		}
		fallbackCSV, err := prompter.AskString("候选模型（逗号分隔）", strings.Join(fallbackModels, ","), false)
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
		enabled, err := resolveChannelEnabled(prompter, preset, stateChannels, selectedIDs, useFlagSelection, yes, out)
		if err != nil {
			return nil, err
		}
		if !enabled {
			continue
		}

		existingChannel := existing.Channels[preset.ID]

		listenAddr, path, err := collectNetworkConfig(prompter, preset, existingChannel, useFlagSelection, yes)
		if err != nil {
			return nil, err
		}

		fields, err := collectCredentialFields(prompter, preset, existingChannel, yes)
		if err != nil {
			return nil, err
		}

		dmPolicy, groupPolicy, err := collectChannelPolicies(prompter, preset, existingChannel, yes)
		if err != nil {
			return nil, err
		}

		selections = append(selections, config.ChannelSelection{
			ID:              preset.ID,
			Name:            preset.Name,
			Driver:          preset.Driver,
			Provisioner:     normalizedProvisioner(preset.Provisioner),
			ListenAddr:      listenAddr,
			Path:            path,
			Fields:          fields,
			PluginPackage:   preset.PluginPackage,
			OpenClawChannel: preset.OpenClawChannel,
			TokenFields:     slices.Clone(preset.TokenFields),
			LoginRequired:   preset.LoginRequired,
			DMPolicy:        dmPolicy,
			GroupPolicy:     groupPolicy,
		})
	}

	return selections, nil
}

func resolveChannelEnabled(prompter *ui.Prompter, preset presets.ChannelPreset, stateChannels, selectedIDs []string, useFlagSelection, yes bool, out io.Writer) (bool, error) {
	defaultEnabled := slices.Contains(stateChannels, preset.ID) || preset.DefaultEnabled
	switch {
	case useFlagSelection:
		return slices.Contains(selectedIDs, preset.ID), nil
	case yes:
		return defaultEnabled, nil
	default:
		fmt.Fprintln(out, "")
		return prompter.AskYesNo("启用 "+preset.Name+" 吗？", defaultEnabled)
	}
}

func collectNetworkConfig(prompter *ui.Prompter, preset presets.ChannelPreset, existing config.BridgeChannelConfig, useFlagSelection, yes bool) (listenAddr, path string, err error) {
	listenAddr = firstNonEmpty(existing.ListenAddr, preset.DefaultListen)
	path = firstNonEmpty(existing.Path, preset.DefaultPath)
	if !usesBridgeProvisioner(preset.Provisioner) || (yes && !useFlagSelection) {
		return listenAddr, path, nil
	}
	listenAddr, err = prompter.AskString(preset.Name+" 监听地址", listenAddr, false)
	if err != nil {
		return "", "", err
	}
	path, err = prompter.AskString(preset.Name+" 回调路径", path, false)
	return listenAddr, path, err
}

func collectCredentialFields(prompter *ui.Prompter, preset presets.ChannelPreset, existing config.BridgeChannelConfig, yes bool) (map[string]string, error) {
	fields := make(map[string]string, len(preset.RequiredFields))
	for _, field := range preset.RequiredFields {
		defaultValue := firstNonEmpty(envOrEmpty(field.EnvKey), existing.Fields[field.Key])
		if yes && field.Optional {
			fields[field.Key] = defaultValue
			continue
		}
		if yes && strings.TrimSpace(defaultValue) != "" {
			fields[field.Key] = defaultValue
			continue
		}
		value, err := prompter.AskString(field.Label, defaultValue, field.Secret)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(value) == "" && !field.Optional {
			return nil, fmt.Errorf("%s 需要填写 %s", preset.Name, field.Label)
		}
		fields[field.Key] = value
	}
	return fields, nil
}

func collectChannelPolicies(prompter *ui.Prompter, preset presets.ChannelPreset, existing config.BridgeChannelConfig, yes bool) (dmPolicy, groupPolicy string, err error) {
	dmPolicy, _ = preset.Defaults["dmPolicy"].(string)
	groupPolicy, _ = preset.Defaults["groupPolicy"].(string)
	if existing.DMPolicy != "" {
		dmPolicy = existing.DMPolicy
	}
	if existing.GroupPolicy != "" {
		groupPolicy = existing.GroupPolicy
	}
	if yes {
		return dmPolicy, groupPolicy, nil
	}
	dmPolicy, err = prompter.AskString(preset.Name+" 私聊策略", dmPolicy, false)
	if err != nil {
		return "", "", err
	}
	groupPolicy, err = prompter.AskString(preset.Name+" 群聊策略", groupPolicy, false)
	return dmPolicy, groupPolicy, err
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

func printHelp(out io.Writer) {
	fmt.Fprintln(out, "openclaw-install")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "命令：")
	fmt.Fprintln(out, "  install            交互式安装流程")
	fmt.Fprintln(out, "  doctor             检查本机环境与镜像可达性")
	fmt.Fprintln(out, "  doctor --preview   预览自动检测的配置值及来源")
	fmt.Fprintln(out, "  reconfigure        不重新安装，只重写 provider/channel 配置")
	fmt.Fprintln(out, "  bridge serve       启动单个 bridge 通道进程")
	fmt.Fprintln(out, "  upgrade            自我更新到最新版本")
	fmt.Fprintln(out, "  version            输出安装器版本")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "示例：")
	fmt.Fprintln(out, "  openclaw-install install")
	fmt.Fprintln(out, "  openclaw-install install --yes            # 全自动安装")
	fmt.Fprintln(out, "  openclaw-install doctor --preview         # 预览检测结果")
	fmt.Fprintln(out, "  openclaw-install bridge serve --channel feishu")
}

func newFlagSet(name string, out io.Writer, summary string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() {
		fmt.Fprintf(out, "用法：openclaw-install %s [参数]\n", name)
		if strings.TrimSpace(summary) != "" {
			fmt.Fprintln(out, summary)
		}
		if flagSetHasOptions(fs) {
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "参数：")
			fs.PrintDefaults()
		}
	}
	return fs
}

func flagSetHasOptions(fs *flag.FlagSet) bool {
	hasOptions := false
	fs.VisitAll(func(*flag.Flag) {
		hasOptions = true
	})
	return hasOptions
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func providerModelIDs(preset presets.ProviderPreset, catalog []config.ProviderModel) []string {
	if len(catalog) > 0 {
		out := make([]string, 0, len(catalog))
		for _, model := range catalog {
			if strings.TrimSpace(model.ID) == "" {
				continue
			}
			out = append(out, model.ID)
		}
		return out
	}
	return append([]string{}, preset.Models...)
}

func convertProviderCatalog(input []presets.ProviderModel) []config.ProviderModel {
	if len(input) == 0 {
		return nil
	}
	out := make([]config.ProviderModel, 0, len(input))
	for _, model := range input {
		out = append(out, config.ProviderModel{
			ID:            model.ID,
			Name:          model.Name,
			Reasoning:     model.Reasoning,
			Input:         append([]string{}, model.Input...),
			Cost:          config.ModelCost(model.Cost),
			ContextWindow: model.ContextWindow,
			MaxTokens:     model.MaxTokens,
		})
	}
	return out
}

func normalizedProvisioner(value string) string {
	if strings.TrimSpace(value) == "" {
		return "bridge"
	}
	return value
}

func usesBridgeProvisioner(provisioner string) bool {
	return normalizedProvisioner(provisioner) == "bridge"
}

func boolLabel(value bool) string {
	if value {
		return "已检测"
	}
	return "未检测"
}

// envOrEmpty returns the env var value or empty string if the key is empty or unset.
func envOrEmpty(key string) string {
	if key == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(key))
}
