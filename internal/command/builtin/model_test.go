package builtin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

func TestResolveModelCommandRefAllowsForcedStrictModel(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderOpenAI: {
				APIKey: "sk-test",
				Model:  string(models.GPT41),
				Models: map[string]config.ModelConfig{
					"future-model": {Name: "Future Model"},
				},
			},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(dir); err != nil {
		t.Fatalf("config.Load error: %v", err)
	}

	for _, input := range []string{"openai future-model", "openai:future-model"} {
		ref, err := resolveModelCommandRef(input)
		if err != nil {
			t.Fatalf("resolveModelCommandRef(%q) error: %v", input, err)
		}
		if ref.Provider != models.ProviderOpenAI || ref.ModelID != "future-model" {
			t.Fatalf("unexpected ref for %q: %+v", input, ref)
		}
	}
}

func TestRefreshProviderSavePersistsDiscoveredModels(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"future-model"}]}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		DefaultProvider: models.ProviderOpenAICompatible,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderOpenAICompatible: {
				APIKey:  "sk-test",
				BaseURL: srv.URL + "/v1",
				Model:   "future-model",
			},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(dir); err != nil {
		t.Fatalf("config.Load error: %v", err)
	}

	cmd := &modelCmd{}
	res := cmd.refreshProvider(command.Context{}, "openai_compatible --save")
	if !strings.Contains(res.Output, "已保存 1 个模型") {
		t.Fatalf("unexpected output: %s", res.Output)
	}

	loaded := config.Get()
	if loaded == nil {
		t.Fatal("config not loaded")
	}
	saved, ok := loaded.Providers[models.ProviderOpenAICompatible].Models["future-model"]
	if !ok {
		t.Fatalf("model not persisted in config: %+v", loaded.Providers[models.ProviderOpenAICompatible])
	}
	if saved.DefaultMaxTokens != 0 || saved.ContextWindow != 0 || saved.SupportsReasoning != nil {
		t.Fatalf("refresh --save should persist only list-provided facts, got %+v", saved)
	}
}
