package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/openscholar/openscholar/internal/llm/models"
)

func TestLoad_LocalProviderWithoutAPIKey(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgData := Config{
		DefaultProvider: models.ProviderOllama,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOllama: {BaseURL: "http://localhost:11434", Model: "qwen2.5:14b"},
		},
	}
	data, _ := json.Marshal(cfgData)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.DefaultProvider != models.ProviderOllama {
		t.Fatalf("unexpected default provider: %s", loaded.DefaultProvider)
	}
}

func TestLoad_LegacyLocalBaseURLMapsToProviderConfig(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{
  "defaultProvider": "ollama",
  "ollama_base_url": "http://remote-ollama:11434/v1",
  "providers": {
    "ollama": {"model": "qwen2.5:14b"}
  }
}`)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got := loaded.Providers[models.ProviderOllama].BaseURL; got != "http://remote-ollama:11434/v1" {
		t.Fatalf("ollama provider baseURL = %q", got)
	}
}

func TestLoad_PathsConfig(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{
  "defaultProvider": "ollama",
  "providers": {
    "ollama": {"baseURL": "http://localhost:11434", "model": "qwen2.5:14b"}
  },
  "paths": {
    "root": ".openscholar",
    "state": ".openscholar/state",
    "logs": ".openscholar/logs"
  }
}`)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.Paths.Root != ".openscholar" {
		t.Fatalf("paths.root = %q", loaded.Paths.Root)
	}
	if loaded.Paths.State != ".openscholar/state" {
		t.Fatalf("paths.state = %q", loaded.Paths.State)
	}
	if loaded.Paths.Logs != ".openscholar/logs" {
		t.Fatalf("paths.logs = %q", loaded.Paths.Logs)
	}
}
