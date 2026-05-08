package minimax

import (
	"fmt"
	"strings"
)

const (
	// TokenPlanBaseURL is the current official Anthropic-compatible endpoint.
	TokenPlanBaseURL = "https://api.minimaxi.com/anthropic"
	// LegacyBaseURL is kept for diagnostics and explicit legacy profile users.
	LegacyBaseURL = "https://api.minimax.io/anthropic"
)

type AuthMode string

const (
	AuthXAPIKey AuthMode = "anthropic_x_api_key"
	AuthBearer  AuthMode = "bearer"
)

type Runtime struct {
	BaseURL string
	Auth    AuthMode
	Profile string
}

func ResolveRuntime(baseURL, profile, authMode string) (Runtime, error) {
	resolved := Runtime{}
	resolved.BaseURL = strings.TrimSpace(baseURL)
	resolved.Profile = strings.ToLower(strings.TrimSpace(profile))
	if resolved.Profile == "" {
		resolved.Profile = "auto"
	}

	switch resolved.Profile {
	case "auto", "legacy", "token-plan", "custom":
	default:
		return Runtime{}, fmt.Errorf("provider minimax has unsupported profile %q", profile)
	}

	explicitAuth := strings.ToLower(strings.TrimSpace(authMode))
	switch explicitAuth {
	case "":
	case "auto":
		explicitAuth = ""
	case string(AuthXAPIKey), string(AuthBearer):
	default:
		return Runtime{}, fmt.Errorf("provider minimax has unsupported authMode %q", authMode)
	}

	switch resolved.Profile {
	case "legacy":
		if resolved.BaseURL == "" {
			resolved.BaseURL = LegacyBaseURL
		}
	case "token-plan":
		if resolved.BaseURL == "" {
			resolved.BaseURL = TokenPlanBaseURL
		}
	case "custom":
		if resolved.BaseURL == "" {
			return Runtime{}, fmt.Errorf("provider minimax profile=custom requires an explicit baseURL")
		}
	case "auto":
		if resolved.BaseURL == "" {
			resolved.BaseURL = TokenPlanBaseURL
		}
	}

	if explicitAuth != "" {
		resolved.Auth = AuthMode(explicitAuth)
		return resolved, nil
	}

	lowerBase := strings.ToLower(strings.TrimSpace(resolved.BaseURL))
	switch {
	case resolved.Profile == "legacy":
		resolved.Auth = AuthXAPIKey
	case strings.Contains(lowerBase, "api.minimaxi.com/anthropic"):
		resolved.Auth = AuthBearer
	case strings.Contains(lowerBase, "api.minimax.io/anthropic"):
		resolved.Auth = AuthBearer
	case resolved.Profile == "token-plan":
		resolved.Auth = AuthBearer
	default:
		resolved.Auth = AuthXAPIKey
	}

	return resolved, nil
}
