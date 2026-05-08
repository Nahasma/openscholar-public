package models

import (
	"testing"
)

// TestRoutingSmallFastModelForProvider verifies the provider → lightweight model mapping.
func TestRoutingSmallFastModelForProvider(t *testing.T) {
	cases := []struct {
		provider ModelProvider
		want     ModelID
	}{
		{ProviderAnthropic, Claude37Haiku},
		{ProviderOpenAI, GPT41Mini},
		{ProviderDeepSeek, DeepSeekChat},
		{ProviderMiniMax, MiniMaxM27},
		{ProviderGLM, GLM5},
		{ProviderSiliconFlow, SFDeepSeekV3},
		{ProviderOllama, ""},
		{ProviderVLLM, ""},
		{ProviderOpenAICompatible, ""},
		{ModelProvider("unknown"), ""},
	}

	for _, tc := range cases {
		got := SmallFastModelForProvider(tc.provider)
		if got != tc.want {
			t.Errorf("SmallFastModelForProvider(%q) = %q, want %q", tc.provider, got, tc.want)
		}
	}
}

// TestRoutingIsLightweightAgent verifies lightweight agent detection.
func TestRoutingIsLightweightAgent(t *testing.T) {
	lightweight := []string{"task", "title", "explore"}
	for _, name := range lightweight {
		if !IsLightweightAgent(name) {
			t.Errorf("IsLightweightAgent(%q) = false, want true", name)
		}
	}

	heavy := []string{"main", "leader", "worker", "researcher", "scholar", ""}
	for _, name := range heavy {
		if IsLightweightAgent(name) {
			t.Errorf("IsLightweightAgent(%q) = true, want false", name)
		}
	}
}

// TestRoutingResolveWithFallback_KnownModel verifies that known models resolve without fallback.
func TestRoutingResolveWithFallback_KnownModel(t *testing.T) {
	res, err := ResolveWithFallback(ProviderAnthropic, string(Claude4Sonnet))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsFallback {
		t.Error("IsFallback should be false for a known model")
	}
	if res.Warning != "" {
		t.Errorf("Warning should be empty for known model, got %q", res.Warning)
	}
	if res.Model.ID != Claude4Sonnet {
		t.Errorf("Model.ID = %q, want %q", res.Model.ID, Claude4Sonnet)
	}
	if res.RequestedID != string(Claude4Sonnet) {
		t.Errorf("RequestedID = %q, want %q", res.RequestedID, Claude4Sonnet)
	}
}

func TestRoutingResolveWithFallback_CrossProviderExactIDUsesRequestedProvider(t *testing.T) {
	res, err := ResolveWithFallback(ProviderOpenAI, string(Claude4Sonnet))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsFallback {
		t.Fatal("cross-provider exact ID should resolve as fallback for requested provider")
	}
	if res.Model.Provider != ProviderOpenAI {
		t.Fatalf("Model.Provider = %q, want %q", res.Model.Provider, ProviderOpenAI)
	}
	if res.Model.ID != Claude4Sonnet {
		t.Fatalf("Model.ID = %q, want %q", res.Model.ID, Claude4Sonnet)
	}
}

// TestRoutingResolveWithFallback_CustomProvider verifies that custom providers accept unknown models.
func TestRoutingResolveWithFallback_CustomProvider(t *testing.T) {
	customProviderList := []ModelProvider{
		ProviderOpenAICompatible,
		ProviderOllama,
		ProviderVLLM,
	}
	for _, p := range customProviderList {
		res, err := ResolveWithFallback(p, "my-custom-model-v1")
		if err != nil {
			t.Fatalf("provider %q: unexpected error: %v", p, err)
		}
		if !res.IsFallback {
			t.Errorf("provider %q: IsFallback should be true for unknown model", p)
		}
		if res.Warning == "" {
			t.Errorf("provider %q: Warning should be non-empty for fallback", p)
		}
		// Cost and context must remain zero — no fabricated values.
		if res.Model.CostPer1MIn != 0 || res.Model.CostPer1MOut != 0 {
			t.Errorf("provider %q: cost fields should be zero for custom model", p)
		}
		if res.Model.ContextWindow != 0 {
			t.Errorf("provider %q: stored ContextWindow should remain zero for custom model", p)
		}
		if RuntimeContextWindow(res.Model) <= 0 {
			t.Errorf("provider %q: RuntimeContextWindow should be non-zero", p)
		}
		if res.Model.Provider != p {
			t.Errorf("provider %q: Model.Provider = %q, want same provider", p, res.Model.Provider)
		}
	}
}

// TestRoutingResolveWithFallback_StrictProviderUnknown verifies that strict providers degrade gracefully.
func TestRoutingResolveWithFallback_StrictProviderUnknown(t *testing.T) {
	res, err := ResolveWithFallback(ProviderAnthropic, "claude-future-9000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsFallback {
		t.Error("IsFallback should be true for unknown model on strict provider")
	}
	if res.Warning == "" {
		t.Error("Warning should be non-empty for unknown model on strict provider")
	}
	// No fabricated metadata.
	if res.Model.CostPer1MIn != 0 || res.Model.CostPer1MOut != 0 {
		t.Error("cost fields should be zero for unknown strict-provider model")
	}
	if res.Model.ContextWindow != 0 {
		t.Error("stored ContextWindow should remain zero for unknown strict-provider model")
	}
	if RuntimeContextWindow(res.Model) <= 0 {
		t.Error("RuntimeContextWindow should be non-zero for unknown strict-provider model")
	}
}

// TestRoutingResolveWithFallback_EmptyID verifies that an empty model ID returns an error.
func TestRoutingResolveWithFallback_EmptyID(t *testing.T) {
	_, err := ResolveWithFallback(ProviderAnthropic, "")
	if err == nil {
		t.Error("expected error for empty model ID, got nil")
	}
}

// TestRoutingResolveWithFallback_NoFabricatedCosts verifies zero values across providers.
func TestRoutingResolveWithFallback_NoFabricatedCosts(t *testing.T) {
	cases := []struct {
		provider  ModelProvider
		modelName string
	}{
		{ProviderOpenAI, "gpt-99-turbo"},
		{ProviderDeepSeek, "deepseek-unknown"},
		{ProviderMiniMax, "minimax-future"},
		{ProviderGLM, "glm-99"},
		{ProviderSiliconFlow, "some/unknown-model"},
	}

	for _, tc := range cases {
		res, err := ResolveWithFallback(tc.provider, tc.modelName)
		if err != nil {
			t.Fatalf("provider %q model %q: unexpected error: %v", tc.provider, tc.modelName, err)
		}
		if !res.IsFallback {
			t.Errorf("provider %q model %q: expected IsFallback=true", tc.provider, tc.modelName)
		}
		if res.Model.CostPer1MIn != 0 || res.Model.CostPer1MOut != 0 ||
			res.Model.CostPer1MInCached != 0 || res.Model.CostPer1MOutCached != 0 {
			t.Errorf("provider %q model %q: cost fields must be zero, got in=%.4f out=%.4f",
				tc.provider, tc.modelName, res.Model.CostPer1MIn, res.Model.CostPer1MOut)
		}
		if res.Model.ContextWindow != 0 {
			t.Errorf("provider %q model %q: stored ContextWindow must be zero, got %d",
				tc.provider, tc.modelName, res.Model.ContextWindow)
		}
		if RuntimeContextWindow(res.Model) <= 0 {
			t.Errorf("provider %q model %q: RuntimeContextWindow must be non-zero",
				tc.provider, tc.modelName)
		}
	}
}
