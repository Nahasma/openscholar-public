package config

import (
	"fmt"
	"os"

	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

// AuthSource identifies where an API key was resolved from.
type AuthSource string

const (
	AuthEnv    AuthSource = "env"
	AuthConfig AuthSource = "config"
	AuthHelper AuthSource = "helper"
)

func LoadAPIKey(providerName models.ModelProvider) string {
	if cfg == nil {
		return ""
	}
	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return ""
	}
	return providerCfg.APIKey
}

func KnownProviders() []models.ModelProvider {
	out := make([]models.ModelProvider, 0, len(models.ProviderCatalog()))
	for _, spec := range models.ProviderCatalog() {
		out = append(out, spec.ID)
	}
	return out
}

// providerEnvKey returns the environment variable name for a given provider.
func providerEnvKey(p models.ModelProvider) string {
	envVars := models.ProviderAPIKeyEnvVars(p)
	if len(envVars) == 0 {
		return ""
	}
	return envVars[0]
}

// ResolveAPIKey resolves an API key for the given provider.
// Priority: env var > config.apiKey > apiKeyHelper command.
func ResolveAPIKey(providerName models.ModelProvider, providerCfg Provider) (key string, source AuthSource, err error) {
	// 1. Check environment variable (highest priority)
	for _, envName := range models.ProviderAPIKeyEnvVars(providerName) {
		if val := os.Getenv(envName); val != "" {
			return val, AuthEnv, nil
		}
	}

	// 2. Check config.apiKey field
	if providerCfg.APIKey != "" {
		return providerCfg.APIKey, AuthConfig, nil
	}

	// 3. TODO: enable when Provider.APIKeyHelper field is added
	// if providerCfg.APIKeyHelper != "" {
	//     ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	//     defer cancel()
	//     helperKey, helperErr := ExecuteKeyHelper(ctx, providerCfg.APIKeyHelper)
	//     if helperErr == nil && helperKey != "" {
	//         return helperKey, AuthHelper, nil
	//     }
	// }

	switch models.ProviderAuthMode(providerName) {
	case models.AuthOptional, models.AuthNone:
		return "", "", nil
	default:
		envName := providerEnvKey(providerName)
		if envName == "" {
			return "", "", fmt.Errorf("no API key found for provider %q: configure apiKey in config", providerName)
		}
		return "", "", fmt.Errorf("no API key found for provider %q: set env var %s or configure apiKey in config", providerName, envName)
	}
}
