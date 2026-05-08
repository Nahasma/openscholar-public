package models

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OllamaTag represents a model in Ollama's /api/tags response.
type OllamaTag struct {
	Name       string `json:"name"`
	ModifiedAt string `json:"modified_at"`
	Size       int64  `json:"size"`
}

// DiscoverOllamaModels queries a running Ollama instance for installed models.
// Returns nil (not error) if Ollama is unavailable — no hard dependency.
func DiscoverOllamaModels(baseURL string) []Model {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/api/tags")
	if err != nil {
		return nil // Ollama not available, silently skip
	}
	defer resp.Body.Close()

	var result struct {
		Models []OllamaTag `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	discovered := make([]Model, 0, len(result.Models))
	for _, tag := range result.Models {
		id := ModelID("ollama:" + tag.Name)
		discovered = append(discovered, Model{
			ID:               id,
			Name:             fmt.Sprintf("Ollama %s", tag.Name),
			Provider:         ProviderOllama,
			APIModel:         tag.Name,
			CostPer1MIn:      0, // local model, free
			CostPer1MOut:     0,
			ContextWindow:    8192, // Ollama default, actual depends on model
			DefaultMaxTokens: 4096,
		})
	}
	return discovered
}

// RegisterOllamaModels is deprecated.
// It keeps compatibility but no longer mutates global SupportedModels.
// Returns the number of discovered models.
func RegisterOllamaModels(baseURL string) int {
	discovered := DiscoverOllamaModels(baseURL)
	return len(discovered)
}
