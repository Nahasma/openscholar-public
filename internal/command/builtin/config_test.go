package builtin

import (
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/llm/models"
)

func TestConfigDefaultShowsCenter(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	t.Setenv("OPENAI_API_KEY", "test-key")
	_, _ = config.Load(t.TempDir())

	cmd := &configCmd{}
	res := cmd.Execute(command.Context{})
	if res.Action != "" {
		t.Fatalf("expected no action, got %q", res.Action)
	}
	if res.OutputTitle != "Settings Center" {
		t.Fatalf("expected settings center title, got %q", res.OutputTitle)
	}
	if !strings.Contains(res.Output, "/config wizard") {
		t.Fatalf("expected settings center body, got: %s", res.Output)
	}
}

func TestConfigWizardCompatibility(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	t.Setenv("OPENAI_API_KEY", "test-key")
	if _, err := config.Load(t.TempDir()); err != nil {
		t.Fatalf("load config: %v", err)
	}

	cmd := &configCmd{}
	res := cmd.Execute(command.Context{Args: "wizard"})
	if res.Action != "config-wizard" {
		t.Fatalf("expected config-wizard action, got %q", res.Action)
	}
}

func TestConfigWizardRequiresLoadedConfig(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)

	cmd := &configCmd{}
	res := cmd.Execute(command.Context{Args: "wizard"})
	if res.Action != "" {
		t.Fatalf("expected no action without loaded config, got %q", res.Action)
	}
	if !strings.Contains(res.Output, "配置未加载") {
		t.Fatalf("expected config-not-loaded output, got %q", res.Output)
	}
}

func TestConfigDebugMasksSecrets(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	t.Setenv("OPENAI_API_KEY", "test-key")
	cfg, _ := config.Load(t.TempDir())
	if cfg == nil {
		t.Fatalf("expected loaded config")
	}
	prov := cfg.Providers[models.ProviderOpenAI]
	prov.APIKey = "super-secret-key"
	cfg.Providers[models.ProviderOpenAI] = prov
	cfg.DefaultProvider = models.ProviderOpenAI

	cmd := &configCmd{}
	res := cmd.Execute(command.Context{Args: "debug"})
	if strings.Contains(res.Output, "super-secret-key") {
		t.Fatalf("debug leaked secret: %s", res.Output)
	}
	if !strings.Contains(res.Output, "OpenAI") || !strings.Contains(res.Output, "env") {
		t.Fatalf("debug should include auth source: %s", res.Output)
	}
	if res.EffectiveOutputKind() != command.OutputCommandBlock {
		t.Fatalf("output kind = %q, want %q", res.EffectiveOutputKind(), command.OutputCommandBlock)
	}
}
