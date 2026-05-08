package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderedSupportedModels_CompletenessAndUniqueness(t *testing.T) {
	ordered := OrderedSupportedModels()
	require.NotEmpty(t, ordered)

	seen := make(map[ModelID]bool, len(ordered))
	for _, m := range ordered {
		require.NotEmpty(t, m.ID)
		require.Falsef(t, seen[m.ID], "duplicate model id in ordered list: %s", m.ID)
		seen[m.ID] = true
		_, ok := SupportedModels[m.ID]
		require.Truef(t, ok, "ordered model missing from supported map: %s", m.ID)
	}

	for id := range SupportedModels {
		require.Truef(t, seen[id], "supported model missing from ordered list: %s", id)
	}
}

func TestOrderedModelsByProvider_MiniMaxOrderStable(t *testing.T) {
	first := OrderedModelsByProvider(ProviderMiniMax)
	require.GreaterOrEqual(t, len(first), 4)
	expected := []ModelID{MiniMaxM27, MiniMaxM27HighSpeed, MiniMaxM25, MiniMaxM25HighSpeed}
	for i, id := range expected {
		assert.Equal(t, id, first[i].ID)
	}

	for i := 0; i < 20; i++ {
		next := OrderedModelsByProvider(ProviderMiniMax)
		require.Equal(t, len(first), len(next))
		for idx := range first {
			assert.Equal(t, first[idx].ID, next[idx].ID)
		}
	}
}

func TestSupportedModels_AllHaveRequiredFields(t *testing.T) {
	for id, model := range SupportedModels {
		assert.NotEmpty(t, model.Name, "model %s has empty name", id)
		assert.NotEmpty(t, model.Provider, "model %s has empty provider", id)
		assert.NotEmpty(t, model.APIModel, "model %s has empty api_model", id)
		assert.Greater(t, model.ContextWindow, int64(0), "model %s has zero context window", id)
		assert.Greater(t, model.DefaultMaxTokens, int64(0), "model %s has zero max tokens", id)
	}
}

func TestResolveModel_ExactMatch(t *testing.T) {
	model, ok := ResolveModel(string(Claude4Sonnet))
	require.True(t, ok)
	assert.Equal(t, Claude4Sonnet, model.ID)
	assert.Equal(t, ProviderAnthropic, model.Provider)
}

func TestResolveModel_NameMatch(t *testing.T) {
	model, ok := ResolveModel("Claude Sonnet 4")
	require.True(t, ok)
	assert.Equal(t, Claude4Sonnet, model.ID)
}

func TestResolveModel_CaseInsensitive(t *testing.T) {
	model, ok := ResolveModel("claude sonnet 4")
	require.True(t, ok)
	assert.Equal(t, Claude4Sonnet, model.ID)
}

func TestResolveModel_NotFound(t *testing.T) {
	_, ok := ResolveModel("nonexistent-model")
	assert.False(t, ok)
}

func TestModelsByProvider(t *testing.T) {
	grouped := ModelsByProvider()
	assert.NotEmpty(t, grouped)

	// Anthropic should have at least 2 models
	anthropicModels := grouped[ProviderAnthropic]
	assert.GreaterOrEqual(t, len(anthropicModels), 2)
	for _, m := range anthropicModels {
		assert.Equal(t, ProviderAnthropic, m.Provider)
	}

	// OpenAI should have at least 2 models
	openaiModels := grouped[ProviderOpenAI]
	assert.GreaterOrEqual(t, len(openaiModels), 2)
}

func TestProviderDisplayOrder(t *testing.T) {
	order := ProviderDisplayOrder()
	assert.NotEmpty(t, order)
	assert.Equal(t, ProviderAnthropic, order[0], "Anthropic should be first")
}

func TestProviderDisplayName(t *testing.T) {
	assert.Equal(t, "Anthropic", ProviderDisplayName(ProviderAnthropic))
	assert.Equal(t, "OpenAI", ProviderDisplayName(ProviderOpenAI))
	assert.Equal(t, "DeepSeek", ProviderDisplayName(ProviderDeepSeek))
	assert.Equal(t, "unknown_provider", ProviderDisplayName("unknown_provider"))
}

func TestResolveProvider(t *testing.T) {
	tests := []struct {
		input    string
		expected ModelProvider
		ok       bool
	}{
		{"anthropic", ProviderAnthropic, true},
		{"Anthropic", ProviderAnthropic, true},
		{"openai", ProviderOpenAI, true},
		{"deep", ProviderDeepSeek, true}, // prefix match
		{"nonexistent", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ResolveProvider(tt.input)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestCheapestModel(t *testing.T) {
	assert.Equal(t, Claude37Haiku, CheapestModel(ProviderAnthropic))
	assert.Equal(t, GPT41Mini, CheapestModel(ProviderOpenAI))
	assert.Equal(t, DeepSeekChat, CheapestModel(ProviderDeepSeek))
	assert.Equal(t, ModelID(""), CheapestModel("unknown"))
}

func TestNewCustomModel(t *testing.T) {
	m := NewCustomModel(ProviderOpenAI, "my-custom-model")
	assert.Equal(t, ModelID("my-custom-model"), m.ID)
	assert.Equal(t, ProviderOpenAI, m.Provider)
	assert.Equal(t, "my-custom-model", m.APIModel)
	assert.Equal(t, int64(16384), m.DefaultMaxTokens)
	assert.Equal(t, ModelStatusCurrent, m.Status)
}

func TestSupportedModels_Phase2SeedsPresent(t *testing.T) {
	for _, id := range []ModelID{
		GPT55, GPT54, GPT54Mini,
		Claude47Opus, Claude46Sonnet, Claude45Haiku, Claude45HaikuAlias,
		DeepSeekV4Flash, DeepSeekV4Pro,
	} {
		if _, ok := SupportedModels[id]; !ok {
			t.Fatalf("missing curated model seed: %s", id)
		}
	}
	if got := SupportedModels[DeepSeekChat].Status; got != ModelStatusDeprecated {
		t.Fatalf("deepseek-chat status = %q, want %q", got, ModelStatusDeprecated)
	}
	if got := SupportedModels[GPT55].ContextWindow; got != 1000000 {
		t.Fatalf("gpt-5.5 context window = %d, want 1000000", got)
	}
	if got := SupportedModels[GPT54Mini].ContextWindow; got != 400000 {
		t.Fatalf("gpt-5.4-mini context window = %d, want 400000", got)
	}
	if got := SupportedModels[GPT54].DefaultMaxTokens; got != 128000 {
		t.Fatalf("gpt-5.4 max output = %d, want 128000", got)
	}
}

func TestMergeModelOptions_OrderAndDedupe(t *testing.T) {
	curated := []Model{
		{ID: "a", APIModel: "a"},
		{ID: "b", APIModel: "b"},
	}
	configured := []Model{
		{ID: "b", APIModel: "b"},
		{ID: "c", APIModel: "c"},
	}
	discovered := []Model{
		{ID: "c", APIModel: "c"},
		{ID: "d", APIModel: "d"},
	}
	got := MergeModelOptions(curated, configured, discovered)
	require.Len(t, got, 4)
	assert.Equal(t, ModelID("a"), got[0].Model.ID)
	assert.Equal(t, ModelSourceCurated, got[0].Source)
	assert.Equal(t, ModelID("c"), got[2].Model.ID)
	assert.Equal(t, ModelSourceCurrentConfig, got[2].Source)
	assert.Equal(t, ModelID("d"), got[3].Model.ID)
	assert.Equal(t, ModelSourceProviderList, got[3].Source)
}
