package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/llm/connectivity"
	"github.com/openscholar/openscholar/internal/llm/models"
)

type modelListLoadedMsg struct {
	Provider  models.ModelProvider
	Models    []models.Model
	Err       error
	FetchedAt time.Time
}

func discoverModelsForProviderCmd(providerName models.ModelProvider) tea.Cmd {
	if !models.ProviderSupportsList(providerName) {
		return nil
	}
	return func() tea.Msg {
		cfg := config.Get()
		if cfg == nil {
			return modelListLoadedMsg{Provider: providerName, Err: fmt.Errorf("config is not loaded"), FetchedAt: time.Now()}
		}
		providerCfg, ok := cfg.Providers[providerName]
		if !ok || providerCfg.Disabled {
			return modelListLoadedMsg{Provider: providerName, Err: fmt.Errorf("provider %s is not configured", providerName), FetchedAt: time.Now()}
		}
		apiKey, _, err := config.ResolveAPIKey(providerName, providerCfg)
		if err != nil {
			return modelListLoadedMsg{Provider: providerName, Err: err, FetchedAt: time.Now()}
		}
		res := connectivity.DiscoverModels(context.Background(), connectivity.TestRequest{
			Provider: providerName,
			APIKey:   apiKey,
			BaseURL:  providerCfg.BaseURL,
			Profile:  providerCfg.Profile,
			AuthMode: providerCfg.AuthMode,
			Timeout:  10 * time.Second,
		})
		if !res.ProviderOK && !res.Warning {
			return modelListLoadedMsg{Provider: providerName, Err: errors.New(res.Message), FetchedAt: time.Now()}
		}
		return modelListLoadedMsg{Provider: providerName, Models: res.Models, FetchedAt: time.Now()}
	}
}

func modelConfigsFromDiscovered(discovered []models.Model) map[string]config.ModelConfig {
	if len(discovered) == 0 {
		return nil
	}
	out := make(map[string]config.ModelConfig, len(discovered))
	for _, m := range discovered {
		if m.ID == "" {
			continue
		}
		out[string(m.ID)] = config.ModelConfig{Name: m.Name}
	}
	return out
}
