package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/llm/agent"
	agentcustom "github.com/openscholar/openscholar/internal/llm/agent/custom"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/provider"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "管理自定义 Agent",
}

var agentCreateCmd = &cobra.Command{
	Use:   "create [description]",
	Short: "通过自然语言描述创建 Agent",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		description := strings.Join(args, " ")

		cwd, _ := os.Getwd()
		cfg, err := config.Load(cwd)
		if err != nil {
			return fmt.Errorf("无法加载配置: %w", err)
		}

		p, err := customAgentProvider(cfg)
		if err != nil {
			return fmt.Errorf("无法初始化 Provider: %w", err)
		}

		fmt.Println("正在生成 Agent 配置...")
		agentConfig, err := agent.Generate(cmd.Context(), description, p)
		if err != nil {
			return fmt.Errorf("生成失败: %w", err)
		}

		agentDir := config.DataPath("agents")
		os.MkdirAll(agentDir, 0755)

		path := filepath.Join(agentDir, agentConfig.Name+".md")
		if err := os.WriteFile(path, []byte(agentConfig.ToMarkdown()), 0644); err != nil {
			return fmt.Errorf("写入失败: %w", err)
		}

		fmt.Printf("Agent 已创建: %s\n", path)
		fmt.Printf("  名称: %s\n", agentConfig.Name)
		fmt.Printf("  描述: %s\n", agentConfig.Description)
		if agentConfig.WhenToUse != "" {
			fmt.Printf("  用途: %s\n", agentConfig.WhenToUse)
		}
		return nil
	},
}

func customAgentProvider(cfg *config.Config) (provider.Provider, error) {
	providerName := cfg.DefaultProvider
	agentCfg := cfg.Agents[config.AgentCoder]
	if agentCfg.Provider != "" {
		providerName = agentCfg.Provider
	}
	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %s not configured", providerName)
	}
	modelName := agentCfg.Model
	if modelName == "" {
		modelName = providerCfg.Model
	}
	model, ok := models.SupportedModels[models.ModelID(modelName)]
	if !ok {
		return nil, fmt.Errorf("模型 %s 不可用", modelName)
	}
	apiKey, _, _ := config.ResolveAPIKey(providerName, providerCfg)
	return provider.NewProvider(
		providerName,
		provider.WithAPIKey(apiKey),
		provider.WithBaseURL(providerCfg.BaseURL),
		provider.WithProviderProfile(providerCfg.Profile),
		provider.WithProviderAuthMode(providerCfg.AuthMode),
		provider.WithAuthSource(config.ProviderAuthSource(cfg, providerName)),
		provider.WithModel(model),
		provider.WithMaxTokens(4096),
	)
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有自定义 Agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		loader := agentcustom.NewLoader(cwd)
		agents := loader.LoadAll()

		if len(agents) == 0 {
			fmt.Println("暂无自定义 Agent")
			return nil
		}
		for _, a := range agents {
			fmt.Printf("  %s — %s\n", a.Name, a.Description)
		}
		return nil
	},
}

var agentDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "删除自定义 Agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		cwd, _ := os.Getwd()
		loader := agentcustom.NewLoader(cwd)

		agentConfig, err := loader.Load(name)
		if err != nil {
			return fmt.Errorf("Agent %q 不存在", name)
		}

		if err := os.Remove(agentConfig.FilePath); err != nil {
			return fmt.Errorf("删除失败: %w", err)
		}

		fmt.Printf("已删除 Agent: %s\n", name)
		return nil
	},
}

func init() {
	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentDeleteCmd)
	rootCmd.AddCommand(agentCmd)
}
