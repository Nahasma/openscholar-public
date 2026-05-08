package models

type AuthMode string

const (
	AuthRequired AuthMode = "required"
	AuthOptional AuthMode = "optional"
	AuthNone     AuthMode = "none"
)

type ProviderKind string

const (
	ProviderKindNativeAnthropic ProviderKind = "native_anthropic"
	ProviderKindNativeOpenAI    ProviderKind = "native_openai"
	ProviderKindAnthropicCompat ProviderKind = "anthropic_compatible"
	ProviderKindOpenAICompat    ProviderKind = "openai_compatible"
	ProviderKindLocal           ProviderKind = "local"
	ProviderKindRouter          ProviderKind = "router"
)

type EndpointSpec struct {
	DefaultBaseURL string
	ListPath       string
	ChatPath       string
	RequiresV1     bool
}

type ModelCapabilities struct {
	Tools     bool
	Reasoning bool
	Vision    bool
	Local     bool
}

type ProviderSpec struct {
	ID             ModelProvider
	DisplayName    string
	Kind           ProviderKind
	AuthMode       AuthMode
	DefaultModel   string
	AllowArbitrary bool
	APIKeyEnvVars  []string
	BaseURLEnvVars []string
	Endpoints      EndpointSpec
	SupportsList   bool
	SupportsPing   bool
	IsRouter       bool
	Capabilities   ModelCapabilities
	Priority       int
}

var providerCatalog = []ProviderSpec{
	{
		ID: ProviderAnthropic, DisplayName: "Anthropic", Kind: ProviderKindNativeAnthropic,
		AuthMode: AuthRequired, DefaultModel: string(Claude4Sonnet),
		APIKeyEnvVars: []string{"ANTHROPIC_API_KEY"},
		Endpoints:     EndpointSpec{DefaultBaseURL: "https://api.anthropic.com", ListPath: "/v1/models", ChatPath: "/v1/messages", RequiresV1: true},
		SupportsList:  true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true, Reasoning: true, Vision: true},
		Priority:     10,
	},
	{
		ID: ProviderOpenAI, DisplayName: "OpenAI", Kind: ProviderKindNativeOpenAI,
		AuthMode: AuthRequired, DefaultModel: string(GPT41),
		APIKeyEnvVars: []string{"OPENAI_API_KEY"},
		Endpoints:     EndpointSpec{DefaultBaseURL: "https://api.openai.com/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:  true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true, Reasoning: true, Vision: true},
		Priority:     20,
	},
	{
		ID: ProviderDeepSeek, DisplayName: "DeepSeek", Kind: ProviderKindOpenAICompat,
		AuthMode: AuthRequired, DefaultModel: string(DeepSeekChat),
		APIKeyEnvVars: []string{"DEEPSEEK_API_KEY"},
		Endpoints:     EndpointSpec{DefaultBaseURL: "https://api.deepseek.com", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:  true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true, Reasoning: true},
		Priority:     30,
	},
	{
		ID: ProviderMiniMax, DisplayName: "MiniMax", Kind: ProviderKindAnthropicCompat,
		AuthMode: AuthRequired, DefaultModel: string(MiniMaxM27),
		APIKeyEnvVars: []string{"MINIMAX_API_KEY"},
		Endpoints:     EndpointSpec{DefaultBaseURL: "https://api.minimaxi.com/anthropic", ListPath: "/v1/models", ChatPath: "/v1/messages", RequiresV1: true},
		SupportsList:  true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true, Reasoning: true},
		Priority:     40,
	},
	{
		ID: ProviderGLM, DisplayName: "GLM (智谱)", Kind: ProviderKindOpenAICompat,
		AuthMode: AuthRequired, DefaultModel: string(GLM5),
		APIKeyEnvVars: []string{"GLM_API_KEY", "ZHIPUAI_API_KEY"},
		Endpoints:     EndpointSpec{DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:  true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true, Reasoning: true},
		Priority:     50,
	},
	{
		ID: ProviderSiliconFlow, DisplayName: "SiliconFlow (硅基流动)", Kind: ProviderKindOpenAICompat,
		AuthMode: AuthRequired, DefaultModel: string(SFQwen35397B),
		APIKeyEnvVars: []string{"SILICONFLOW_API_KEY"},
		Endpoints:     EndpointSpec{DefaultBaseURL: "https://api.siliconflow.cn/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:  true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true, Reasoning: true},
		Priority:     60,
	},
	{
		ID: ProviderGroq, DisplayName: "Groq", Kind: ProviderKindOpenAICompat,
		AuthMode: AuthRequired, APIKeyEnvVars: []string{"GROQ_API_KEY"},
		Endpoints:    EndpointSpec{DefaultBaseURL: "https://api.groq.com/openai/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList: true, SupportsPing: true, AllowArbitrary: true,
		Capabilities: ModelCapabilities{Tools: true},
		Priority:     70,
	},
	{
		ID: ProviderTogether, DisplayName: "Together AI", Kind: ProviderKindOpenAICompat,
		AuthMode: AuthRequired, APIKeyEnvVars: []string{"TOGETHER_API_KEY"},
		Endpoints:    EndpointSpec{DefaultBaseURL: "https://api.together.xyz/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList: true, SupportsPing: true, AllowArbitrary: true,
		Capabilities: ModelCapabilities{Tools: true},
		Priority:     80,
	},
	{
		ID: ProviderFireworks, DisplayName: "Fireworks AI", Kind: ProviderKindOpenAICompat,
		AuthMode: AuthRequired, APIKeyEnvVars: []string{"FIREWORKS_API_KEY"},
		Endpoints:    EndpointSpec{DefaultBaseURL: "https://api.fireworks.ai/inference/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList: true, SupportsPing: true, AllowArbitrary: true,
		Capabilities: ModelCapabilities{Tools: true},
		Priority:     90,
	},
	{
		ID: ProviderMistral, DisplayName: "Mistral AI", Kind: ProviderKindOpenAICompat,
		AuthMode: AuthRequired, APIKeyEnvVars: []string{"MISTRAL_API_KEY"},
		Endpoints:    EndpointSpec{DefaultBaseURL: "https://api.mistral.ai/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList: true, SupportsPing: true, AllowArbitrary: true,
		Capabilities: ModelCapabilities{Tools: true},
		Priority:     100,
	},
	{
		ID: ProviderMoonshot, DisplayName: "Moonshot AI", Kind: ProviderKindOpenAICompat,
		AuthMode: AuthRequired, APIKeyEnvVars: []string{"MOONSHOT_API_KEY", "KIMI_API_KEY"},
		Endpoints:    EndpointSpec{DefaultBaseURL: "https://api.moonshot.cn/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList: true, SupportsPing: true, AllowArbitrary: true,
		Capabilities: ModelCapabilities{Tools: true},
		Priority:     110,
	},
	{
		ID: ProviderDashScope, DisplayName: "DashScope (通义千问)", Kind: ProviderKindOpenAICompat,
		AuthMode: AuthRequired, APIKeyEnvVars: []string{"DASHSCOPE_API_KEY"},
		Endpoints:    EndpointSpec{DefaultBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList: true, SupportsPing: true, AllowArbitrary: true,
		Capabilities: ModelCapabilities{Tools: true, Vision: true},
		Priority:     120,
	},
	{
		ID: ProviderXAI, DisplayName: "xAI", Kind: ProviderKindOpenAICompat,
		AuthMode: AuthRequired, APIKeyEnvVars: []string{"XAI_API_KEY"},
		Endpoints:    EndpointSpec{DefaultBaseURL: "https://api.x.ai/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList: true, SupportsPing: true, AllowArbitrary: true,
		Capabilities: ModelCapabilities{Tools: true, Reasoning: true},
		Priority:     130,
	},
	{
		ID: ProviderPerplexity, DisplayName: "Perplexity", Kind: ProviderKindOpenAICompat,
		AuthMode: AuthRequired, APIKeyEnvVars: []string{"PERPLEXITY_API_KEY"},
		Endpoints:    EndpointSpec{DefaultBaseURL: "https://api.perplexity.ai", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList: true, SupportsPing: true, AllowArbitrary: true,
		Capabilities: ModelCapabilities{Tools: true},
		Priority:     140,
	},
	{
		ID: ProviderOllama, DisplayName: "Ollama (Local)", Kind: ProviderKindLocal,
		AuthMode: AuthNone, AllowArbitrary: true,
		BaseURLEnvVars: []string{"OLLAMA_BASE_URL"},
		Endpoints:      EndpointSpec{DefaultBaseURL: "http://localhost:11434/v1", ListPath: "/api/tags", ChatPath: "/chat/completions", RequiresV1: false},
		SupportsList:   true, SupportsPing: true,
		Capabilities: ModelCapabilities{Local: true},
		Priority:     200,
	},
	{
		ID: ProviderVLLM, DisplayName: "vLLM (Local)", Kind: ProviderKindLocal,
		AuthMode: AuthNone, AllowArbitrary: true,
		BaseURLEnvVars: []string{"VLLM_BASE_URL"},
		Endpoints:      EndpointSpec{DefaultBaseURL: "http://localhost:8000/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:   true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true, Local: true},
		Priority:     210,
	},
	{
		ID: ProviderLMStudio, DisplayName: "LM Studio (Local)", Kind: ProviderKindLocal,
		AuthMode: AuthNone, AllowArbitrary: true,
		BaseURLEnvVars: []string{"LMSTUDIO_BASE_URL", "LM_STUDIO_BASE_URL"},
		Endpoints:      EndpointSpec{DefaultBaseURL: "http://localhost:1234/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:   true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true, Local: true},
		Priority:     220,
	},
	{
		ID: ProviderLocalAI, DisplayName: "LocalAI", Kind: ProviderKindLocal,
		AuthMode: AuthOptional, AllowArbitrary: true,
		APIKeyEnvVars:  []string{"LOCALAI_API_KEY"},
		BaseURLEnvVars: []string{"LOCALAI_BASE_URL"},
		Endpoints:      EndpointSpec{DefaultBaseURL: "http://localhost:8080/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:   true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true, Local: true},
		Priority:     230,
	},
	{
		ID: ProviderLlamaCPP, DisplayName: "llama.cpp Server", Kind: ProviderKindLocal,
		AuthMode: AuthNone, AllowArbitrary: true,
		BaseURLEnvVars: []string{"LLAMACPP_BASE_URL", "LLAMA_CPP_BASE_URL"},
		Endpoints:      EndpointSpec{DefaultBaseURL: "http://localhost:8080/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:   true, SupportsPing: true,
		Capabilities: ModelCapabilities{Local: true},
		Priority:     240,
	},
	{
		ID: ProviderOpenRouter, DisplayName: "OpenRouter", Kind: ProviderKindRouter,
		AuthMode: AuthRequired, AllowArbitrary: true, IsRouter: true,
		APIKeyEnvVars: []string{"OPENROUTER_API_KEY"},
		Endpoints:     EndpointSpec{DefaultBaseURL: "https://openrouter.ai/api/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:  true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true},
		Priority:     300,
	},
	{
		ID: ProviderLiteLLM, DisplayName: "LiteLLM", Kind: ProviderKindRouter,
		AuthMode: AuthOptional, AllowArbitrary: true, IsRouter: true,
		APIKeyEnvVars:  []string{"LITELLM_API_KEY"},
		BaseURLEnvVars: []string{"LITELLM_BASE_URL"},
		Endpoints:      EndpointSpec{DefaultBaseURL: "http://localhost:4000/v1", ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:   true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true},
		Priority:     310,
	},
	{
		ID: ProviderRouter, DisplayName: "Custom Router", Kind: ProviderKindRouter,
		AuthMode: AuthOptional, AllowArbitrary: true, IsRouter: true,
		APIKeyEnvVars:  []string{"ROUTER_API_KEY"},
		BaseURLEnvVars: []string{"ROUTER_BASE_URL"},
		Endpoints:      EndpointSpec{ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:   true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true},
		Priority:     320,
	},
	{
		ID: ProviderGateway, DisplayName: "Custom Gateway", Kind: ProviderKindRouter,
		AuthMode: AuthOptional, AllowArbitrary: true, IsRouter: true,
		APIKeyEnvVars:  []string{"GATEWAY_API_KEY"},
		BaseURLEnvVars: []string{"GATEWAY_BASE_URL"},
		Endpoints:      EndpointSpec{ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:   true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true},
		Priority:     330,
	},
	{
		ID: ProviderOpenAICompatible, DisplayName: "OpenAI Compatible", Kind: ProviderKindOpenAICompat,
		AuthMode: AuthOptional, AllowArbitrary: true,
		APIKeyEnvVars:  []string{"OPENAI_COMPATIBLE_API_KEY"},
		BaseURLEnvVars: []string{"OPENAI_COMPATIBLE_BASE_URL"},
		Endpoints:      EndpointSpec{ListPath: "/models", ChatPath: "/chat/completions", RequiresV1: true},
		SupportsList:   true, SupportsPing: true,
		Capabilities: ModelCapabilities{Tools: true},
		Priority:     900,
	},
}

func ProviderCatalog() []ProviderSpec {
	out := make([]ProviderSpec, len(providerCatalog))
	copy(out, providerCatalog)
	for i := range out {
		out[i].APIKeyEnvVars = copyStrings(out[i].APIKeyEnvVars)
		out[i].BaseURLEnvVars = copyStrings(out[i].BaseURLEnvVars)
	}
	return out
}

func ProviderSpecByID(id ModelProvider) (ProviderSpec, bool) {
	for _, spec := range providerCatalog {
		if spec.ID == id {
			spec.APIKeyEnvVars = copyStrings(spec.APIKeyEnvVars)
			spec.BaseURLEnvVars = copyStrings(spec.BaseURLEnvVars)
			return spec, true
		}
	}
	return ProviderSpec{}, false
}

func ProviderRequiresAPIKey(id ModelProvider) bool {
	spec, ok := ProviderSpecByID(id)
	return ok && spec.AuthMode == AuthRequired
}

func ProviderAuthMode(id ModelProvider) AuthMode {
	spec, ok := ProviderSpecByID(id)
	if !ok {
		return AuthRequired
	}
	return spec.AuthMode
}

func ProviderAllowsArbitraryModel(id ModelProvider) bool {
	spec, ok := ProviderSpecByID(id)
	return ok && spec.AllowArbitrary
}

func ProviderDefaultBaseURL(id ModelProvider) string {
	spec, ok := ProviderSpecByID(id)
	if !ok {
		return ""
	}
	return spec.Endpoints.DefaultBaseURL
}

func ProviderRequiresBaseURL(id ModelProvider) bool {
	spec, ok := ProviderSpecByID(id)
	if !ok || spec.Endpoints.DefaultBaseURL != "" {
		return false
	}
	switch spec.Kind {
	case ProviderKindOpenAICompat, ProviderKindLocal, ProviderKindRouter, ProviderKindAnthropicCompat:
		return true
	default:
		return false
	}
}

func ProviderAPIKeyEnvVars(id ModelProvider) []string {
	spec, ok := ProviderSpecByID(id)
	if !ok || len(spec.APIKeyEnvVars) == 0 {
		return nil
	}
	return copyStrings(spec.APIKeyEnvVars)
}

func ProviderBaseURLEnvVars(id ModelProvider) []string {
	spec, ok := ProviderSpecByID(id)
	if !ok || len(spec.BaseURLEnvVars) == 0 {
		return nil
	}
	return copyStrings(spec.BaseURLEnvVars)
}

func ProviderKindOf(id ModelProvider) ProviderKind {
	spec, ok := ProviderSpecByID(id)
	if !ok {
		return ""
	}
	return spec.Kind
}

func ProviderSupportsList(id ModelProvider) bool {
	spec, ok := ProviderSpecByID(id)
	return ok && spec.SupportsList
}

func ProviderSupportsPing(id ModelProvider) bool {
	spec, ok := ProviderSpecByID(id)
	return ok && spec.SupportsPing
}

func ProviderIsRouter(id ModelProvider) bool {
	spec, ok := ProviderSpecByID(id)
	return ok && spec.IsRouter
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
