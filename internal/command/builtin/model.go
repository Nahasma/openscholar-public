package builtin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/app"
	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/connectivity"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

type modelCmd struct {
	app *app.App
}

func (c *modelCmd) Name() string { return "model" }
func (c *modelCmd) Description() string {
	return "List or switch models (e.g. /model, /model sonnet, /model general deepseek-chat)"
}

func (c *modelCmd) Execute(ctx command.Context) command.Result {
	args := strings.TrimSpace(ctx.Args)
	if args == "" {
		return c.listProviders(ctx)
	}

	parts := strings.Fields(args)

	if parts[0] == "status" {
		return c.status(ctx)
	}
	if parts[0] == "force" {
		return c.forceModel(ctx, strings.TrimSpace(strings.TrimPrefix(args, "force")))
	}
	if parts[0] == "test" {
		return c.testModel(ctx, strings.TrimSpace(strings.TrimPrefix(args, "test")))
	}
	if parts[0] == "refresh" {
		return c.refreshProvider(ctx, strings.TrimSpace(strings.TrimPrefix(args, "refresh")))
	}

	switch len(parts) {
	case 1:
		if strings.Contains(parts[0], ":") {
			if ref, err := models.ResolveModelRef(parts[0]); err == nil {
				return c.switchModelRef(ctx, ref)
			}
			if providerName, modelID, ok := splitKnownProviderModel(parts[0]); ok {
				if ref, err := resolveModelRefForConfiguredProvider(providerName, modelID); err == nil {
					return c.switchModelRef(ctx, ref)
				}
			}
		}
		// Check if it's a provider name first
		if prov, ok := models.ResolveProvider(parts[0]); ok {
			return c.listProviderModels(ctx, prov)
		}
		if ref, err := models.ResolveModelRef(parts[0]); err == nil {
			return c.switchModelRef(ctx, ref)
		}
		// Otherwise, fuzzy match model (backward compatible)
		return c.switchModel(ctx, parts[0])
	case 2:
		// Check if first arg is an agent name: /model general deepseek-chat
		if agentName, ok := config.ParseAgentName(parts[0]); ok {
			return c.setAgentModel(ctx, agentName, parts[1])
		}
		// Check if first arg is a provider: /model anthropic sonnet
		if prov, ok := models.ResolveProvider(parts[0]); ok {
			ref, err := resolveModelRefForConfiguredProvider(prov, parts[1])
			if err != nil {
				return command.Result{Output: fmt.Sprintf("未找到模型: %s %s", parts[0], parts[1])}
			}
			return c.switchModelRef(ctx, ref)
		}
		// Fallback: treat as model name
		return c.switchModel(ctx, args)
	default:
		if len(parts) == 3 {
			if agentName, ok := config.ParseAgentName(parts[0]); ok {
				if prov, ok := models.ResolveProvider(parts[1]); ok {
					return c.setAgentProviderModel(ctx, agentName, prov, parts[2])
				}
			}
		}
		return c.switchModel(ctx, args)
	}
}

func (c *modelCmd) listProviders(_ command.Context) command.Result {
	return command.Result{Action: "model-select"}
}

func (c *modelCmd) listProviderModels(ctx command.Context, prov models.ModelProvider) command.Result {
	current := ctx.App.CoderAgent.Model()
	cfg := config.Get()
	count := 0

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s 模型:\n\n", models.ProviderDisplayName(prov)))

	for _, opt := range config.ModelOptionsForProvider(cfg, prov, nil) {
		m := opt.Model

		marker := "  "
		if m.ID == current.ID {
			marker = "→ "
		}

		// Show which agents use this model
		var agents []string
		if cfg != nil {
			for agentName, agentCfg := range cfg.Agents {
				if agentCfg.Model == string(m.ID) {
					agents = append(agents, string(agentName))
				}
			}
		}

		agentStr := ""
		if len(agents) > 0 {
			agentStr = fmt.Sprintf("  [%s]", strings.Join(agents, ", "))
		}

		sb.WriteString(fmt.Sprintf("%s%-25s %s%s\n", marker, m.Name, m.ID, agentStr))
		count++
	}

	if count == 0 && models.ProviderAllowsArbitraryModel(prov) {
		sb.WriteString("此 provider 支持自由输入模型名。\n")
	}
	sb.WriteString(fmt.Sprintf("\n用法: /model <model-name> 切换模型"))
	return command.Result{Output: sb.String()}
}

func (c *modelCmd) status(ctx command.Context) command.Result {
	current := ctx.App.CoderAgent.Model()
	return command.Result{Output: fmt.Sprintf("当前 coder 模型: %s (%s:%s)", current.Name, current.Provider, current.ID)}
}

func (c *modelCmd) switchModel(ctx command.Context, query string) command.Result {
	model, ok := models.ResolveModel(query)
	if !ok {
		return command.Result{Output: fmt.Sprintf("未找到模型: %s\n使用 /model 查看可用模型", query)}
	}

	if err := ctx.App.SetModel(model.ID); err != nil {
		return command.Result{Output: fmt.Sprintf("切换失败: %v", err)}
	}

	return command.Result{Output: fmt.Sprintf("已切换为 %s (%s)", model.Name, model.ID)}
}

func (c *modelCmd) switchModelRef(ctx command.Context, ref models.ModelRef) command.Result {
	if err := ctx.App.SetModelProvider(ref.Provider, ref.ModelID); err != nil {
		return command.Result{Output: fmt.Sprintf("切换失败: %v", err)}
	}
	name := ref.ModelID
	if ref.Metadata != nil && ref.Metadata.Name != "" {
		name = ref.Metadata.Name
	}
	return command.Result{Output: fmt.Sprintf("已切换为 %s (%s:%s)", name, ref.Provider, ref.ModelID)}
}

func (c *modelCmd) forceModel(ctx command.Context, input string) command.Result {
	parts := strings.Fields(input)
	var ref models.ModelRef
	var err error

	switch len(parts) {
	case 1:
		ref, err = models.ResolveModelRefFallback(parts[0])
	case 2:
		prov, ok := models.ResolveProvider(parts[0])
		if !ok {
			return command.Result{Output: fmt.Sprintf("未知 provider: %s", parts[0])}
		}
		ref, err = models.ResolveModelRefForProviderFallback(prov, parts[1])
	default:
		return command.Result{Output: "用法: /model force provider:model 或 /model force provider model"}
	}
	if err != nil {
		return command.Result{Output: fmt.Sprintf("未找到模型: %s", input)}
	}
	if err := ctx.App.SetModelProviderForce(ref.Provider, ref.ModelID); err != nil {
		return command.Result{Output: fmt.Sprintf("设置失败: %v", err)}
	}
	return command.Result{Output: fmt.Sprintf("已强制切换为 %s (%s:%s)", ref.ModelID, ref.Provider, ref.ModelID)}
}

func (c *modelCmd) testModel(ctx command.Context, input string) command.Result {
	ref, err := resolveModelCommandRef(input)
	if err != nil {
		return command.Result{Output: fmt.Sprintf("未找到模型: %s", input)}
	}
	req, err := connectivityRequest(ref.Provider, ref.ModelID)
	if err != nil {
		return command.Result{Output: fmt.Sprintf("无法测试模型: %v", err)}
	}
	execCtx := ctx.ExecContext
	if execCtx == nil {
		execCtx = context.Background()
	}
	res := connectivity.PingChat(execCtx, req)
	return command.Result{Output: formatConnectivityResult("模型连通测试", ref.Provider, ref.ModelID, res)}
}

func (c *modelCmd) refreshProvider(ctx command.Context, input string) command.Result {
	save := false
	parts := strings.Fields(strings.TrimSpace(input))
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "--save" {
			save = true
			continue
		}
		filtered = append(filtered, p)
	}
	if len(filtered) != 1 {
		return command.Result{Output: "用法: /model refresh <provider> [--save]"}
	}
	prov, ok := models.ResolveProvider(filtered[0])
	if !ok {
		return command.Result{Output: fmt.Sprintf("未知 provider: %s", input)}
	}
	req, err := connectivityRequest(prov, "")
	if err != nil {
		return command.Result{Output: fmt.Sprintf("无法刷新模型列表: %v", err)}
	}
	execCtx := ctx.ExecContext
	if execCtx == nil {
		execCtx = context.Background()
	}
	res := connectivity.DiscoverModels(execCtx, req)
	if !res.ProviderOK && !res.Warning {
		return command.Result{Output: formatConnectivityResult("模型列表刷新", prov, "", res)}
	}
	var sb strings.Builder
	sb.WriteString(formatConnectivityResult("模型列表刷新", prov, "", res))
	if len(res.Models) > 0 {
		sb.WriteString("\n\n发现模型:\n")
		for _, m := range res.Models {
			sb.WriteString(fmt.Sprintf("  %s\n", m.ID))
		}
		if save {
			modelCfgs := make(map[string]config.ModelConfig, len(res.Models))
			for _, m := range res.Models {
				modelCfgs[string(m.ID)] = config.ModelConfig{
					Name: m.Name,
				}
			}
			cfg := config.Get()
			wd := ""
			if cfg != nil {
				wd = cfg.WorkingDir
			}
			if err := config.SaveProviderModels(wd, prov, modelCfgs); err != nil {
				sb.WriteString(fmt.Sprintf("\n\n保存失败: %v", err))
			} else {
				sb.WriteString(fmt.Sprintf("\n\n已保存 %d 个模型到配置 providers.%s.models", len(modelCfgs), prov))
			}
		}
	}
	return command.Result{Output: sb.String()}
}

func resolveModelCommandRef(input string) (models.ModelRef, error) {
	parts := strings.Fields(strings.TrimSpace(input))
	switch len(parts) {
	case 1:
		ref, err := models.ResolveModelRef(parts[0])
		if err == nil {
			return ref, nil
		}
		if providerName, modelID, ok := splitKnownProviderModel(parts[0]); ok {
			return resolveModelRefForConfiguredProvider(providerName, modelID)
		}
		return models.ModelRef{}, err
	case 2:
		prov, ok := models.ResolveProvider(parts[0])
		if !ok {
			return models.ModelRef{}, fmt.Errorf("unknown provider %q", parts[0])
		}
		return resolveModelRefForConfiguredProvider(prov, parts[1])
	default:
		return models.ModelRef{}, fmt.Errorf("usage: /model test provider:model or /model test provider model")
	}
}

func resolveModelRefForConfiguredProvider(providerName models.ModelProvider, modelID string) (models.ModelRef, error) {
	ref, err := models.ResolveModelRefForProvider(providerName, modelID)
	if err == nil {
		return ref, nil
	}
	cfg := config.Get()
	if cfg == nil {
		return models.ModelRef{}, err
	}
	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return models.ModelRef{}, err
	}
	if _, ok := providerCfg.Models[modelID]; !ok {
		return models.ModelRef{}, err
	}
	model := config.ModelFromConfig(providerName, modelID, providerCfg.Models[modelID])
	return models.ModelRef{
		Provider:      providerName,
		ModelID:       modelID,
		APIModel:      modelID,
		OwnerProvider: providerName,
		Metadata:      &model,
		Source:        models.ModelSourceCurrentConfig,
		IsRouter:      models.ProviderIsRouter(providerName),
	}, nil
}

func splitKnownProviderModel(input string) (models.ModelProvider, string, bool) {
	idx := strings.Index(input, ":")
	if idx <= 0 {
		return "", "", false
	}
	providerName, ok := models.ResolveProvider(input[:idx])
	if !ok {
		return "", "", false
	}
	modelID := strings.TrimSpace(input[idx+1:])
	if modelID == "" {
		return "", "", false
	}
	return providerName, modelID, true
}

func connectivityRequest(providerName models.ModelProvider, modelID string) (connectivity.TestRequest, error) {
	cfg := config.Get()
	if cfg == nil {
		return connectivity.TestRequest{}, fmt.Errorf("config is not loaded")
	}
	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return connectivity.TestRequest{}, fmt.Errorf("provider %s is not configured", providerName)
	}
	apiKey, _, err := config.ResolveAPIKey(providerName, providerCfg)
	if err != nil {
		return connectivity.TestRequest{}, err
	}
	if modelID == "" {
		modelID = providerCfg.Model
	}
	return connectivity.TestRequest{
		Provider: providerName,
		ModelID:  modelID,
		APIKey:   apiKey,
		BaseURL:  providerCfg.BaseURL,
		Profile:  providerCfg.Profile,
		AuthMode: providerCfg.AuthMode,
		Timeout:  10 * time.Second,
	}, nil
}

func formatConnectivityResult(title string, providerName models.ModelProvider, modelID string, res connectivity.TestResult) string {
	status := "失败"
	if res.ProviderOK && (modelID == "" || res.ModelOK) {
		status = "成功"
	}
	if res.Warning {
		status = "警告"
	}
	target := string(providerName)
	if modelID != "" {
		target += ":" + modelID
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %s: %s", title, status, target))
	if res.Category != "" {
		sb.WriteString(fmt.Sprintf("\n类别: %s", res.Category))
	}
	if res.Message != "" {
		sb.WriteString(fmt.Sprintf("\n信息: %s", res.Message))
	}
	if res.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("\n建议: 检查 baseURL，常见格式为 %s", res.Suggestion))
	}
	if res.Latency > 0 {
		sb.WriteString(fmt.Sprintf("\n耗时: %s", res.Latency.Truncate(time.Millisecond)))
	}
	return sb.String()
}

func (c *modelCmd) setAgentModel(ctx command.Context, agentName config.AgentName, modelQuery string) command.Result {
	ref, err := models.ResolveModelRef(modelQuery)
	if err != nil {
		return command.Result{Output: fmt.Sprintf("未找到模型: %s", modelQuery)}
	}

	// For coder agent, use the existing SetModel which also updates the live agent
	if agentName == config.AgentCoder {
		if err := ctx.App.SetModelProvider(ref.Provider, ref.ModelID); err != nil {
			return command.Result{Output: fmt.Sprintf("设置失败: %v", err)}
		}
		return command.Result{Output: fmt.Sprintf("coder Agent 已切换为 %s (%s:%s)", ref.ModelID, ref.Provider, ref.ModelID)}
	}

	// For other agents, persist provider+model
	if err := config.SaveAgentModelProvider(config.WorkingDirectory(), agentName, ref.Provider, ref.ModelID); err != nil {
		return command.Result{Output: fmt.Sprintf("设置失败: %v", err)}
	}

	return command.Result{Output: fmt.Sprintf("%s Agent 已切换为 %s (%s:%s)", agentName, ref.ModelID, ref.Provider, ref.ModelID)}
}

func (c *modelCmd) setAgentProviderModel(ctx command.Context, agentName config.AgentName, providerName models.ModelProvider, modelQuery string) command.Result {
	ref, err := resolveModelRefForConfiguredProvider(providerName, modelQuery)
	if err != nil {
		return command.Result{Output: fmt.Sprintf("未找到模型: %s %s", providerName, modelQuery)}
	}
	if agentName == config.AgentCoder {
		if err := ctx.App.SetModelProvider(ref.Provider, ref.ModelID); err != nil {
			return command.Result{Output: fmt.Sprintf("设置失败: %v", err)}
		}
		return command.Result{Output: fmt.Sprintf("coder Agent 已切换为 %s (%s:%s)", ref.ModelID, ref.Provider, ref.ModelID)}
	}
	if err := config.SaveAgentModelProvider(config.WorkingDirectory(), agentName, ref.Provider, ref.ModelID); err != nil {
		return command.Result{Output: fmt.Sprintf("设置失败: %v", err)}
	}
	return command.Result{Output: fmt.Sprintf("%s Agent 已切换为 %s (%s:%s)", agentName, ref.ModelID, ref.Provider, ref.ModelID)}
}
