package models

import "strings"

const (
	defaultAnthropicContextWindow   int64 = 200000
	defaultOpenAIContextWindow      int64 = 200000
	defaultDeepSeekContextWindow    int64 = 65536
	defaultMiniMaxContextWindow     int64 = 204800
	defaultGLMContextWindow         int64 = 200000
	defaultSiliconFlowContextWindow int64 = 65536
	defaultCustomContextWindow      int64 = 32768
)

// RuntimeContextWindow resolves a non-zero context window for runtime budgeting.
// Known model metadata wins; otherwise provider/family heuristics provide a
// conservative fallback so CTX, /context, and auto-compact keep functioning.
func RuntimeContextWindow(model Model) int64 {
	if model.ContextWindow > 0 {
		return model.ContextWindow
	}

	if inferred := inferContextWindowFromModelName(model.APIModel); inferred > 0 {
		return inferred
	}
	if inferred := inferContextWindowFromModelName(string(model.ID)); inferred > 0 {
		return inferred
	}

	switch model.Provider {
	case ProviderAnthropic:
		return defaultAnthropicContextWindow
	case ProviderOpenAI:
		return defaultOpenAIContextWindow
	case ProviderDeepSeek:
		return defaultDeepSeekContextWindow
	case ProviderMiniMax:
		return defaultMiniMaxContextWindow
	case ProviderGLM:
		return defaultGLMContextWindow
	case ProviderSiliconFlow:
		return defaultSiliconFlowContextWindow
	case ProviderOpenAICompatible, ProviderOllama, ProviderVLLM,
		ProviderGroq, ProviderTogether, ProviderFireworks, ProviderMistral,
		ProviderMoonshot, ProviderDashScope, ProviderXAI, ProviderPerplexity,
		ProviderOpenRouter, ProviderLiteLLM, ProviderLMStudio, ProviderLocalAI,
		ProviderLlamaCPP, ProviderRouter, ProviderGateway:
		return defaultCustomContextWindow
	default:
		return defaultAnthropicContextWindow
	}
}

func inferContextWindowFromModelName(name string) int64 {
	v := strings.ToLower(strings.TrimSpace(name))
	if v == "" {
		return 0
	}

	switch {
	case strings.Contains(v, "gpt-4.1"):
		return 1047576
	case strings.Contains(v, "gpt-5"):
		return 1050000
	case strings.Contains(v, "[1m]"):
		return 1000000
	case strings.Contains(v, "claude"),
		strings.Contains(v, "sonnet"),
		strings.Contains(v, "opus"),
		strings.Contains(v, "haiku"):
		return defaultAnthropicContextWindow
	case strings.Contains(v, "deepseek"):
		return defaultDeepSeekContextWindow
	case strings.Contains(v, "minimax"):
		return defaultMiniMaxContextWindow
	case strings.Contains(v, "glm"):
		return defaultGLMContextWindow
	case strings.Contains(v, "qwen"),
		strings.Contains(v, "llama"),
		strings.Contains(v, "mistral"),
		strings.Contains(v, "gemma"):
		return defaultCustomContextWindow
	default:
		return 0
	}
}
