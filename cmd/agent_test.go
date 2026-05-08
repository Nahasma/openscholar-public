package cmd

import (
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

func TestCustomAgentProviderPassesMiniMaxProfileConfig(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: models.ProviderMiniMax,
		Providers: map[models.ModelProvider]config.Provider{
			models.ProviderMiniMax: {
				APIKey:  "test-key",
				Model:   string(models.MiniMaxM27),
				Profile: "custom",
			},
		},
		Agents: map[config.AgentName]config.Agent{
			config.AgentCoder: {Provider: models.ProviderMiniMax, Model: string(models.MiniMaxM27)},
		},
	}

	_, err := customAgentProvider(cfg)
	if err == nil {
		t.Fatal("expected profile=custom without baseURL to fail")
	}
	if !strings.Contains(err.Error(), "profile=custom") {
		t.Fatalf("expected MiniMax profile error, got %v", err)
	}
}
