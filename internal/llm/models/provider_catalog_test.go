package models

import "testing"

func TestProviderCatalog_OrderAndFacts(t *testing.T) {
	c := ProviderCatalog()
	if len(c) < 9 {
		t.Fatalf("expected at least 9 providers, got %d", len(c))
	}
	if c[0].ID != ProviderAnthropic {
		t.Fatalf("unexpected first provider: %s", c[0].ID)
	}
	if !ProviderRequiresAPIKey(ProviderOpenAI) {
		t.Fatal("openai should require API key")
	}
	if ProviderRequiresAPIKey(ProviderOllama) {
		t.Fatal("ollama should not require API key")
	}
	if !ProviderAllowsArbitraryModel(ProviderOpenAICompatible) {
		t.Fatal("openai_compatible should allow arbitrary model")
	}
	if ProviderAuthMode(ProviderOpenAICompatible) != AuthOptional {
		t.Fatal("openai_compatible auth should be optional")
	}
	if ProviderKindOf(ProviderOpenRouter) != ProviderKindRouter {
		t.Fatal("openrouter should be a router provider")
	}
	if !ProviderIsRouter(ProviderOpenRouter) {
		t.Fatal("openrouter should be marked as router")
	}
	if ProviderDefaultBaseURL(ProviderGroq) != "https://api.groq.com/openai/v1" {
		t.Fatalf("unexpected groq base URL: %q", ProviderDefaultBaseURL(ProviderGroq))
	}
	minimax, ok := ProviderSpecByID(ProviderMiniMax)
	if !ok {
		t.Fatal("minimax provider missing")
	}
	if !minimax.SupportsList || minimax.Endpoints.ListPath != "/v1/models" {
		t.Fatalf("minimax list endpoint not configured: %+v", minimax.Endpoints)
	}
	if minimax.Endpoints.DefaultBaseURL != "https://api.minimaxi.com/anthropic" {
		t.Fatalf("unexpected minimax base URL: %q", minimax.Endpoints.DefaultBaseURL)
	}
	if ProviderRequiresBaseURL(ProviderOpenAICompatible) != true {
		t.Fatal("openai_compatible should require configured base URL")
	}
	if ProviderRequiresBaseURL(ProviderOllama) {
		t.Fatal("ollama has a default base URL and should not require one")
	}
}

func TestProviderCatalog_ReturnsIndependentEnvSlices(t *testing.T) {
	specs := ProviderCatalog()
	specs[0].APIKeyEnvVars[0] = "MUTATED"

	envVars := ProviderAPIKeyEnvVars(ProviderAnthropic)
	if len(envVars) == 0 || envVars[0] != "ANTHROPIC_API_KEY" {
		t.Fatalf("catalog env vars were mutated: %#v", envVars)
	}
}
