package builtin

import (
	"fmt"
	"os"
	"strings"

	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/llm/models"
)

type configCmd struct{}

func (c *configCmd) Name() string        { return "config" }
func (c *configCmd) Description() string { return "设置中心 (providers, agents, models)" }
func (c *configCmd) Spec() command.CommandSpec {
	return command.CommandSpec{
		Name:         "config",
		Aliases:      []string{"settings"},
		Category:     "setup",
		Source:       "builtin",
		Description:  c.Description(),
		ArgumentHint: "[show|wizard|debug|providers|agents]",
		Usage:        "/config [show|wizard|debug|providers|agents]",
		Subcommands: []command.CommandSpec{
			{Name: "show", Description: "显示当前配置摘要", WhenToUse: "检查当前生效配置"},
			{Name: "wizard", Description: "打开配置向导", WhenToUse: "交互式修改配置"},
			{Name: "debug", Description: "显示配置来源与认证来源排查信息", WhenToUse: "排查配置/密钥问题"},
			{Name: "providers", Description: "仅查看 provider 状态", WhenToUse: "快速查看 provider 可用性"},
			{Name: "agents", Description: "仅查看 agent 映射", WhenToUse: "快速查看 provider/model 分配"},
		},
		CanRunWhileBusy: true,
	}
}

func (c *configCmd) Execute(ctx command.Context) command.Result {
	args := strings.TrimSpace(ctx.Args)

	switch args {
	case "":
		return c.showCenter()
	case "wizard":
		if config.Get() == nil {
			return command.Result{Output: "配置未加载。请先运行 openscholar init 或设置 provider API key 后重试。"}
		}
		return command.Result{
			Action:                "config-wizard",
			Actions:               []command.CommandAction{{Kind: command.CommandActionConfigWizard}},
			OutputDismissText:     "Config wizard dismissed",
			SuppressDismissRecord: true,
		}
	case "show":
		return c.showConfig()
	case "debug":
		return command.Result{
			Output:            config.DebugReport(config.Get()),
			OutputKind:        command.OutputCommandBlock,
			OutputDismissText: "Config debug dismissed",
		}
	case "providers":
		return c.showProviders()
	case "agents":
		return c.showAgents()
	default:
		return command.Result{Output: "用法: /config [show|wizard|debug|providers|agents]"}
	}
}

func (c *configCmd) showConfig() command.Result {
	cfg := config.Get()
	if cfg == nil {
		return command.Result{Output: "配置未加载"}
	}

	var sb strings.Builder
	sb.WriteString("当前配置:\n\n")

	// Default provider
	sb.WriteString(fmt.Sprintf("  默认 Provider: %s\n\n", models.ProviderDisplayName(cfg.DefaultProvider)))

	// Providers
	sb.WriteString("  已配置的 Providers:\n")
	for _, prov := range models.ProviderDisplayOrder() {
		if p, ok := cfg.Providers[prov]; ok && p.APIKey != "" {
			sb.WriteString(fmt.Sprintf("    ✓ %s\n", models.ProviderDisplayName(prov)))
		}
	}

	// Agents
	sb.WriteString("\n  Agent 配置:\n")
	for _, name := range []config.AgentName{
		config.AgentCoder, config.AgentGeneral, config.AgentExplore,
		config.AgentSummarizer, config.AgentTitle, config.AgentLeader,
	} {
		if a, ok := cfg.Agents[name]; ok {
			provName := string(a.Provider)
			if provName == "" {
				provName = string(cfg.DefaultProvider)
			}
			sb.WriteString(fmt.Sprintf("    %-12s %s (%s)\n", name, a.Model, provName))
		}
	}

	sb.WriteString("\n  使用 /config 查看设置中心")
	return command.Result{
		Output:            sb.String(),
		OutputKind:        command.OutputCommandBlock,
		OutputDismissText: "Settings dialog dismissed",
	}
}

func (c *configCmd) showCenter() command.Result {
	cfg := config.Get()
	var configured, missing int
	if cfg != nil {
		for _, prov := range models.ProviderDisplayOrder() {
			auth := config.ProviderAuthSource(cfg, prov)
			switch auth {
			case "env", "config", "none":
				configured++
			default:
				missing++
			}
		}
	}
	defaultProvider := "(未设置)"
	configPath := "(未加载)"
	if cfg != nil {
		defaultProvider = models.ProviderDisplayName(cfg.DefaultProvider)
		configPath = config.ConfigFilePath(cfg.WorkingDir)
	}
	var sb strings.Builder
	if cfg == nil {
		sb.WriteString("  配置状态: 未加载\n")
	} else {
		sb.WriteString(fmt.Sprintf("  Provider 状态: %d ready / %d missing\n", configured, missing))
	}
	sb.WriteString(fmt.Sprintf("  默认 Provider: %s\n", defaultProvider))
	sb.WriteString(fmt.Sprintf("  配置路径: %s\n\n", configPath))
	sb.WriteString("下一步:\n")
	sb.WriteString("  /config wizard      交互式修改配置\n")
	sb.WriteString("  /config show        查看当前生效配置（密钥已掩码）\n")
	sb.WriteString("  /config debug       查看配置来源与告警\n")
	sb.WriteString("  /config providers   查看 provider 详情\n")
	sb.WriteString("  /config agents      查看 agent 映射\n")
	return command.Result{
		Output:            sb.String(),
		OutputTitle:       "Settings Center",
		OutputKind:        command.OutputCommandBlock,
		OutputDismissText: "Settings dialog dismissed",
	}
}

func (c *configCmd) showProviders() command.Result {
	cfg := config.Get()
	if cfg == nil {
		return command.Result{Output: "配置未加载"}
	}
	var sb strings.Builder
	sb.WriteString("Providers\n\n")
	for _, prov := range models.ProviderDisplayOrder() {
		pcfg := cfg.Providers[prov]
		auth := config.ProviderAuthSource(cfg, prov)
		disabled := "no"
		if pcfg.Disabled {
			disabled = "yes"
		}
		sb.WriteString(fmt.Sprintf("  - %-20s auth=%-7s disabled=%s\n", models.ProviderDisplayName(prov), auth, disabled))
	}
	return command.Result{
		Output:            sb.String(),
		OutputKind:        command.OutputCommandBlock,
		OutputDismissText: "Provider status dismissed",
	}
}

func (c *configCmd) showAgents() command.Result {
	cfg := config.Get()
	if cfg == nil {
		return command.Result{Output: "配置未加载"}
	}
	var sb strings.Builder
	sb.WriteString("Agents\n\n")
	for _, name := range []config.AgentName{
		config.AgentCoder, config.AgentGeneral, config.AgentExplore, config.AgentSummarizer, config.AgentTitle, config.AgentLeader,
		config.AgentTask, config.AgentPlan, config.AgentVerify, config.AgentCoordinator,
	} {
		a, ok := cfg.Agents[name]
		if !ok {
			continue
		}
		prov := a.Provider
		if prov == "" {
			prov = cfg.DefaultProvider
		}
		sb.WriteString(fmt.Sprintf("  - %-12s provider=%-18s model=%s\n", name, prov, a.Model))
	}
	if wd, err := os.Getwd(); err == nil && wd != cfg.WorkingDir {
		sb.WriteString(fmt.Sprintf("\n  note: cwd=%s, config.workingDir=%s\n", wd, cfg.WorkingDir))
	}
	return command.Result{
		Output:            sb.String(),
		OutputKind:        command.OutputCommandBlock,
		OutputDismissText: "Agent mapping dismissed",
	}
}
