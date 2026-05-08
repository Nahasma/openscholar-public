package models

import "strings"

type (
	ModelID       string
	ModelProvider string
)

type ModelMetadataState string
type ModelStatus string

const (
	ModelMetadataKnown   ModelMetadataState = "known"
	ModelMetadataPartial ModelMetadataState = "partial"
	ModelMetadataUnknown ModelMetadataState = "unknown"

	ModelStatusCurrent    ModelStatus = "current"
	ModelStatusDeprecated ModelStatus = "deprecated"
	ModelStatusLegacy     ModelStatus = "legacy"
)

type Model struct {
	ID                 ModelID            `json:"id"`
	Name               string             `json:"name"`
	Provider           ModelProvider      `json:"provider"`
	APIModel           string             `json:"api_model"`
	CostPer1MIn        float64            `json:"cost_per_1m_in"`
	CostPer1MOut       float64            `json:"cost_per_1m_out"`
	CostPer1MInCached  float64            `json:"cost_per_1m_in_cached"`
	CostPer1MOutCached float64            `json:"cost_per_1m_out_cached"`
	ContextWindow      int64              `json:"context_window"`
	DefaultMaxTokens   int64              `json:"default_max_tokens"`
	CanReason          bool               `json:"can_reason"`
	MetadataState      ModelMetadataState `json:"metadata_state,omitempty"`
	CostKnown          bool               `json:"cost_known,omitempty"`
	Status             ModelStatus        `json:"status,omitempty"`
	StatusNote         string             `json:"status_note,omitempty"`
}

// Providers
const (
	ProviderAnthropic        ModelProvider = "anthropic"
	ProviderOpenAI           ModelProvider = "openai"
	ProviderDeepSeek         ModelProvider = "deepseek"
	ProviderMiniMax          ModelProvider = "minimax"
	ProviderGLM              ModelProvider = "glm"
	ProviderSiliconFlow      ModelProvider = "siliconflow"
	ProviderGroq             ModelProvider = "groq"
	ProviderTogether         ModelProvider = "together"
	ProviderFireworks        ModelProvider = "fireworks"
	ProviderMistral          ModelProvider = "mistral"
	ProviderMoonshot         ModelProvider = "moonshot"
	ProviderDashScope        ModelProvider = "dashscope"
	ProviderXAI              ModelProvider = "xai"
	ProviderPerplexity       ModelProvider = "perplexity"
	ProviderOpenRouter       ModelProvider = "openrouter"
	ProviderLiteLLM          ModelProvider = "litellm"
	ProviderLMStudio         ModelProvider = "lmstudio"
	ProviderLocalAI          ModelProvider = "localai"
	ProviderLlamaCPP         ModelProvider = "llamacpp"
	ProviderRouter           ModelProvider = "router"
	ProviderGateway          ModelProvider = "gateway"
	ProviderOpenAICompatible ModelProvider = "openai_compatible"
	ProviderOllama           ModelProvider = "ollama"
	ProviderVLLM             ModelProvider = "vllm"
)

// Model IDs
const (
	Claude4Sonnet      ModelID = "claude-sonnet-4-20250514"
	Claude4Opus        ModelID = "claude-opus-4-20250514"
	Claude37Haiku      ModelID = "claude-3-5-haiku-20241022"
	Claude46Sonnet     ModelID = "claude-sonnet-4-6"
	Claude47Opus       ModelID = "claude-opus-4-7"
	Claude45Haiku      ModelID = "claude-haiku-4-5-20251001"
	Claude45HaikuAlias ModelID = "claude-haiku-4-5"

	GPT55     ModelID = "gpt-5.5"
	GPT54     ModelID = "gpt-5.4"
	GPT54Mini ModelID = "gpt-5.4-mini"
	GPT41     ModelID = "gpt-4.1"
	GPT41Mini ModelID = "gpt-4.1-mini"
	O4Mini    ModelID = "o4-mini"

	DeepSeekV4Flash ModelID = "deepseek-v4-flash"
	DeepSeekV4Pro   ModelID = "deepseek-v4-pro"
	DeepSeekChat    ModelID = "deepseek-chat"
	DeepSeekR1      ModelID = "deepseek-reasoner"

	MiniMaxM27          ModelID = "MiniMax-M2.7"
	MiniMaxM27HighSpeed ModelID = "MiniMax-M2.7-highspeed"
	MiniMaxM25          ModelID = "MiniMax-M2.5"
	MiniMaxM25HighSpeed ModelID = "MiniMax-M2.5-highspeed"

	GLM5      ModelID = "glm-5"
	GLM5Turbo ModelID = "glm-5-turbo"
	GLM47     ModelID = "glm-4.7"
	GLM46     ModelID = "glm-4.6"

	SFDeepSeekV3 ModelID = "deepseek-ai/DeepSeek-V3"
	SFDeepSeekR1 ModelID = "deepseek-ai/DeepSeek-R1"
	SFQwen35397B ModelID = "Qwen/Qwen3.5-397B-A17B"
	SFQwen35122B ModelID = "Qwen/Qwen3.5-122B-A10B"
	SFQwen3527B  ModelID = "Qwen/Qwen3.5-27B"
)

var SupportedModels = withCuratedMetadata(map[ModelID]Model{
	Claude4Sonnet: {
		ID:                 Claude4Sonnet,
		Name:               "Claude Sonnet 4",
		Provider:           ProviderAnthropic,
		APIModel:           "claude-sonnet-4-20250514",
		CostPer1MIn:        3.0,
		CostPer1MOut:       15.0,
		CostPer1MInCached:  3.75,
		CostPer1MOutCached: 0.30,
		ContextWindow:      200000,
		DefaultMaxTokens:   16384,
		CanReason:          true,
		Status:             ModelStatusLegacy,
	},
	Claude4Opus: {
		ID:                 Claude4Opus,
		Name:               "Claude Opus 4",
		Provider:           ProviderAnthropic,
		APIModel:           "claude-opus-4-20250514",
		CostPer1MIn:        15.0,
		CostPer1MOut:       75.0,
		CostPer1MInCached:  18.75,
		CostPer1MOutCached: 1.50,
		ContextWindow:      200000,
		DefaultMaxTokens:   16384,
		CanReason:          true,
		Status:             ModelStatusLegacy,
	},
	Claude37Haiku: {
		ID:               Claude37Haiku,
		Name:             "Claude 3.5 Haiku",
		Provider:         ProviderAnthropic,
		APIModel:         "claude-3-5-haiku-20241022",
		CostPer1MIn:      0.80,
		CostPer1MOut:     4.0,
		ContextWindow:    200000,
		DefaultMaxTokens: 8192,
		Status:           ModelStatusLegacy,
	},
	Claude47Opus: {
		ID:               Claude47Opus,
		Name:             "Claude Opus 4.7",
		Provider:         ProviderAnthropic,
		APIModel:         "claude-opus-4-7",
		ContextWindow:    1000000,
		DefaultMaxTokens: 128000,
		CanReason:        true,
		Status:           ModelStatusCurrent,
	},
	Claude46Sonnet: {
		ID:               Claude46Sonnet,
		Name:             "Claude Sonnet 4.6",
		Provider:         ProviderAnthropic,
		APIModel:         "claude-sonnet-4-6",
		ContextWindow:    1000000,
		DefaultMaxTokens: 64000,
		CanReason:        true,
		Status:           ModelStatusCurrent,
	},
	Claude45Haiku: {
		ID:               Claude45Haiku,
		Name:             "Claude Haiku 4.5",
		Provider:         ProviderAnthropic,
		APIModel:         "claude-haiku-4-5-20251001",
		ContextWindow:    200000,
		DefaultMaxTokens: 64000,
		Status:           ModelStatusCurrent,
	},
	Claude45HaikuAlias: {
		ID:               Claude45HaikuAlias,
		Name:             "Claude Haiku 4.5 (Alias)",
		Provider:         ProviderAnthropic,
		APIModel:         "claude-haiku-4-5",
		ContextWindow:    200000,
		DefaultMaxTokens: 64000,
		Status:           ModelStatusCurrent,
	},
	GPT55: {
		ID:               GPT55,
		Name:             "GPT-5.5",
		Provider:         ProviderOpenAI,
		APIModel:         "gpt-5.5",
		CostPer1MIn:      5.0,
		CostPer1MOut:     30.0,
		ContextWindow:    1000000,
		DefaultMaxTokens: 128000,
		CanReason:        true,
		Status:           ModelStatusCurrent,
	},
	GPT54: {
		ID:               GPT54,
		Name:             "GPT-5.4",
		Provider:         ProviderOpenAI,
		APIModel:         "gpt-5.4",
		CostPer1MIn:      2.50,
		CostPer1MOut:     15.0,
		ContextWindow:    1000000,
		DefaultMaxTokens: 128000,
		CanReason:        true,
		Status:           ModelStatusCurrent,
	},
	GPT54Mini: {
		ID:               GPT54Mini,
		Name:             "GPT-5.4 Mini",
		Provider:         ProviderOpenAI,
		APIModel:         "gpt-5.4-mini",
		CostPer1MIn:      0.75,
		CostPer1MOut:     4.50,
		ContextWindow:    400000,
		DefaultMaxTokens: 128000,
		Status:           ModelStatusCurrent,
	},
	GPT41: {
		ID:                 GPT41,
		Name:               "GPT-4.1",
		Provider:           ProviderOpenAI,
		APIModel:           "gpt-4.1",
		CostPer1MIn:        2.0,
		CostPer1MOut:       8.0,
		CostPer1MOutCached: 0.50,
		ContextWindow:      1047576,
		DefaultMaxTokens:   32768,
		Status:             ModelStatusLegacy,
	},
	GPT41Mini: {
		ID:                 GPT41Mini,
		Name:               "GPT-4.1 Mini",
		Provider:           ProviderOpenAI,
		APIModel:           "gpt-4.1-mini",
		CostPer1MIn:        0.40,
		CostPer1MOut:       1.60,
		CostPer1MOutCached: 0.10,
		ContextWindow:      1047576,
		DefaultMaxTokens:   32768,
		Status:             ModelStatusLegacy,
	},
	O4Mini: {
		ID:                 O4Mini,
		Name:               "o4-mini",
		Provider:           ProviderOpenAI,
		APIModel:           "o4-mini",
		CostPer1MIn:        1.10,
		CostPer1MOut:       4.40,
		CostPer1MOutCached: 0.275,
		ContextWindow:      200000,
		DefaultMaxTokens:   100000,
		CanReason:          true,
		Status:             ModelStatusLegacy,
	},
	DeepSeekV4Flash: {
		ID:               DeepSeekV4Flash,
		Name:             "DeepSeek-V4-Flash",
		Provider:         ProviderDeepSeek,
		APIModel:         "deepseek-v4-flash",
		ContextWindow:    65536,
		DefaultMaxTokens: 8192,
		Status:           ModelStatusCurrent,
	},
	DeepSeekV4Pro: {
		ID:               DeepSeekV4Pro,
		Name:             "DeepSeek-V4-Pro",
		Provider:         ProviderDeepSeek,
		APIModel:         "deepseek-v4-pro",
		ContextWindow:    65536,
		DefaultMaxTokens: 8192,
		CanReason:        true,
		Status:           ModelStatusCurrent,
	},
	DeepSeekChat: {
		ID:                DeepSeekChat,
		Name:              "DeepSeek-V3",
		Provider:          ProviderDeepSeek,
		APIModel:          "deepseek-chat",
		CostPer1MIn:       0.27,
		CostPer1MOut:      1.10,
		CostPer1MInCached: 0.07,
		ContextWindow:     65536,
		DefaultMaxTokens:  8192,
		Status:            ModelStatusDeprecated,
		StatusNote:        "Deprecated on 2026-07-24 per provider notice; retained for compatibility before that date.",
	},
	DeepSeekR1: {
		ID:                DeepSeekR1,
		Name:              "DeepSeek-R1",
		Provider:          ProviderDeepSeek,
		APIModel:          "deepseek-reasoner",
		CostPer1MIn:       0.55,
		CostPer1MOut:      2.19,
		CostPer1MInCached: 0.14,
		ContextWindow:     65536,
		DefaultMaxTokens:  8192,
		CanReason:         true,
		Status:            ModelStatusDeprecated,
		StatusNote:        "Deprecated on 2026-07-24 per provider notice; retained for compatibility before that date.",
	},
	MiniMaxM27: {
		ID:               MiniMaxM27,
		Name:             "MiniMax-M2.7",
		Provider:         ProviderMiniMax,
		APIModel:         "MiniMax-M2.7",
		ContextWindow:    204800,
		DefaultMaxTokens: 16384,
		CanReason:        true,
		Status:           ModelStatusCurrent,
	},
	MiniMaxM27HighSpeed: {
		ID:               MiniMaxM27HighSpeed,
		Name:             "MiniMax-M2.7 HighSpeed",
		Provider:         ProviderMiniMax,
		APIModel:         "MiniMax-M2.7-highspeed",
		ContextWindow:    204800,
		DefaultMaxTokens: 16384,
		CanReason:        true,
		Status:           ModelStatusCurrent,
	},
	MiniMaxM25: {
		ID:               MiniMaxM25,
		Name:             "MiniMax-M2.5",
		Provider:         ProviderMiniMax,
		APIModel:         "MiniMax-M2.5",
		ContextWindow:    204800,
		DefaultMaxTokens: 16384,
		CanReason:        true,
		Status:           ModelStatusCurrent,
	},
	MiniMaxM25HighSpeed: {
		ID:               MiniMaxM25HighSpeed,
		Name:             "MiniMax-M2.5 HighSpeed",
		Provider:         ProviderMiniMax,
		APIModel:         "MiniMax-M2.5-highspeed",
		ContextWindow:    204800,
		DefaultMaxTokens: 16384,
		CanReason:        true,
		Status:           ModelStatusCurrent,
	},
	GLM5: {
		ID:               GLM5,
		Name:             "GLM-5",
		Provider:         ProviderGLM,
		APIModel:         "glm-5",
		ContextWindow:    200000,
		DefaultMaxTokens: 16384,
		CanReason:        true,
	},
	GLM5Turbo: {
		ID:               GLM5Turbo,
		Name:             "GLM-5 Turbo",
		Provider:         ProviderGLM,
		APIModel:         "glm-5-turbo",
		ContextWindow:    200000,
		DefaultMaxTokens: 16384,
		CanReason:        true,
	},
	GLM47: {
		ID:               GLM47,
		Name:             "GLM-4.7",
		Provider:         ProviderGLM,
		APIModel:         "glm-4.7",
		ContextWindow:    200000,
		DefaultMaxTokens: 16384,
		CanReason:        true,
	},
	GLM46: {
		ID:               GLM46,
		Name:             "GLM-4.6",
		Provider:         ProviderGLM,
		APIModel:         "glm-4.6",
		ContextWindow:    200000,
		DefaultMaxTokens: 16384,
		CanReason:        true,
	},
	SFDeepSeekV3: {
		ID:               SFDeepSeekV3,
		Name:             "DeepSeek-V3 (SiliconFlow)",
		Provider:         ProviderSiliconFlow,
		APIModel:         "deepseek-ai/DeepSeek-V3",
		ContextWindow:    65536,
		DefaultMaxTokens: 8192,
	},
	SFDeepSeekR1: {
		ID:               SFDeepSeekR1,
		Name:             "DeepSeek-R1 (SiliconFlow)",
		Provider:         ProviderSiliconFlow,
		APIModel:         "deepseek-ai/DeepSeek-R1",
		ContextWindow:    65536,
		DefaultMaxTokens: 8192,
		CanReason:        true,
	},
	SFQwen35397B: {
		ID:               SFQwen35397B,
		Name:             "Qwen3.5-397B (SiliconFlow)",
		Provider:         ProviderSiliconFlow,
		APIModel:         "Qwen/Qwen3.5-397B-A17B",
		ContextWindow:    131072,
		DefaultMaxTokens: 16384,
	},
	SFQwen35122B: {
		ID:               SFQwen35122B,
		Name:             "Qwen3.5-122B (SiliconFlow)",
		Provider:         ProviderSiliconFlow,
		APIModel:         "Qwen/Qwen3.5-122B-A10B",
		ContextWindow:    131072,
		DefaultMaxTokens: 16384,
	},
	SFQwen3527B: {
		ID:               SFQwen3527B,
		Name:             "Qwen3.5-27B (SiliconFlow)",
		Provider:         ProviderSiliconFlow,
		APIModel:         "Qwen/Qwen3.5-27B",
		ContextWindow:    131072,
		DefaultMaxTokens: 16384,
	},
})

var supportedModelOrder = []ModelID{
	Claude4Sonnet,
	Claude4Opus,
	Claude37Haiku,
	Claude47Opus,
	Claude46Sonnet,
	Claude45Haiku,
	Claude45HaikuAlias,
	GPT55,
	GPT54,
	GPT54Mini,
	GPT41,
	GPT41Mini,
	O4Mini,
	DeepSeekV4Flash,
	DeepSeekV4Pro,
	DeepSeekChat,
	DeepSeekR1,
	MiniMaxM27,
	MiniMaxM27HighSpeed,
	MiniMaxM25,
	MiniMaxM25HighSpeed,
	GLM5,
	GLM5Turbo,
	GLM47,
	GLM46,
	SFDeepSeekV3,
	SFDeepSeekR1,
	SFQwen35397B,
	SFQwen35122B,
	SFQwen3527B,
}

func withCuratedMetadata(models map[ModelID]Model) map[ModelID]Model {
	for id, model := range models {
		if model.Status == "" {
			model.Status = ModelStatusCurrent
		}
		if model.MetadataState == "" {
			if model.CostPer1MIn != 0 || model.CostPer1MOut != 0 ||
				model.CostPer1MInCached != 0 || model.CostPer1MOutCached != 0 {
				model.MetadataState = ModelMetadataKnown
				model.CostKnown = true
			} else {
				model.MetadataState = ModelMetadataPartial
			}
		}
		models[id] = model
	}
	return models
}

// OrderedSupportedModels returns curated models in deterministic display order.
func OrderedSupportedModels() []Model {
	ordered := make([]Model, 0, len(supportedModelOrder))
	for _, id := range supportedModelOrder {
		if m, ok := SupportedModels[id]; ok {
			ordered = append(ordered, m)
		}
	}
	return ordered
}

// OrderedModelsByProvider returns curated models for a provider in deterministic order.
func OrderedModelsByProvider(provider ModelProvider) []Model {
	ordered := OrderedSupportedModels()
	result := make([]Model, 0)
	for _, m := range ordered {
		if m.Provider == provider {
			result = append(result, m)
		}
	}
	return result
}

// ModelsByProviderOrdered groups curated models by provider in deterministic order.
func ModelsByProviderOrdered() map[ModelProvider][]Model {
	result := make(map[ModelProvider][]Model)
	for _, m := range OrderedSupportedModels() {
		result[m.Provider] = append(result[m.Provider], m)
	}
	return result
}

// ResolveModel finds a model by alias, ID, name, or fuzzy match (case-insensitive substring).
func ResolveModel(nameOrAlias string) (Model, bool) {
	lower := strings.ToLower(nameOrAlias)

	// 1. Alias lookup (highest priority)
	if id, ok := ModelAliases[lower]; ok {
		if m, ok := SupportedModels[id]; ok {
			return m, true
		}
	}

	// 2. Exact ID match
	if m, ok := SupportedModels[ModelID(nameOrAlias)]; ok {
		return m, true
	}

	// Exact name match (case-insensitive)
	for _, m := range SupportedModels {
		if strings.ToLower(string(m.ID)) == lower || strings.ToLower(m.Name) == lower {
			return m, true
		}
	}

	// Fuzzy: substring match on name or ID
	var candidates []Model
	for _, m := range SupportedModels {
		if strings.Contains(strings.ToLower(m.Name), lower) ||
			strings.Contains(strings.ToLower(string(m.ID)), lower) {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}

	return Model{}, false
}

// ModelsByProvider returns all supported models grouped by provider.
func ModelsByProvider() map[ModelProvider][]Model {
	return ModelsByProviderOrdered()
}

// ProviderDisplayOrder returns providers in a consistent display order.
func ProviderDisplayOrder() []ModelProvider {
	order := make([]ModelProvider, 0, len(providerCatalog))
	for _, spec := range providerCatalog {
		order = append(order, spec.ID)
	}
	return order
}

// ProviderDisplayName returns a human-friendly name for a provider.
func ProviderDisplayName(p ModelProvider) string {
	if spec, ok := ProviderSpecByID(p); ok {
		return spec.DisplayName
	}
	return string(p)
}

// CheapestModel returns the cheapest model ID for a given provider (for validation).
func CheapestModel(provider ModelProvider) ModelID {
	switch provider {
	case ProviderAnthropic:
		return Claude37Haiku
	case ProviderOpenAI:
		return GPT41Mini
	case ProviderDeepSeek:
		return DeepSeekChat
	case ProviderMiniMax:
		return MiniMaxM25HighSpeed
	case ProviderGLM:
		return GLM46
	case ProviderSiliconFlow:
		return SFQwen3527B
	default:
		return ""
	}
}

// ResolveProvider finds a provider by name (case-insensitive, supports abbreviations).
func ResolveProvider(name string) (ModelProvider, bool) {
	lower := strings.ToLower(name)
	for _, p := range ProviderDisplayOrder() {
		if strings.ToLower(string(p)) == lower || strings.ToLower(ProviderDisplayName(p)) == lower {
			return p, true
		}
	}
	// Prefix match
	for _, p := range ProviderDisplayOrder() {
		if strings.HasPrefix(strings.ToLower(string(p)), lower) ||
			strings.HasPrefix(strings.ToLower(ProviderDisplayName(p)), lower) {
			return p, true
		}
	}
	return "", false
}

func NewCustomModel(provider ModelProvider, apiModel string) Model {
	return Model{
		ID:               ModelID(apiModel),
		Name:             apiModel,
		Provider:         provider,
		APIModel:         apiModel,
		MetadataState:    ModelMetadataUnknown,
		DefaultMaxTokens: 16384,
		Status:           ModelStatusCurrent,
	}
}

type ModelOption struct {
	Model   Model
	Source  ModelSource
	Warning string
}

func MergeModelOptions(curated []Model, configured []Model, discovered []Model) []ModelOption {
	seen := make(map[ModelID]struct{}, len(curated)+len(configured)+len(discovered))
	out := make([]ModelOption, 0, len(curated)+len(configured)+len(discovered))
	appendSet := func(models []Model, source ModelSource) {
		for _, m := range models {
			if m.ID == "" {
				continue
			}
			if _, ok := seen[m.ID]; ok {
				continue
			}
			seen[m.ID] = struct{}{}
			out = append(out, ModelOption{Model: m, Source: source})
		}
	}
	appendSet(curated, ModelSourceCurated)
	appendSet(configured, ModelSourceCurrentConfig)
	appendSet(discovered, ModelSourceProviderList)
	return out
}
