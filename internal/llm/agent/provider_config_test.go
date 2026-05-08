package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

func TestCreateAgentProviderRejectsUnknownStrictModel(t *testing.T) {
	withConfigFile(t, config.Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderOpenAI: {APIKey: "sk-test", Model: "typo-model"},
		},
	})

	_, err := createAgentProvider(config.AgentCoder)
	if err == nil {
		t.Fatal("expected unknown strict-provider model error")
	}
}

func TestCreateAgentProviderAllowsConfiguredCustomStrictModel(t *testing.T) {
	withConfigFile(t, config.Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderOpenAI: {
				APIKey: "sk-test",
				Model:  "future-model",
				Models: map[string]config.ModelConfig{
					"future-model": {Name: "Future Model", ContextWindow: 123456},
				},
			},
		},
	})

	p, err := createAgentProvider(config.AgentCoder)
	if err != nil {
		t.Fatalf("createAgentProvider error: %v", err)
	}
	if got := p.Model().ID; got != "future-model" {
		t.Fatalf("model ID = %q", got)
	}
	if got := p.Model().ContextWindow; got != 123456 {
		t.Fatalf("context window = %d", got)
	}
}

func TestRuntimeProviderAllowsProviderPrefixedConfiguredCustomStrictModel(t *testing.T) {
	withConfigFile(t, config.Config{
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
	})
	current, err := createAgentProvider(config.AgentCoder)
	if err != nil {
		t.Fatalf("createAgentProvider error: %v", err)
	}
	ag := &agent{agentProvider: current, agentName: config.AgentCoder}

	p, err := ag.createRuntimeProvider("openai:future-model")
	if err != nil {
		t.Fatalf("createRuntimeProvider error: %v", err)
	}
	if got := p.Model().ID; got != "future-model" {
		t.Fatalf("runtime model ID = %q", got)
	}
}

func TestRuntimeProviderRejectsCrossProviderOverride(t *testing.T) {
	withConfigFile(t, config.Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderOpenAI: {
				APIKey: "sk-test",
				Model:  string(models.GPT41),
			},
			models.ProviderAnthropic: {
				APIKey: "sk-ant-test",
				Model:  string(models.Claude4Sonnet),
			},
		},
	})
	current, err := createAgentProvider(config.AgentCoder)
	if err != nil {
		t.Fatalf("createAgentProvider error: %v", err)
	}
	ag := &agent{agentProvider: current, agentName: config.AgentCoder}

	if _, err := ag.createRuntimeProvider("anthropic:" + string(models.Claude4Sonnet)); err == nil {
		t.Fatal("expected cross-provider runtime override to be rejected")
	}
}

func TestCreateSmallFastCallLLMFallsBackToCoder(t *testing.T) {
	withConfigFile(t, config.Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderOpenAI: {
				APIKey: "sk-test",
				Model:  string(models.GPT41),
			},
		},
		Agents: map[config.AgentName]config.Agent{
			config.AgentCoder:      {Provider: models.ProviderOpenAI, Model: string(models.GPT41)},
			config.AgentSummarizer: {Provider: models.ProviderOpenAI, Model: "typo-model"},
		},
	})

	callLLM, err := CreateSmallFastCallLLM()
	if err != nil {
		t.Fatalf("CreateSmallFastCallLLM error: %v", err)
	}
	if callLLM == nil {
		t.Fatal("expected non-nil caller")
	}
}

func TestCreateSmallFastCallLLMReportsSummarizerAndFallbackErrors(t *testing.T) {
	withConfigFile(t, config.Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderOpenAI: {
				APIKey: "sk-test",
				Model:  string(models.GPT41),
			},
		},
		Agents: map[config.AgentName]config.Agent{
			config.AgentCoder:      {Provider: models.ProviderOpenAI, Model: "bad-coder-model"},
			config.AgentSummarizer: {Provider: models.ProviderOpenAI, Model: "bad-summary-model"},
		},
	})

	_, err := CreateSmallFastCallLLM()
	if err == nil {
		t.Fatal("expected CreateSmallFastCallLLM error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "summarizer caller unavailable") {
		t.Fatalf("error %q does not mention summarizer failure", msg)
	}
	if !strings.Contains(msg, "coder fallback caller unavailable") {
		t.Fatalf("error %q does not mention coder fallback failure", msg)
	}
}

func withConfigFile(t *testing.T, cfg config.Config) {
	t.Helper()
	config.Reset()
	t.Cleanup(config.Reset)

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
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
}
