package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/prompt"
	"github.com/openscholar/openscholar/internal/llm/provider"
	"github.com/openscholar/openscholar/internal/llm/web"
	"github.com/openscholar/openscholar/internal/message"
)

type requestProviderCtxKeyType struct{}

var requestProviderCtxKey = requestProviderCtxKeyType{}

func (a *agent) SetModel(model models.Model) error {
	if a.IsBusy() {
		return ErrSessionBusy
	}

	cfg := config.Get()
	agentCfg := cfg.Agents[config.AgentCoder]
	providerName := model.Provider
	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return fmt.Errorf("provider %s not configured", providerName)
	}

	maxTokens := model.DefaultMaxTokens
	if agentCfg.MaxTokens > 0 {
		maxTokens = agentCfg.MaxTokens
	}

	newProvider, err := provider.NewProvider(
		providerName,
		provider.WithAPIKey(func() string {
			key, _, _ := config.ResolveAPIKey(providerName, providerCfg)
			return key
		}()),
		provider.WithBaseURL(providerCfg.BaseURL),
		provider.WithProviderProfile(providerCfg.Profile),
		provider.WithProviderAuthMode(providerCfg.AuthMode),
		provider.WithAuthSource(config.ProviderAuthSource(cfg, providerName)),
		provider.WithModel(model),
		provider.WithSystemMessage(prompt.GetAgentPrompt(config.AgentCoder)),
		provider.WithSystemBlocks(promptBlocksToProvider(prompt.BuildAgentPromptBlocks(config.AgentCoder))),
		provider.WithSystemPromptSource(func() provider.SystemPrompt {
			runtimePrompt := prompt.BuildAgentPromptRuntime(config.AgentCoder, time.Now())
			return provider.SystemPrompt{
				Message: runtimePrompt.SystemMessage,
				Blocks:  promptBlocksToProvider(runtimePrompt.Blocks),
			}
		}),
		provider.WithMaxTokens(maxTokens),
	)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	a.agentProvider = newProvider
	return nil
}

// ReloadProvider recreates the agent's LLM provider from current config.
// Call after config changes (e.g., /config wizard) to apply new settings.
func (a *agent) ReloadProvider() error {
	if a.IsBusy() {
		return ErrSessionBusy
	}
	newProvider, err := createAgentProvider(a.agentName)
	if err != nil {
		return err
	}
	a.agentProvider = newProvider

	// Also refresh the summarizer provider if applicable
	if a.agentName == config.AgentCoder {
		sp, _ := createAgentProvider(config.AgentSummarizer)
		a.summarizerProvider = sp
	}
	return nil
}

// CreateCallLLM creates a lightweight LLMCaller using the given agent's provider config.
// This is useful for tools (e.g. KBQuery, KBSearch) that need to call LLM independently.
func CreateCallLLM(agentName config.AgentName) (func(ctx context.Context, prompt string) (string, error), error) {
	p, err := createAgentProvider(agentName)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, prompt string) (string, error) {
		resp, err := p.SendMessages(ctx, []message.Message{{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: prompt}},
		}}, nil)
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}, nil
}

// CreateSmallFastCallLLM creates an LLM caller for lightweight tool-side tasks.
// It prefers AgentSummarizer and falls back to AgentCoder if summarizer is unavailable.
func CreateSmallFastCallLLM() (func(ctx context.Context, prompt string) (string, error), error) {
	callLLM, err := CreateCallLLM(config.AgentSummarizer)
	if err == nil {
		return callLLM, nil
	}
	fallback, fallbackErr := CreateCallLLM(config.AgentCoder)
	if fallbackErr != nil {
		return nil, errors.Join(
			fmt.Errorf("summarizer caller unavailable: %w", err),
			fmt.Errorf("coder fallback caller unavailable: %w", fallbackErr),
		)
	}
	return fallback, nil
}

func (a *agent) providerForRequest(ctx context.Context) provider.Provider {
	if p, ok := ctx.Value(requestProviderCtxKey).(provider.Provider); ok && p != nil {
		return p
	}
	return a.agentProvider
}

func (a *agent) modelForRequest(ctx context.Context) models.Model {
	return a.providerForRequest(ctx).Model()
}

func (a *agent) prepareRequestRuntime(ctx context.Context) (context.Context, error) {
	runtime, ok := RequestRuntimeFromContext(ctx)
	if !ok {
		return ctx, nil
	}
	if runtime.DisableModelInvocation {
		return nil, ErrModelInvocationDisabled
	}
	if len(runtime.AllowedTools) > 0 {
		_, unknown := resolveAllowedToolSet(a.allTools(), runtime.AllowedTools)
		if len(unknown) > 0 {
			return nil, fmt.Errorf("runtime override references unknown tools: %s", strings.Join(unknown, ", "))
		}
	}
	if runtime.Model == "" {
		return ctx, nil
	}

	p, err := a.createRuntimeProvider(runtime.Model)
	if err != nil {
		return nil, err
	}
	return context.WithValue(ctx, requestProviderCtxKey, p), nil
}

func (a *agent) createRuntimeProvider(requested string) (provider.Provider, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return nil, fmt.Errorf("runtime model override is empty")
	}

	cfg := config.Get()
	currentProvider := a.agentProvider.Model().Provider
	var model models.Model

	ref, err := models.ResolveModelRef(requested)
	if err == nil {
		providerName := ref.Provider
		if providerName != currentProvider {
			return nil, fmt.Errorf("runtime model override %q resolves to provider %s; current provider is %s", requested, providerName, currentProvider)
		}
		if ref.Metadata != nil {
			model = *ref.Metadata
		} else {
			resolution, resErr := models.ResolveWithFallback(providerName, ref.ModelID)
			if resErr != nil {
				return nil, fmt.Errorf("model resolution failed for runtime override %q: %w", requested, resErr)
			}
			model = resolution.Model
		}
		return a.buildRuntimeProvider(cfg, providerName, model)
	}
	if providerName, modelID, ok := splitKnownProviderModel(requested); ok {
		if providerName != currentProvider {
			return nil, fmt.Errorf("runtime model override %q targets provider %s; current provider is %s", requested, providerName, currentProvider)
		}
		providerCfg, ok := cfg.Providers[providerName]
		if !ok {
			return nil, fmt.Errorf("provider %s not configured for runtime model override", providerName)
		}
		model, err = resolveConfiguredModel(providerName, modelID, providerCfg)
		if err != nil {
			return nil, fmt.Errorf("model resolution failed for runtime override %q: %w", requested, err)
		}
		return a.buildRuntimeProvider(cfg, providerName, model)
	}

	providerCfg, ok := cfg.Providers[currentProvider]
	if !ok {
		return nil, fmt.Errorf("provider %s not configured for runtime model override", currentProvider)
	}
	model, err = resolveConfiguredModel(currentProvider, requested, providerCfg)
	if err != nil {
		return nil, fmt.Errorf("model resolution failed for runtime override %q: %w", requested, err)
	}
	return a.buildRuntimeProvider(cfg, currentProvider, model)
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

func (a *agent) buildRuntimeProvider(cfg *config.Config, providerName models.ModelProvider, model models.Model) (provider.Provider, error) {
	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %s not configured for runtime model override", providerName)
	}
	if providerCfg.Disabled {
		return nil, fmt.Errorf("provider %s is disabled", providerName)
	}

	maxTokens := model.DefaultMaxTokens
	if agentCfg, ok := cfg.Agents[a.agentName]; ok && agentCfg.MaxTokens > 0 {
		maxTokens = agentCfg.MaxTokens
	}
	apiKey, _, _ := config.ResolveAPIKey(providerName, providerCfg)

	p, err := provider.NewProvider(
		providerName,
		provider.WithAPIKey(apiKey),
		provider.WithBaseURL(providerCfg.BaseURL),
		provider.WithProviderProfile(providerCfg.Profile),
		provider.WithProviderAuthMode(providerCfg.AuthMode),
		provider.WithAuthSource(config.ProviderAuthSource(cfg, providerName)),
		provider.WithModel(model),
		provider.WithSystemMessage(prompt.GetAgentPrompt(a.agentName)),
		provider.WithSystemBlocks(promptBlocksToProvider(prompt.BuildAgentPromptBlocks(a.agentName))),
		provider.WithSystemPromptSource(func() provider.SystemPrompt {
			runtimePrompt := prompt.BuildAgentPromptRuntime(a.agentName, time.Now())
			return provider.SystemPrompt{
				Message: runtimePrompt.SystemMessage,
				Blocks:  promptBlocksToProvider(runtimePrompt.Blocks),
			}
		}),
		provider.WithMaxTokens(maxTokens),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create runtime override provider: %w", err)
	}
	return p, nil
}

func createAgentProvider(agentName config.AgentName) (provider.Provider, error) {
	cfg := config.Get()
	agentConfig, ok := cfg.Agents[agentName]
	if !ok {
		return nil, fmt.Errorf("agent %s not configured", agentName)
	}
	providerName := agentConfig.Provider
	if providerName == "" {
		providerName = cfg.DefaultProvider
	}

	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %s not configured", providerName)
	}
	if providerCfg.Disabled {
		return nil, fmt.Errorf("provider %s is disabled", providerName)
	}

	modelName := agentConfig.Model
	if modelName == "" {
		modelName = providerCfg.Model
	}
	if modelName == "" {
		return nil, fmt.Errorf("agent %s has no model configured", agentName)
	}

	model, err := resolveConfiguredModel(providerName, modelName, providerCfg)
	if err != nil {
		return nil, fmt.Errorf("model resolution failed for agent %s: %w", agentName, err)
	}

	maxTokens := model.DefaultMaxTokens
	if agentConfig.MaxTokens > 0 {
		maxTokens = agentConfig.MaxTokens
	}

	// C8: Resolve API key from multiple sources (env > config > helper)
	apiKey, _, _ := config.ResolveAPIKey(providerName, providerCfg)

	return provider.NewProvider(
		providerName,
		provider.WithAPIKey(apiKey),
		provider.WithBaseURL(providerCfg.BaseURL),
		provider.WithProviderProfile(providerCfg.Profile),
		provider.WithProviderAuthMode(providerCfg.AuthMode),
		provider.WithAuthSource(config.ProviderAuthSource(cfg, providerName)),
		provider.WithModel(model),
		provider.WithSystemMessage(prompt.GetAgentPrompt(agentName)),
		provider.WithSystemBlocks(promptBlocksToProvider(prompt.BuildAgentPromptBlocks(agentName))),
		provider.WithSystemPromptSource(func() provider.SystemPrompt {
			runtimePrompt := prompt.BuildAgentPromptRuntime(agentName, time.Now())
			return provider.SystemPrompt{
				Message: runtimePrompt.SystemMessage,
				Blocks:  promptBlocksToProvider(runtimePrompt.Blocks),
			}
		}),
		provider.WithMaxTokens(maxTokens),
	)
}

func resolveConfiguredModel(providerName models.ModelProvider, modelName string, providerCfg config.Provider) (models.Model, error) {
	ref, err := models.ResolveModelRefForProvider(providerName, modelName)
	if err == nil {
		if ref.Metadata == nil {
			return models.Model{}, fmt.Errorf("model %s resolved without metadata", modelName)
		}
		return *ref.Metadata, nil
	}
	if modelCfg, ok := providerCfg.Models[modelName]; ok {
		return config.ModelFromConfig(providerName, modelName, modelCfg), nil
	}
	return models.Model{}, err
}

func promptBlocksToProvider(blocks []prompt.PromptBlock) []provider.SystemBlock {
	result := make([]provider.SystemBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Text == "" {
			continue
		}
		result = append(result, provider.SystemBlock{
			Text:      block.Text,
			IsDynamic: block.IsDynamic,
		})
	}
	return result
}

// CreateWebSearchBackend creates a web.SearchProvider for the given agent's
// provider configuration. Returns nil if the provider doesn't support web search.
func CreateWebSearchBackend(agentName config.AgentName, openaiSearchModel string) web.SearchProvider {
	p, err := createAgentProvider(agentName)
	if err != nil {
		return nil
	}
	return provider.WebSearchBackend(p, openaiSearchModel)
}

// WebSearchBackendType returns the backend type ("anthropic"/"openai") for the agent's provider.
func WebSearchBackendType(agentName config.AgentName) string {
	p, err := createAgentProvider(agentName)
	if err != nil {
		return ""
	}
	return provider.WebSearchBackendType(p)
}
