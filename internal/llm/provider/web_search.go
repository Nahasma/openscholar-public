package provider

import (
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/web"
)

// WebSearchBackend returns a web.SearchProvider appropriate for the given
// provider, or nil if the provider does not support web search.
func WebSearchBackend(p Provider, openaiSearchModel string) web.SearchProvider {
	model := p.Model()

	switch model.Provider {
	case models.ProviderAnthropic:
		// Extract Anthropic client from baseProvider[*anthropicClient]
		if bp, ok := p.(*baseProvider[*anthropicClient]); ok {
			return web.NewAnthropicSearch(bp.client.client, string(model.APIModel))
		}

	case models.ProviderOpenAI:
		// Extract OpenAI client from baseProvider[*openaiClient]
		if bp, ok := p.(*baseProvider[*openaiClient]); ok {
			searchModel := openaiSearchModel
			if searchModel == "" {
				searchModel = "gpt-4o-search-preview"
			}
			return web.NewOpenAISearch(bp.client.client, searchModel)
		}
	}

	return nil
}

// WebSearchBackendType returns the provider type name ("anthropic" or "openai")
// for a given Provider's web search backend, or empty string if unsupported.
func WebSearchBackendType(p Provider) string {
	switch p.Model().Provider {
	case models.ProviderAnthropic:
		return "anthropic"
	case models.ProviderOpenAI:
		return "openai"
	default:
		return ""
	}
}
