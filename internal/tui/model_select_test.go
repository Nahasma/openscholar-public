package tui

import (
	"testing"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

func TestModelSelectState_MiniMaxOrderStableOnRepeatedSetup(t *testing.T) {
	m := newTestModel()
	m.dlg.modelSelect.providers = []models.ModelProvider{models.ProviderMiniMax}
	m.dlg.modelSelect.providerIdx = 0
	m.dlg.modelSelect.selectedModelID = models.MiniMaxM27

	var first []models.ModelID
	for i := 0; i < 100; i++ {
		m.setupModelListForProvider()
		if len(m.dlg.modelSelect.list) < 4 {
			t.Fatalf("expected >=4 minimax models, got %d", len(m.dlg.modelSelect.list))
		}
		got := []models.ModelID{
			m.dlg.modelSelect.list[0].ID,
			m.dlg.modelSelect.list[1].ID,
			m.dlg.modelSelect.list[2].ID,
			m.dlg.modelSelect.list[3].ID,
		}
		if i == 0 {
			first = got
			continue
		}
		for idx := range first {
			if got[idx] != first[idx] {
				t.Fatalf("order drift at run=%d idx=%d: got=%v want=%v", i, idx, got, first)
			}
		}
	}
}

func TestModelSelectState_PreservesSelectedMiniMaxModelIDOnRebuild(t *testing.T) {
	m := newTestModel()
	m.dlg.modelSelect.providers = []models.ModelProvider{models.ProviderMiniMax}
	m.dlg.modelSelect.providerIdx = 0
	m.dlg.modelSelect.selectedModelID = models.MiniMaxM25

	m.setupModelListForProvider()
	if m.dlg.modelSelect.list[m.dlg.modelSelect.listIdx].ID != models.MiniMaxM25 {
		t.Fatalf("expected selected id %s, got %s", models.MiniMaxM25, m.dlg.modelSelect.list[m.dlg.modelSelect.listIdx].ID)
	}

	m.dlg.modelSelect.listIdx = 0
	m.ensureModelSelectionVisible()
	if m.dlg.modelSelect.selectedModelID != models.MiniMaxM27 {
		t.Fatalf("expected selected id %s after moving focus, got %s", models.MiniMaxM27, m.dlg.modelSelect.selectedModelID)
	}

	m.dlg.modelSelect.selectedModelID = models.MiniMaxM25
	m.setupModelListForProvider()
	if m.dlg.modelSelect.list[m.dlg.modelSelect.listIdx].ID != models.MiniMaxM25 {
		t.Fatalf("selection drifted after rebuild, got %s", m.dlg.modelSelect.list[m.dlg.modelSelect.listIdx].ID)
	}
}

func TestModelSelectOverlay_PreservesSelectedMiniMaxModelIDOnRebuild(t *testing.T) {
	o := NewModelSelectOverlay(models.ProviderMiniMax, models.MiniMaxM25)
	o.providers = []models.ModelProvider{models.ProviderMiniMax}
	o.providerIdx = 0
	o.selectedModelID = models.MiniMaxM25

	o.setupModelList()
	if o.list[o.listIdx].ID != models.MiniMaxM25 {
		t.Fatalf("expected selected id %s, got %s", models.MiniMaxM25, o.list[o.listIdx].ID)
	}

	o.listIdx = 0
	o.ensureVisible()
	if o.selectedModelID != models.MiniMaxM27 {
		t.Fatalf("expected selected id %s after focus move, got %s", models.MiniMaxM27, o.selectedModelID)
	}

	o.selectedModelID = models.MiniMaxM25
	o.setupModelList()
	if o.list[o.listIdx].ID != models.MiniMaxM25 {
		t.Fatalf("selection drifted after overlay rebuild, got %s", o.list[o.listIdx].ID)
	}
}

func TestModelSelectState_IncludesConfiguredProviderModels(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)

	dir := t.TempDir()
	cfg := config.Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderOpenAI: {
				APIKey: "sk-test",
				Model:  "future-model",
				Models: map[string]config.ModelConfig{
					"future-model": {Name: "Future Model"},
				},
			},
		},
	}
	if err := config.SaveFull(dir, cfg); err != nil {
		t.Fatalf("SaveFull error: %v", err)
	}
	if _, err := config.Load(dir); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	m := newTestModel()
	m.dlg.modelSelect.providers = []models.ModelProvider{models.ProviderOpenAI}
	m.dlg.modelSelect.providerIdx = 0
	m.dlg.modelSelect.selectedModelID = models.ModelID("future-model")

	m.setupModelListForProvider()
	if m.dlg.modelSelect.list[m.dlg.modelSelect.listIdx].ID != "future-model" {
		t.Fatalf("configured model not selected: %+v", m.dlg.modelSelect.list)
	}
}

func TestModelSelectState_MergesDiscoveredProviderModels(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)

	dir := t.TempDir()
	cfg := config.Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderOpenAI: {APIKey: "sk-test", Model: string(models.GPT41)},
		},
	}
	if err := config.SaveFull(dir, cfg); err != nil {
		t.Fatalf("SaveFull error: %v", err)
	}
	if _, err := config.Load(dir); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	m := newTestModel()
	m.dlg.modelSelect.providers = []models.ModelProvider{models.ProviderOpenAI}
	m.dlg.modelSelect.providerIdx = 0
	m.dlg.modelSelect.selectedModelID = "provider-list-model"
	m.dlg.modelSelect.discovered = map[models.ModelProvider][]models.Model{
		models.ProviderOpenAI: {models.NewCustomModel(models.ProviderOpenAI, "provider-list-model")},
	}

	m.setupModelListForProvider()
	if m.dlg.modelSelect.list[m.dlg.modelSelect.listIdx].ID != "provider-list-model" {
		t.Fatalf("discovered model not selected: %+v", m.dlg.modelSelect.list)
	}
}

func TestModelSelectOverlay_AppliesModelListLoadedMsg(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)

	dir := t.TempDir()
	cfg := config.Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderOpenAI: {APIKey: "sk-test", Model: string(models.GPT41)},
		},
	}
	if err := config.SaveFull(dir, cfg); err != nil {
		t.Fatalf("SaveFull error: %v", err)
	}
	if _, err := config.Load(dir); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	o := NewModelSelectOverlay(models.ProviderOpenAI, models.GPT41)
	o.providers = []models.ModelProvider{models.ProviderOpenAI}
	o.providerIdx = 0
	o.selectedModelID = "provider-list-model"
	_, _, _ = o.Update(modelListLoadedMsg{
		Provider: models.ProviderOpenAI,
		Models:   []models.Model{models.NewCustomModel(models.ProviderOpenAI, "provider-list-model")},
	})

	if o.list[o.listIdx].ID != "provider-list-model" {
		t.Fatalf("overlay discovered model not selected: %+v", o.list)
	}
}
