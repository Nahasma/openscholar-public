package config

import (
	"os"
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/llm/models"
)

func TestMaskSecret(t *testing.T) {
	if got := MaskSecret("12345678"); got != "****" {
		t.Fatalf("short mask = %q", got)
	}
	if got := MaskSecret("abcdef123456"); got != "****3456" {
		t.Fatalf("mask = %q", got)
	}
}

func TestProviderAuthSource(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cfg := &Config{Providers: map[models.ModelProvider]Provider{}}
	cfg.Providers[models.ProviderOpenAI] = Provider{APIKey: "x"}
	if got := ProviderAuthSource(cfg, models.ProviderOpenAI); got != "config" {
		t.Fatalf("auth source = %q", got)
	}
	if got := ProviderAuthSource(cfg, models.ProviderOllama); got != "none" {
		t.Fatalf("ollama auth source = %q", got)
	}
}

func TestProviderAuthSourceTreatsLoadedConfigExportAsConfig(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	t.Setenv("OPENAI_API_KEY", "")

	dir := t.TempDir()
	key := "file-secret-key"
	cfg := Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: Provider{APIKey: key},
		},
	}
	if err := SaveFull(dir, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if os.Getenv("OPENAI_API_KEY") != key {
		t.Fatalf("expected config key exported to env")
	}
	if got := ProviderAuthSource(loaded, models.ProviderOpenAI); got != "config" {
		t.Fatalf("auth source = %q", got)
	}
}

func TestProviderAuthSourceReportsEnvOnlyLoadedConfigAsEnv(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	t.Setenv("OPENAI_API_KEY", "env-only-key")

	loaded, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := ProviderAuthSource(loaded, models.ProviderOpenAI); got != "env" {
		t.Fatalf("auth source = %q, want env", got)
	}
}

func TestDebugReportDoesNotLeakKeys(t *testing.T) {
	dir := t.TempDir()
	key := "super-secret-key"
	t.Setenv("OPENAI_API_KEY", "")
	cfg := &Config{
		WorkingDir:      dir,
		DefaultProvider: models.ProviderOpenAI,
		Providers:       map[models.ModelProvider]Provider{models.ProviderOpenAI: Provider{APIKey: key}},
		Agents:          map[AgentName]Agent{AgentCoder: Agent{Provider: models.ProviderOpenAI, Model: "gpt-4.1"}},
	}
	out := DebugReport(cfg)
	if strings.Contains(out, key) {
		t.Fatalf("debug leaked key: %s", out)
	}
	for _, want := range []string{"Default Provider:", "Providers:", "Agent Mappings:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("debug output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "Provider") || !strings.Contains(out, "Auth") || !strings.Contains(out, "Disabled") {
		t.Fatalf("provider headers missing:\n%s", out)
	}
	if !strings.Contains(out, "Agent") || !strings.Contains(out, "Model") {
		t.Fatalf("agent headers missing:\n%s", out)
	}
	if _, err := os.Stat(ConfigFilePath(dir)); !os.IsNotExist(err) {
		t.Fatalf("expected missing config file in temp dir")
	}
}
