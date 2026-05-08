package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nahasma/openscholar-public/internal/fileop"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

// ConfigFilePath returns the absolute path to the project config file.
func ConfigFilePath(workingDir string) string {
	return filepath.Join(workingDir, configRelativePath)
}

// InitConfigFile creates the config file with a sample template if it does not exist.
func InitConfigFile(workingDir string) (string, error) {
	path := ConfigFilePath(workingDir)
	if _, err := os.Stat(path); err == nil {
		return path, fmt.Errorf("config file already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return path, fmt.Errorf("failed to check config file %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, fmt.Errorf("failed to create config directory: %w", err)
	}

	sample := defaultConfigTemplate()
	data, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		return path, fmt.Errorf("failed to render config template: %w", err)
	}
	data = append(data, '\n')

	if err := fileop.WriteFileAtomic(path, data, 0o644); err != nil {
		return path, fmt.Errorf("failed to write config file %s: %w", path, err)
	}
	return path, nil
}

// SaveAgentModel persists the agent's model choice to the project config file.
func SaveAgentModel(workingDir string, agentName AgentName, modelID string) error {
	return SaveAgentModelProvider(workingDir, agentName, "", modelID)
}

// SaveAgentModelProvider persists the agent's provider+model choice to the project config file.
func SaveAgentModelProvider(workingDir string, agentName AgentName, provider models.ModelProvider, modelID string) error {
	return saveAgentModelProvider(workingDir, agentName, provider, modelID, nil)
}

// SaveAgentModelProviderForce persists an explicit custom model choice.
func SaveAgentModelProviderForce(workingDir string, agentName AgentName, provider models.ModelProvider, modelID string) error {
	modelCfg := &ModelConfig{Name: modelID}
	return saveAgentModelProvider(workingDir, agentName, provider, modelID, modelCfg)
}

func saveAgentModelProvider(workingDir string, agentName AgentName, provider models.ModelProvider, modelID string, modelCfg *ModelConfig) error {
	path := ConfigFilePath(workingDir)
	if modelCfg == nil && provider != "" && cfg != nil {
		if providerCfg, ok := cfg.Providers[provider]; ok {
			if persistedModelCfg, ok := providerCfg.Models[modelID]; ok {
				modelCfg = &persistedModelCfg
			}
		}
	}

	var fileCfg Config
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &fileCfg)
	}

	if fileCfg.Agents == nil {
		fileCfg.Agents = make(map[AgentName]Agent)
	}
	if modelCfg != nil && provider != "" {
		if fileCfg.Providers == nil {
			fileCfg.Providers = make(map[models.ModelProvider]Provider)
		}
		providerCfg := fileCfg.Providers[provider]
		if providerCfg.Models == nil {
			providerCfg.Models = make(map[string]ModelConfig)
		}
		providerCfg.Models[modelID] = mergeModelConfig(providerCfg.Models[modelID], *modelCfg)
		fileCfg.Providers[provider] = providerCfg
	}

	agentCfg := fileCfg.Agents[agentName]
	agentCfg.Model = modelID
	if provider != "" {
		agentCfg.Provider = provider
	}
	fileCfg.Agents[agentName] = agentCfg

	os.MkdirAll(filepath.Dir(path), 0o755)

	out, _ := json.MarshalIndent(fileCfg, "", "  ")
	if err := fileop.WriteFileAtomic(path, append(out, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Sync in-memory config
	if cfg != nil {
		if cfg.Agents == nil {
			cfg.Agents = make(map[AgentName]Agent)
		}
		a := cfg.Agents[agentName]
		a.Model = modelID
		if provider != "" {
			a.Provider = provider
		}
		cfg.Agents[agentName] = a
		if modelCfg != nil && provider != "" {
			if cfg.Providers == nil {
				cfg.Providers = make(map[models.ModelProvider]Provider)
			}
			providerCfg := cfg.Providers[provider]
			if providerCfg.Models == nil {
				providerCfg.Models = make(map[string]ModelConfig)
			}
			providerCfg.Models[modelID] = mergeModelConfig(providerCfg.Models[modelID], *modelCfg)
			cfg.Providers[provider] = providerCfg
		}
	}
	return nil
}

// SaveFull writes a complete Config to the project config file.
// Used by the Hard Init wizard to persist the initial configuration.
func SaveFull(workingDir string, newCfg Config) error {
	path := ConfigFilePath(workingDir)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Don't persist WorkingDir — it's a runtime field
	saveCfg := newCfg
	saveCfg.WorkingDir = ""

	data, err := json.MarshalIndent(saveCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := fileop.WriteFileAtomic(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// SaveProviderModels persists discovered models under providers.<provider>.models.
func SaveProviderModels(workingDir string, provider models.ModelProvider, modelCfgs map[string]ModelConfig) error {
	if provider == "" {
		return fmt.Errorf("provider is required")
	}
	if len(modelCfgs) == 0 {
		return nil
	}
	if workingDir == "" {
		workingDir = WorkingDirectory()
	}
	path := ConfigFilePath(workingDir)
	var fileCfg Config
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &fileCfg)
	}
	if fileCfg.Providers == nil {
		fileCfg.Providers = make(map[models.ModelProvider]Provider)
	}
	providerCfg := fileCfg.Providers[provider]
	if providerCfg.Models == nil {
		providerCfg.Models = make(map[string]ModelConfig)
	}
	for id, cfg := range modelCfgs {
		if id == "" {
			continue
		}
		providerCfg.Models[id] = mergeModelConfig(providerCfg.Models[id], cfg)
	}
	fileCfg.Providers[provider] = providerCfg
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	out, err := json.MarshalIndent(fileCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}
	if err := fileop.WriteFileAtomic(path, append(out, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if cfg != nil {
		if cfg.Providers == nil {
			cfg.Providers = make(map[models.ModelProvider]Provider)
		}
		inMemory := cfg.Providers[provider]
		if inMemory.Models == nil {
			inMemory.Models = make(map[string]ModelConfig)
		}
		for id, mc := range modelCfgs {
			if id == "" {
				continue
			}
			inMemory.Models[id] = mergeModelConfig(inMemory.Models[id], mc)
		}
		cfg.Providers[provider] = inMemory
	}
	return nil
}

// MergeProviderModelsInMemory merges provider model metadata into the loaded
// runtime config without writing the config file.
func MergeProviderModelsInMemory(provider models.ModelProvider, modelCfgs map[string]ModelConfig) {
	if cfg == nil || provider == "" || len(modelCfgs) == 0 {
		return
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[models.ModelProvider]Provider)
	}
	providerCfg := cfg.Providers[provider]
	if providerCfg.Models == nil {
		providerCfg.Models = make(map[string]ModelConfig)
	}
	for id, mc := range modelCfgs {
		if id == "" {
			continue
		}
		providerCfg.Models[id] = mergeModelConfig(providerCfg.Models[id], mc)
	}
	cfg.Providers[provider] = providerCfg
}

func mergeModelConfig(existing, incoming ModelConfig) ModelConfig {
	merged := existing
	if incoming.Name != "" {
		merged.Name = incoming.Name
	}
	if incoming.ContextWindow > 0 {
		merged.ContextWindow = incoming.ContextWindow
	}
	if incoming.DefaultMaxTokens > 0 {
		merged.DefaultMaxTokens = incoming.DefaultMaxTokens
	}
	if incoming.SupportsTools != nil {
		merged.SupportsTools = incoming.SupportsTools
	}
	if incoming.SupportsReasoning != nil {
		merged.SupportsReasoning = incoming.SupportsReasoning
	}
	if incoming.SupportsVision != nil {
		merged.SupportsVision = incoming.SupportsVision
	}
	return merged
}
