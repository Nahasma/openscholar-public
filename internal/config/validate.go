package config

import (
	"fmt"

	"github.com/Nahasma/openscholar-public/internal/hooks"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

// ValidationError represents a single configuration validation error.
type ValidationError struct {
	Path    string // config field path, e.g. "agents.coder.provider"
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// Validate checks a Config for structural and semantic correctness.
// Returns all errors found (does not stop at first error).
func Validate(cfg *Config) []ValidationError {
	var errs []ValidationError

	// Validate providers
	errs = append(errs, validateProviders(cfg)...)

	// Validate agents
	errs = append(errs, validateAgents(cfg)...)

	// Validate MCP servers
	errs = append(errs, validateMCPServers(cfg)...)

	// Validate harness config
	errs = append(errs, validateHarness(cfg)...)

	// Validate paper type
	errs = append(errs, validatePaperType(cfg)...)

	// Validate lifecycle hooks
	errs = append(errs, validateHooks(cfg)...)

	// Validate subagent orchestration config
	errs = append(errs, validateSubagentOrchestration(cfg)...)

	return errs
}

// knownProviders is the set of valid provider names.
var knownProviders = func() map[models.ModelProvider]bool {
	out := make(map[models.ModelProvider]bool)
	for _, spec := range models.ProviderCatalog() {
		out[spec.ID] = true
	}
	return out
}()

func validateProviders(cfg *Config) []ValidationError {
	var errs []ValidationError

	if cfg.DefaultProvider != "" && !knownProviders[cfg.DefaultProvider] {
		errs = append(errs, ValidationError{
			Path:    "default_provider",
			Message: fmt.Sprintf("unknown provider %q", cfg.DefaultProvider),
		})
	}

	for name, p := range cfg.Providers {
		path := fmt.Sprintf("providers.%s", name)

		if !knownProviders[name] {
			errs = append(errs, ValidationError{
				Path:    path,
				Message: fmt.Sprintf("unknown provider %q", name),
			})
		}

		if !p.Disabled && p.APIKey == "" && models.ProviderRequiresAPIKey(name) {
			errs = append(errs, ValidationError{
				Path:    path + ".api_key",
				Message: "API key is required",
			})
		}
	}

	return errs
}

func validateAgents(cfg *Config) []ValidationError {
	var errs []ValidationError

	for name, agent := range cfg.Agents {
		path := fmt.Sprintf("agents.%s", name)

		// Validate agent name
		if _, valid := ParseAgentName(string(name)); !valid {
			errs = append(errs, ValidationError{
				Path:    path,
				Message: fmt.Sprintf("unknown agent name %q", name),
			})
		}

		// Validate provider reference
		if agent.Provider != "" {
			if _, exists := cfg.Providers[agent.Provider]; !exists {
				// Provider might be configured via env vars later, just warn if unknown
				if !knownProviders[agent.Provider] {
					errs = append(errs, ValidationError{
						Path:    path + ".provider",
						Message: fmt.Sprintf("references unknown provider %q", agent.Provider),
					})
				}
			}
		}

		// Validate max tokens
		if agent.MaxTokens < 0 {
			errs = append(errs, ValidationError{
				Path:    path + ".max_tokens",
				Message: "must be non-negative",
			})
		}
	}

	return errs
}

func validateMCPServers(cfg *Config) []ValidationError {
	var errs []ValidationError

	for i, mcp := range cfg.MCPServers {
		path := fmt.Sprintf("mcp_servers[%d]", i)

		if mcp.Name == "" {
			errs = append(errs, ValidationError{
				Path:    path + ".name",
				Message: "name is required",
			})
		}

		hasCommand := mcp.Command != ""
		hasURL := mcp.URL != ""

		if !hasCommand && !hasURL {
			errs = append(errs, ValidationError{
				Path:    path,
				Message: "either command or url is required",
			})
		}

		if hasCommand && hasURL {
			errs = append(errs, ValidationError{
				Path:    path,
				Message: "cannot specify both command and url",
			})
		}
	}

	return errs
}

func validateHarness(cfg *Config) []ValidationError {
	var errs []ValidationError

	h := cfg.Harness
	if h.AutoCompactThresholdRatio < 0 || h.AutoCompactThresholdRatio > 1 {
		errs = append(errs, ValidationError{
			Path:    "harness.auto_compact_threshold_ratio",
			Message: fmt.Sprintf("must be between 0 and 1, got %f", h.AutoCompactThresholdRatio),
		})
	}
	if h.AutoCompactBufferTokens < 0 {
		errs = append(errs, ValidationError{
			Path:    "harness.auto_compact_buffer_tokens",
			Message: "must be non-negative",
		})
	}
	if h.CompactBlockingBufferTokens < 0 {
		errs = append(errs, ValidationError{
			Path:    "harness.compact_blocking_buffer_tokens",
			Message: "must be non-negative",
		})
	}

	return errs
}

var validPaperTypes = map[PaperType]bool{
	"":         true, // empty is valid (defaults)
	"survey":   true,
	"research": true,
	"position": true,
	"thesis":   true,
}

func validatePaperType(cfg *Config) []ValidationError {
	var errs []ValidationError

	if !validPaperTypes[cfg.PaperType] {
		errs = append(errs, ValidationError{
			Path:    "paper_type",
			Message: fmt.Sprintf("unknown paper type %q, valid types: survey, research, position, thesis", cfg.PaperType),
		})
	}

	return errs
}

func validateHooks(cfg *Config) []ValidationError {
	var errs []ValidationError
	for i, h := range cfg.Hooks {
		path := fmt.Sprintf("hooks[%d]", i)
		if h.Event == "" {
			errs = append(errs, ValidationError{Path: path + ".event", Message: "event is required"})
		} else if !hooks.ValidEvent(h.Event) {
			errs = append(errs, ValidationError{Path: path + ".event", Message: fmt.Sprintf("unknown hook event %q", h.Event)})
		}
		if h.Command == "" {
			errs = append(errs, ValidationError{Path: path + ".command", Message: "command is required"})
		}
		if h.Timeout < 0 {
			errs = append(errs, ValidationError{Path: path + ".timeout", Message: "must be non-negative"})
		}
	}
	return errs
}

func validateSubagentOrchestration(cfg *Config) []ValidationError {
	var errs []ValidationError
	s := cfg.SubagentOrchestration

	if s.GlobalMaxConcurrent < 0 {
		errs = append(errs, ValidationError{
			Path:    "subagent_orchestration.global_max_concurrent",
			Message: "must be non-negative",
		})
	}
	if s.MaxNestedDepth < 0 {
		errs = append(errs, ValidationError{
			Path:    "subagent_orchestration.max_nested_depth",
			Message: "must be non-negative",
		})
	}
	if s.DefaultResultMaxChars < 0 {
		errs = append(errs, ValidationError{
			Path:    "subagent_orchestration.default_result_max_chars",
			Message: "must be non-negative",
		})
	}
	if s.NotificationMaxChars < 0 {
		errs = append(errs, ValidationError{
			Path:    "subagent_orchestration.notification_max_chars",
			Message: "must be non-negative",
		})
	}

	for profileName, profile := range s.Profiles {
		path := fmt.Sprintf("subagent_orchestration.profiles.%s", profileName)
		if profile.MaxConcurrent < 0 {
			errs = append(errs, ValidationError{
				Path:    path + ".max_concurrent",
				Message: "must be non-negative",
			})
		}
		if profile.ResultMaxChars < 0 {
			errs = append(errs, ValidationError{
				Path:    path + ".result_max_chars",
				Message: "must be non-negative",
			})
		}
		if profile.TimeoutSeconds < 0 {
			errs = append(errs, ValidationError{
				Path:    path + ".timeout_seconds",
				Message: "must be non-negative",
			})
		}
		if profile.MaxTurns < 0 {
			errs = append(errs, ValidationError{
				Path:    path + ".max_turns",
				Message: "must be non-negative",
			})
		}
	}

	return errs
}
