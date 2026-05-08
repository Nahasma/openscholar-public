package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/openscholar/openscholar/internal/llm/models"
)

func TestSaveAgentModelProvider(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	if err := SaveAgentModelProvider(dir, AgentGeneral, models.ProviderOllama, "qwen2.5:14b"); err != nil {
		t.Fatalf("save error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".openscholar", "config.json"))
	if err != nil {
		t.Fatalf("read config error: %v", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	a := c.Agents[AgentGeneral]
	if a.Provider != models.ProviderOllama || a.Model != "qwen2.5:14b" {
		t.Fatalf("unexpected saved agent config: %+v", a)
	}
}

func TestSaveAgentModelProviderForcePersistsProviderModelConfig(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	if err := SaveAgentModelProviderForce(dir, AgentCoder, models.ProviderOpenAI, "future-model"); err != nil {
		t.Fatalf("save error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".openscholar", "config.json"))
	if err != nil {
		t.Fatalf("read config error: %v", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if _, ok := c.Providers[models.ProviderOpenAI].Models["future-model"]; !ok {
		t.Fatalf("forced model not saved: %+v", c.Providers[models.ProviderOpenAI])
	}
}

func TestSaveProviderModels(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	if err := SaveProviderModels(dir, models.ProviderOpenAI, map[string]ModelConfig{
		"gpt-5.5": {Name: "GPT-5.5"},
	}); err != nil {
		t.Fatalf("save error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".openscholar", "config.json"))
	if err != nil {
		t.Fatalf("read config error: %v", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if _, ok := c.Providers[models.ProviderOpenAI].Models["gpt-5.5"]; !ok {
		t.Fatalf("provider model not saved: %+v", c.Providers[models.ProviderOpenAI])
	}
}

func TestSaveProviderModelsPreservesExistingMetadata(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	supportsReasoning := true
	if err := SaveProviderModels(dir, models.ProviderOpenAI, map[string]ModelConfig{
		"future-model": {
			Name:              "Future Model",
			ContextWindow:     1234,
			DefaultMaxTokens:  99,
			SupportsReasoning: &supportsReasoning,
		},
	}); err != nil {
		t.Fatalf("initial save error: %v", err)
	}
	if err := SaveProviderModels(dir, models.ProviderOpenAI, map[string]ModelConfig{
		"future-model": {Name: "Future Model Renamed"},
	}); err != nil {
		t.Fatalf("merge save error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".openscholar", "config.json"))
	if err != nil {
		t.Fatalf("read config error: %v", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	saved := c.Providers[models.ProviderOpenAI].Models["future-model"]
	if saved.Name != "Future Model Renamed" {
		t.Fatalf("name = %q, want updated name", saved.Name)
	}
	if saved.ContextWindow != 1234 || saved.DefaultMaxTokens != 99 {
		t.Fatalf("metadata was not preserved: %+v", saved)
	}
	if saved.SupportsReasoning == nil || !*saved.SupportsReasoning {
		t.Fatalf("supports reasoning was not preserved: %+v", saved)
	}
}

func TestSaveAgentModelProviderPersistsInMemoryProviderModel(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	cfg := Config{
		WorkingDir:      dir,
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: {APIKey: "sk-test", Model: string(models.GPT41)},
		},
	}
	if err := SaveFull(dir, cfg); err != nil {
		t.Fatalf("SaveFull error: %v", err)
	}
	if _, err := Load(dir); err != nil {
		t.Fatalf("Load error: %v", err)
	}
	MergeProviderModelsInMemory(models.ProviderOpenAI, map[string]ModelConfig{
		"provider-list-model": {Name: "Provider List Model"},
	})

	if err := SaveAgentModelProvider(dir, AgentCoder, models.ProviderOpenAI, "provider-list-model"); err != nil {
		t.Fatalf("SaveAgentModelProvider error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".openscholar", "config.json"))
	if err != nil {
		t.Fatalf("read config error: %v", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if got := c.Providers[models.ProviderOpenAI].Models["provider-list-model"].Name; got != "Provider List Model" {
		t.Fatalf("provider model name = %q, want Provider List Model", got)
	}
}

func TestModelOptionsForProvider_MergesConfiguredAfterCurated(t *testing.T) {
	cfg := &Config{
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: {
				Model: "custom-current",
				Models: map[string]ModelConfig{
					"custom-z": {Name: "Custom Z"},
					"gpt-5.5":  {Name: "duplicate curated"},
				},
			},
		},
	}
	opts := ModelOptionsForProvider(cfg, models.ProviderOpenAI, []models.Model{
		models.NewCustomModel(models.ProviderOpenAI, "custom-discovered"),
	})
	if len(opts) < 4 {
		t.Fatalf("expected merged options, got %+v", opts)
	}
	if opts[0].Source != models.ModelSourceCurated {
		t.Fatalf("first option source = %q, want curated", opts[0].Source)
	}
	seen := map[models.ModelID]int{}
	for _, opt := range opts {
		seen[opt.Model.ID]++
	}
	if seen[models.GPT55] != 1 {
		t.Fatalf("curated duplicate not deduped, seen=%v", seen[models.GPT55])
	}
	var foundCurrent, foundDiscovered bool
	for _, opt := range opts {
		if opt.Model.ID == "custom-current" && opt.Source == models.ModelSourceCurrentConfig {
			foundCurrent = true
		}
		if opt.Model.ID == "custom-discovered" && opt.Source == models.ModelSourceProviderList {
			foundDiscovered = true
		}
	}
	if !foundCurrent || !foundDiscovered {
		t.Fatalf("missing merged config/discovery options: %+v", opts)
	}
}
