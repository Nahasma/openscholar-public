package config

import (
	"os"

	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

func mergeEnvProviders(cfg *Config) {
	for _, spec := range models.ProviderCatalog() {
		for _, envName := range spec.APIKeyEnvVars {
			overlayProviderAPIKey(cfg, spec.ID, envName)
		}
		for _, envName := range spec.BaseURLEnvVars {
			overlayProviderBaseURL(cfg, spec.ID, envName)
		}
	}

	// Scholar API keys from environment
	overlayScholarKey(cfg, "OPENALEX_API_KEY", func(s *ScholarConfig, v string) { s.OpenAlexAPIKey = v })
	overlayScholarKey(cfg, "NCBI_API_KEY", func(s *ScholarConfig, v string) { s.NCBIAPIKey = v })
	overlayScholarKey(cfg, "CORE_API_KEY", func(s *ScholarConfig, v string) { s.CoreAPIKey = v })
	overlayScholarKey(cfg, "PATENTSVIEW_API_KEY", func(s *ScholarConfig, v string) { s.PatentsViewAPIKey = v })
	overlayScholarKey(cfg, "SEMANTIC_SCHOLAR_API_KEY", func(s *ScholarConfig, v string) { s.SemanticScholarKey = v })
	overlayScholarKey(cfg, "SCHOLAR_CONTACT_EMAIL", func(s *ScholarConfig, v string) { s.ContactEmail = v })
}

func overlayProviderAPIKey(cfg *Config, providerName models.ModelProvider, envName string) {
	apiKey := os.Getenv(envName)
	if apiKey == "" {
		return
	}
	providerCfg := cfg.Providers[providerName]
	providerCfg.APIKey = apiKey
	cfg.Providers[providerName] = providerCfg
}

func overlayProviderBaseURL(cfg *Config, providerName models.ModelProvider, envName string) {
	baseURL := os.Getenv(envName)
	if baseURL == "" {
		return
	}
	providerCfg := cfg.Providers[providerName]
	providerCfg.BaseURL = baseURL
	cfg.Providers[providerName] = providerCfg
}

func overlayScholarKey(cfg *Config, envName string, setter func(*ScholarConfig, string)) {
	v := os.Getenv(envName)
	if v == "" {
		return
	}
	setter(&cfg.Scholar, v)
}

// exportAPIKeysToEnv sets provider API keys as environment variables
// so that bash/curl commands in SubAgents can access them.
func exportAPIKeysToEnv(cfg *Config) {
	for _, spec := range models.ProviderCatalog() {
		if len(spec.APIKeyEnvVars) == 0 {
			continue
		}
		if p, ok := cfg.Providers[spec.ID]; ok && p.APIKey != "" {
			envName := spec.APIKeyEnvVars[0]
			if os.Getenv(envName) == "" {
				os.Setenv(envName, p.APIKey)
			}
		}
	}
}
