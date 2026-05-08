package models

import (
	"fmt"
	"log"
)

// Resolution holds the result of a model resolution attempt.
type Resolution struct {
	Model       Model
	RequestedID string
	IsFallback  bool
	Warning     string
}

// customProviders are providers that allow arbitrary model IDs not in SupportedModels.
// SmallFastModelForProvider returns the fastest and cheapest known model for the given provider.
// Returns an empty string for providers without a designated lightweight model.
func SmallFastModelForProvider(p ModelProvider) ModelID {
	switch p {
	case ProviderAnthropic:
		return Claude37Haiku
	case ProviderOpenAI:
		return GPT41Mini
	case ProviderDeepSeek:
		return DeepSeekChat
	case ProviderMiniMax:
		return MiniMaxM27
	case ProviderGLM:
		return GLM5
	case ProviderSiliconFlow:
		return SFDeepSeekV3
	default:
		return ""
	}
}

// lightweightAgents is the set of agent names considered lightweight.
var lightweightAgents = map[string]bool{
	"task":    true,
	"title":   true,
	"explore": true,
}

// IsLightweightAgent reports whether the given agent name is a lightweight agent.
// Accepts a plain string to avoid import cycles with the config package.
func IsLightweightAgent(name string) bool {
	return lightweightAgents[name]
}

// ResolveWithFallback resolves a model ID for the given provider without panicking on unknown models.
//
// Resolution rules:
//  1. If the requested ID is in SupportedModels, return it directly.
//  2. If the provider is a custom/local provider (openai_compatible, ollama, vllm), build a
//     pass-through Model with zero cost/context values and IsFallback=true.
//  3. For strict providers, the model is unknown; return IsFallback=true with a warning.
//     Cost and context fields are intentionally left at zero.
func ResolveWithFallback(provider ModelProvider, requested string) (Resolution, error) {
	if requested == "" {
		return Resolution{}, fmt.Errorf("requested model ID must not be empty")
	}

	// 1. Exact provider-aware lookup in the supported models registry.
	if m, ok := SupportedModels[ModelID(requested)]; ok && m.Provider == provider {
		return Resolution{
			Model:       m,
			RequestedID: requested,
			IsFallback:  false,
		}, nil
	}

	// 2. Custom/local providers — allow any model string.
	if ProviderAllowsArbitraryModel(provider) {
		warning := fmt.Sprintf("model %q is not in the known model registry for provider %q; using as custom model", requested, provider)
		log.Printf("[models/routing] %s", warning)
		m := NewCustomModel(provider, requested)
		return Resolution{
			Model:       m,
			RequestedID: requested,
			IsFallback:  true,
			Warning:     warning,
		}, nil
	}

	// 3. Strict provider with an unknown model — degrade gracefully.
	warning := fmt.Sprintf("model %q is unknown for provider %q; proceeding without cost/context metadata", requested, provider)
	log.Printf("[models/routing] %s", warning)
	m := Model{
		ID:            ModelID(requested),
		Name:          requested,
		Provider:      provider,
		APIModel:      requested,
		MetadataState: ModelMetadataUnknown,
	}
	return Resolution{
		Model:       m,
		RequestedID: requested,
		IsFallback:  true,
		Warning:     warning,
	}, nil
}
