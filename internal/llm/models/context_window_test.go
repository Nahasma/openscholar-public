package models

import "testing"

func TestRuntimeContextWindowUsesKnownOpenAIModelWindows(t *testing.T) {
	if got := RuntimeContextWindow(SupportedModels[GPT41]); got != 1047576 {
		t.Fatalf("GPT-4.1 context window=%d, want 1047576", got)
	}
	if got := RuntimeContextWindow(SupportedModels[GPT41Mini]); got != 1047576 {
		t.Fatalf("GPT-4.1 mini context window=%d, want 1047576", got)
	}
	if got := SupportedModels[GPT41Mini].DefaultMaxTokens; got != 32768 {
		t.Fatalf("GPT-4.1 mini max output=%d, want 32768", got)
	}
	if got := SupportedModels[O4Mini].DefaultMaxTokens; got != 100000 {
		t.Fatalf("o4-mini max output=%d, want 100000", got)
	}
}

func TestRuntimeContextWindowInfersGPT5Window(t *testing.T) {
	model := NewCustomModel(ProviderOpenAI, "gpt-5.4")
	if got := RuntimeContextWindow(model); got != 1050000 {
		t.Fatalf("GPT-5 inferred context window=%d, want 1050000", got)
	}
}

func TestOpenAICachedInputPricesAreConfigured(t *testing.T) {
	tests := []struct {
		id   ModelID
		want float64
	}{
		{id: GPT41, want: 0.50},
		{id: GPT41Mini, want: 0.10},
		{id: O4Mini, want: 0.275},
	}

	for _, tt := range tests {
		if got := SupportedModels[tt.id].CostPer1MOutCached; got != tt.want {
			t.Fatalf("%s cached input price=%v, want %v", tt.id, got, tt.want)
		}
	}
}
