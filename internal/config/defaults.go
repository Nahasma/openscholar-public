package config

import "github.com/openscholar/openscholar/internal/llm/models"

// DefaultExperimentConfig returns the default ExperimentConfig values.
func DefaultExperimentConfig() ExperimentConfig {
	return ExperimentConfig{
		OrchestratorEnabled:  false,
		StructuredFirst:      true,
		DefaultTimeoutSec:    7200,
		MaxDebugRetries:      4,
		MaxAgentReplans:      2,
		StreamMonitorEnabled: false,
		IdleTimeoutSec:       300,
		WorkspaceVersion:     "v1",
	}
}

// DefaultSubagentOrchestrationConfig returns conservative defaults for subagent orchestration.
func DefaultSubagentOrchestrationConfig() SubagentOrchestrationConfig {
	return SubagentOrchestrationConfig{
		Enabled:               false,
		GlobalMaxConcurrent:   4,
		MaxNestedDepth:        2,
		DefaultResultMaxChars: 12000,
		NotificationMaxChars:  6000,
	}
}

// DefaultHarnessConfig returns the default Phase 8 harness configuration.
// All values are conservative: new features are off or set to safe defaults.
func DefaultHarnessConfig() HarnessConfig {
	return HarnessConfig{
		MicroCompactEnabled:         true,
		MicroCompactKeepRecent:      3,
		AutoCompactThresholdRatio:   0,
		AutoCompactBufferTokens:     13000,
		CompactBlockingBufferTokens: 3000,
		PromptTierEnabled:           false,
		AsyncToolsEnabled:           false,
		AsyncToolsList:              []string{"scholar_search", "arxiv_search", "crossref_search", "pubmed_search", "openalex_search", "core_search", "eric_search", "patentsview_search", "europepmc_search", "unpaywall_lookup"},
		LintGuardEnabled:            false,
		LintGuardLatex:              true,
		LintGuardGo:                 true,
		WorktreeEnabled:             false,
		EventJournalEnabled:         true,
	}
}

// mergeHarnessConfig merges file-based harness config into dst.
// Only overrides non-zero values from file config.
func mergeHarnessConfig(dst *HarnessConfig, src *HarnessConfig) {
	if src.MicroCompactKeepRecent > 0 {
		dst.MicroCompactKeepRecent = src.MicroCompactKeepRecent
	}
	if src.AutoCompactThresholdRatio > 0 {
		dst.AutoCompactThresholdRatio = src.AutoCompactThresholdRatio
		if src.AutoCompactBufferTokens <= 0 {
			dst.AutoCompactBufferTokens = 0
		}
	}
	if src.AutoCompactBufferTokens > 0 {
		dst.AutoCompactBufferTokens = src.AutoCompactBufferTokens
	}
	if src.CompactBlockingBufferTokens > 0 {
		dst.CompactBlockingBufferTokens = src.CompactBlockingBufferTokens
	}
	if len(src.AsyncToolsList) > 0 {
		dst.AsyncToolsList = src.AsyncToolsList
	}
	// Boolean fields: only override if file explicitly sets them (Go zero-value is false)
	// Since JSON unmarshal defaults to false, we can't distinguish "not set" from "set to false"
	// for booleans. The convention is: defaults are set in DefaultHarnessConfig(),
	// and file config can explicitly override to true.
	if src.MicroCompactEnabled {
		dst.MicroCompactEnabled = true
	}
	if src.PromptTierEnabled {
		dst.PromptTierEnabled = true
	}
	if src.AsyncToolsEnabled {
		dst.AsyncToolsEnabled = true
	}
	if src.LintGuardEnabled {
		dst.LintGuardEnabled = true
	}
	if src.WorktreeEnabled {
		dst.WorktreeEnabled = true
	}
	if src.EventJournalEnabled {
		dst.EventJournalEnabled = true
	}
	// Allow disabling defaults that are true by default
	if !src.LintGuardLatex {
		dst.LintGuardLatex = false
	}
	if !src.LintGuardGo {
		dst.LintGuardGo = false
	}
}

// mergeSubagentOrchestrationConfig merges file-based subagent_orchestration config into dst.
// Defaults stay in place when source fields are omitted or zero.
func mergeSubagentOrchestrationConfig(dst *SubagentOrchestrationConfig, src *SubagentOrchestrationConfig) {
	if dst == nil || src == nil {
		return
	}
	if src.Enabled {
		dst.Enabled = true
	}
	if src.GlobalMaxConcurrent > 0 {
		dst.GlobalMaxConcurrent = src.GlobalMaxConcurrent
	}
	if src.MaxNestedDepth > 0 {
		dst.MaxNestedDepth = src.MaxNestedDepth
	}
	if src.DefaultResultMaxChars > 0 {
		dst.DefaultResultMaxChars = src.DefaultResultMaxChars
	}
	if src.NotificationMaxChars > 0 {
		dst.NotificationMaxChars = src.NotificationMaxChars
	}
	if len(src.Profiles) > 0 {
		if dst.Profiles == nil {
			dst.Profiles = make(map[string]SubagentProfileConfig, len(src.Profiles))
		}
		for k, v := range src.Profiles {
			dst.Profiles[k] = v
		}
	}
}

func defaultModelForProvider(providerName models.ModelProvider, agentName AgentName) string {
	if providerName == models.ProviderOpenAI {
		switch agentName {
		case AgentSummarizer, AgentTask, AgentTitle, AgentExplore, AgentPlan, AgentVerify:
			return string(models.GPT41Mini)
		default:
			return string(models.GPT41)
		}
	}
	spec, ok := models.ProviderSpecByID(providerName)
	if !ok || spec.AllowArbitrary {
		return ""
	}
	return spec.DefaultModel
}

func defaultMaxTokensForAgent(agentName AgentName) int64 {
	switch agentName {
	case AgentTitle:
		return 80
	case AgentSummarizer, AgentTask, AgentGeneral, AgentExplore, AgentLeader, AgentPlan, AgentVerify, AgentCoordinator:
		return 8192
	default:
		return 16384
	}
}

func defaultConfigTemplate() Config {
	providers := make(map[models.ModelProvider]Provider)
	for _, spec := range models.ProviderCatalog() {
		if spec.DefaultModel == "" && spec.ID != models.ProviderOpenAICompatible {
			continue
		}
		baseURL := spec.Endpoints.DefaultBaseURL
		if spec.Kind == models.ProviderKindNativeAnthropic || spec.Kind == models.ProviderKindNativeOpenAI {
			baseURL = ""
		}
		providers[spec.ID] = Provider{
			BaseURL: baseURL,
			Model:   spec.DefaultModel,
			Kind:    string(spec.Kind),
		}
	}
	if p := providers[models.ProviderOpenAICompatible]; p.BaseURL == "" {
		p.BaseURL = "https://example.com/v1"
		p.Model = "qwen-plus"
		providers[models.ProviderOpenAICompatible] = p
	}

	return Config{
		Data:            DataConfig{Directory: defaultDataDirectory},
		Paths:           PathsConfig{Root: defaultDataDirectory},
		DefaultProvider: models.ProviderOpenAI,
		Providers:       providers,
		Agents: map[AgentName]Agent{
			AgentTitle: {
				Model:     string(models.GPT41Mini),
				MaxTokens: defaultMaxTokensForAgent(AgentTitle),
			},
		},
	}
}
