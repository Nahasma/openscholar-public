package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

// Load initialises the config singleton from workingDir, merging file and env.
func Load(workingDir string) (*Config, error) {
	if cfg != nil {
		return cfg, nil
	}

	loaded := &Config{
		WorkingDir:            workingDir,
		Data:                  DataConfig{Directory: defaultDataDirectory},
		Providers:             make(map[models.ModelProvider]Provider),
		Agents:                make(map[AgentName]Agent),
		Harness:               DefaultHarnessConfig(),
		Experiment:            DefaultExperimentConfig(),
		SubagentOrchestration: DefaultSubagentOrchestrationConfig(),
	}

	if err := mergeConfigFile(loaded, workingDir); err != nil {
		return nil, err
	}
	mergeEnvProviders(loaded)

	// Validate configuration after all sources are merged
	if validationErrs := Validate(loaded); len(validationErrs) > 0 {
		// Log warnings but don't fail — some errors may be resolved by applyAgentDefaults
		for _, ve := range validationErrs {
			fmt.Fprintf(os.Stderr, "config warning: %s\n", ve.Error())
		}
	}

	if loaded.DefaultProvider == "" {
		loaded.DefaultProvider = firstConfiguredProvider(loaded)
	}
	if loaded.DefaultProvider == "" {
		return nil, fmt.Errorf(
			"no provider configured. Set provider API keys, configure local providers (ollama/vllm), or add %s",
			ConfigFilePath(workingDir),
		)
	}

	if err := applyAgentDefaults(loaded); err != nil {
		return nil, err
	}

	// Export API keys as environment variables so bash tools (curl) can access them.
	exportAPIKeysToEnv(loaded)

	cfg = loaded
	return cfg, nil
}

func mergeConfigFile(dst *Config, workingDir string) error {
	path := ConfigFilePath(workingDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var fileCfg Config
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return fmt.Errorf("failed to parse config file %s: %w", path, err)
	}
	var rawCfg map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawCfg); err != nil {
		return fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	if fileCfg.Data.Directory != "" {
		dst.Data.Directory = fileCfg.Data.Directory
	}
	if fileCfg.Paths.Root != "" {
		dst.Paths.Root = fileCfg.Paths.Root
	}
	if fileCfg.Paths.Config != "" {
		dst.Paths.Config = fileCfg.Paths.Config
	}
	if fileCfg.Paths.State != "" {
		dst.Paths.State = fileCfg.Paths.State
	}
	if fileCfg.Paths.Extensions != "" {
		dst.Paths.Extensions = fileCfg.Paths.Extensions
	}
	if fileCfg.Paths.Cache != "" {
		dst.Paths.Cache = fileCfg.Paths.Cache
	}
	if fileCfg.Paths.Logs != "" {
		dst.Paths.Logs = fileCfg.Paths.Logs
	}
	if fileCfg.Paths.Runtime != "" {
		dst.Paths.Runtime = fileCfg.Paths.Runtime
	}
	if fileCfg.DefaultProvider != "" {
		dst.DefaultProvider = fileCfg.DefaultProvider
	}
	if fileCfg.ImageProvider != "" {
		dst.ImageProvider = fileCfg.ImageProvider
	}
	if fileCfg.Debug {
		dst.Debug = true
	}
	if fileCfg.SessionLog {
		dst.SessionLog = true
	}
	if fileCfg.PaperType != "" {
		dst.PaperType = fileCfg.PaperType
	}
	if fileCfg.OllamaBaseURL != "" {
		dst.OllamaBaseURL = fileCfg.OllamaBaseURL
	}
	if fileCfg.VLLMBaseURL != "" {
		dst.VLLMBaseURL = fileCfg.VLLMBaseURL
	}
	if len(fileCfg.MCPServers) > 0 {
		dst.MCPServers = fileCfg.MCPServers
	}
	if len(fileCfg.Hooks) > 0 {
		dst.Hooks = fileCfg.Hooks
	}
	dst.Web = fileCfg.Web
	mergeHarnessConfig(&dst.Harness, &fileCfg.Harness)
	mergeSubagentOrchestrationConfig(&dst.SubagentOrchestration, &fileCfg.SubagentOrchestration)
	if fileCfg.CodeAgent != nil {
		codeAgentCopy := *fileCfg.CodeAgent
		dst.CodeAgent = &codeAgentCopy
	}
	if rawExperiment, ok := rawCfg["experiment"]; ok {
		if err := mergeExperimentConfigJSON(&dst.Experiment, rawExperiment); err != nil {
			return fmt.Errorf("failed to merge experiment config: %w", err)
		}
	}

	for providerName, providerCfg := range fileCfg.Providers {
		dst.Providers[providerName] = providerCfg
	}
	applyLegacyProviderBaseURLs(dst)
	for agentName, agentCfg := range fileCfg.Agents {
		dst.Agents[agentName] = agentCfg
	}
	return nil
}

func applyLegacyProviderBaseURLs(dst *Config) {
	if dst == nil {
		return
	}
	if dst.OllamaBaseURL != "" {
		providerCfg := dst.Providers[models.ProviderOllama]
		if providerCfg.BaseURL == "" {
			providerCfg.BaseURL = dst.OllamaBaseURL
			dst.Providers[models.ProviderOllama] = providerCfg
		}
	}
	if dst.VLLMBaseURL != "" {
		providerCfg := dst.Providers[models.ProviderVLLM]
		if providerCfg.BaseURL == "" {
			providerCfg.BaseURL = dst.VLLMBaseURL
			dst.Providers[models.ProviderVLLM] = providerCfg
		}
	}
}

type experimentConfigPatch struct {
	OrchestratorEnabled  *bool          `json:"orchestrator_enabled"`
	StructuredFirst      *bool          `json:"structured_first"`
	DefaultTimeoutSec    *int           `json:"default_timeout_sec"`
	MaxDebugRetries      *int           `json:"max_debug_retries"`
	MaxAgentReplans      *int           `json:"max_agent_replans"`
	StreamMonitorEnabled *bool          `json:"stream_monitor_enabled"`
	IdleTimeoutSec       *int           `json:"idle_timeout_sec"`
	WorkspaceVersion     *string        `json:"workspace_version"`
	Domain               *string        `json:"domain"`
	DomainOverride       *DomainProfile `json:"domain_override"`
}

func mergeExperimentConfigJSON(dst *ExperimentConfig, raw json.RawMessage) error {
	if dst == nil || len(raw) == 0 {
		return nil
	}
	var patch experimentConfigPatch
	if err := json.Unmarshal(raw, &patch); err != nil {
		return err
	}
	mergeExperimentConfigPatch(dst, patch)
	return nil
}

func mergeExperimentConfigPatch(dst *ExperimentConfig, patch experimentConfigPatch) {
	if dst == nil {
		return
	}
	if patch.OrchestratorEnabled != nil {
		dst.OrchestratorEnabled = *patch.OrchestratorEnabled
	}
	if patch.StructuredFirst != nil {
		dst.StructuredFirst = *patch.StructuredFirst
	}
	if patch.DefaultTimeoutSec != nil && *patch.DefaultTimeoutSec > 0 {
		dst.DefaultTimeoutSec = *patch.DefaultTimeoutSec
	}
	if patch.MaxDebugRetries != nil && *patch.MaxDebugRetries > 0 {
		dst.MaxDebugRetries = *patch.MaxDebugRetries
	}
	if patch.MaxAgentReplans != nil && *patch.MaxAgentReplans > 0 {
		dst.MaxAgentReplans = *patch.MaxAgentReplans
	}
	if patch.StreamMonitorEnabled != nil {
		dst.StreamMonitorEnabled = *patch.StreamMonitorEnabled
	}
	if patch.IdleTimeoutSec != nil && *patch.IdleTimeoutSec > 0 {
		dst.IdleTimeoutSec = *patch.IdleTimeoutSec
	}
	if patch.WorkspaceVersion != nil && *patch.WorkspaceVersion != "" {
		dst.WorkspaceVersion = *patch.WorkspaceVersion
	}
	if patch.Domain != nil && *patch.Domain != "" {
		dst.Domain = *patch.Domain
	}
	if patch.DomainOverride != nil {
		dst.DomainOverride = patch.DomainOverride
	}
}

func firstConfiguredProvider(cfg *Config) models.ModelProvider {
	for _, spec := range models.ProviderCatalog() {
		providerName := spec.ID
		providerCfg, ok := cfg.Providers[providerName]
		if !ok || providerCfg.Disabled {
			continue
		}
		if models.ProviderRequiresAPIKey(providerName) && providerCfg.APIKey == "" {
			continue
		}
		return providerName
	}
	return ""
}

func applyAgentDefaults(cfg *Config) error {
	agentOrder := []AgentName{AgentCoder, AgentSummarizer, AgentTask, AgentTitle, AgentGeneral, AgentExplore, AgentLeader, AgentPlan, AgentVerify, AgentCoordinator}
	for _, agentName := range agentOrder {
		agentCfg := cfg.Agents[agentName]
		if agentCfg.Provider == "" {
			agentCfg.Provider = cfg.DefaultProvider
		}

		providerCfg, ok := cfg.Providers[agentCfg.Provider]
		if !ok {
			return fmt.Errorf("agent %s references unknown provider %s", agentName, agentCfg.Provider)
		}
		if providerCfg.Disabled {
			return fmt.Errorf("agent %s uses disabled provider %s", agentName, agentCfg.Provider)
		}
		if models.ProviderRequiresAPIKey(agentCfg.Provider) && providerCfg.APIKey == "" {
			return fmt.Errorf("provider %s is missing an API key", agentCfg.Provider)
		}

		if agentCfg.Model == "" {
			agentCfg.Model = defaultAgentModel(agentCfg.Provider, agentName, providerCfg.Model)
		}
		if agentCfg.Model == "" {
			return fmt.Errorf("agent %s has no model configured for provider %s", agentName, agentCfg.Provider)
		}
		if agentCfg.MaxTokens == 0 {
			agentCfg.MaxTokens = defaultMaxTokensForAgent(agentName)
		}

		cfg.Agents[agentName] = agentCfg
	}
	return nil
}

func defaultAgentModel(providerName models.ModelProvider, agentName AgentName, providerModel string) string {
	agentDefault := defaultModelForProvider(providerName, agentName)
	providerDefault := defaultModelForProvider(providerName, AgentCoder)
	if agentDefault != "" && agentDefault != providerDefault && (providerModel == "" || providerModel == providerDefault) {
		return agentDefault
	}
	if providerModel != "" {
		return providerModel
	}
	return agentDefault
}
