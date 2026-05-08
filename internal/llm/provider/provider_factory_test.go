package provider

import (
	"testing"

	"github.com/Nahasma/openscholar-public/internal/llm/minimax"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

func TestNewProvider_UsesCatalogDefaultBaseURLForOpenAICompatibleKind(t *testing.T) {
	p, err := NewProvider(
		models.ProviderGroq,
		WithModel(models.NewCustomModel(models.ProviderGroq, "llama-3.3-70b-versatile")),
	)
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}

	bp, ok := p.(*baseProvider[*openaiClient])
	if !ok {
		t.Fatalf("provider type = %T, want openai base provider", p)
	}
	if bp.options.baseURL != models.ProviderDefaultBaseURL(models.ProviderGroq) {
		t.Fatalf("baseURL = %q, want %q", bp.options.baseURL, models.ProviderDefaultBaseURL(models.ProviderGroq))
	}
}

func TestNewProvider_RouterUsesOpenAICompatibleAdapter(t *testing.T) {
	p, err := NewProvider(
		models.ProviderOpenRouter,
		WithModel(models.NewCustomModel(models.ProviderOpenRouter, "anthropic/claude-sonnet-4")),
	)
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}
	if _, ok := p.(*baseProvider[*openaiClient]); !ok {
		t.Fatalf("provider type = %T, want openai base provider", p)
	}
}

func TestNewProvider_CustomRouterRequiresBaseURL(t *testing.T) {
	_, err := NewProvider(
		models.ProviderRouter,
		WithModel(models.NewCustomModel(models.ProviderRouter, "provider/model")),
	)
	if err == nil {
		t.Fatal("expected missing base URL error")
	}
}

func TestNewProvider_AnthropicCompatibleUsesAnthropicAdapter(t *testing.T) {
	p, err := NewProvider(
		models.ProviderMiniMax,
		WithModel(models.SupportedModels[models.MiniMaxM27]),
	)
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}
	if _, ok := p.(*baseProvider[*anthropicClient]); !ok {
		t.Fatalf("provider type = %T, want anthropic base provider", p)
	}
}

func TestNewProvider_MiniMaxProfileLegacy(t *testing.T) {
	p, err := NewProvider(
		models.ProviderMiniMax,
		WithProviderProfile("legacy"),
		WithModel(models.SupportedModels[models.MiniMaxM27]),
	)
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}
	bp := p.(*baseProvider[*anthropicClient])
	if bp.options.baseURL != minimax.LegacyBaseURL {
		t.Fatalf("baseURL = %q, want %q", bp.options.baseURL, minimax.LegacyBaseURL)
	}
}

func TestNewProvider_MiniMaxProfileTokenPlan(t *testing.T) {
	p, err := NewProvider(
		models.ProviderMiniMax,
		WithProviderProfile("token-plan"),
		WithModel(models.SupportedModels[models.MiniMaxM27]),
	)
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}
	bp := p.(*baseProvider[*anthropicClient])
	if bp.options.baseURL != minimax.TokenPlanBaseURL {
		t.Fatalf("baseURL = %q, want %q", bp.options.baseURL, minimax.TokenPlanBaseURL)
	}
}

func TestNewProvider_MiniMaxCustomProfileRequiresBaseURL(t *testing.T) {
	_, err := NewProvider(
		models.ProviderMiniMax,
		WithProviderProfile("custom"),
		WithModel(models.SupportedModels[models.MiniMaxM27]),
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewProvider_MiniMaxUnknownProfileErrors(t *testing.T) {
	_, err := NewProvider(
		models.ProviderMiniMax,
		WithProviderProfile("mystery"),
		WithModel(models.SupportedModels[models.MiniMaxM27]),
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewProvider_MiniMaxExplicitBaseURLHasPriority(t *testing.T) {
	const customBaseURL = "https://example.com/anthropic"
	p, err := NewProvider(
		models.ProviderMiniMax,
		WithProviderProfile("legacy"),
		WithBaseURL(customBaseURL),
		WithModel(models.SupportedModels[models.MiniMaxM27]),
	)
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}
	bp := p.(*baseProvider[*anthropicClient])
	if bp.options.baseURL != customBaseURL {
		t.Fatalf("baseURL = %q, want %q", bp.options.baseURL, customBaseURL)
	}
}

func TestNewProvider_MiniMaxDefaultUsesCatalogBaseURLAndBearer(t *testing.T) {
	p, err := NewProvider(
		models.ProviderMiniMax,
		WithModel(models.SupportedModels[models.MiniMaxM27]),
	)
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}
	bp := p.(*baseProvider[*anthropicClient])
	if bp.options.baseURL != models.ProviderDefaultBaseURL(models.ProviderMiniMax) {
		t.Fatalf("baseURL = %q, want catalog default %q", bp.options.baseURL, models.ProviderDefaultBaseURL(models.ProviderMiniMax))
	}
	if bp.options.effectiveAuth != "bearer" {
		t.Fatalf("effective auth = %q, want bearer", bp.options.effectiveAuth)
	}
}

func TestNewProvider_MiniMaxUnknownAuthModeErrors(t *testing.T) {
	_, err := NewProvider(
		models.ProviderMiniMax,
		WithProviderAuthMode("mystery"),
		WithModel(models.SupportedModels[models.MiniMaxM27]),
	)
	if err == nil {
		t.Fatal("expected error")
	}
}
