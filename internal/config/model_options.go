package config

import (
	"sort"

	"github.com/openscholar/openscholar/internal/llm/models"
)

// ModelFromConfig converts a persisted provider model entry into runtime model
// metadata. Empty metadata stays unknown; callers should not infer capabilities
// from the default custom-model values.
func ModelFromConfig(providerName models.ModelProvider, modelID string, modelCfg ModelConfig) models.Model {
	model := models.NewCustomModel(providerName, modelID)
	if modelCfg.Name != "" {
		model.Name = modelCfg.Name
	}
	if modelCfg.ContextWindow > 0 {
		model.ContextWindow = modelCfg.ContextWindow
	}
	if modelCfg.DefaultMaxTokens > 0 {
		model.DefaultMaxTokens = modelCfg.DefaultMaxTokens
	}
	if modelCfg.SupportsReasoning != nil {
		model.CanReason = *modelCfg.SupportsReasoning
	}
	model.MetadataState = models.ModelMetadataPartial
	if modelCfg.ContextWindow == 0 && modelCfg.DefaultMaxTokens == 0 && modelCfg.SupportsReasoning == nil &&
		modelCfg.SupportsTools == nil && modelCfg.SupportsVision == nil {
		model.MetadataState = models.ModelMetadataUnknown
	}
	return model
}

// ConfiguredModelsForProvider returns persisted provider-specific models in a
// deterministic order. The provider's current Model field is included even when
// it has no explicit Models entry.
func ConfiguredModelsForProvider(cfg *Config, providerName models.ModelProvider) []models.Model {
	if cfg == nil {
		return nil
	}
	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		return nil
	}

	ids := make([]string, 0, len(providerCfg.Models)+1)
	seen := make(map[string]struct{}, len(providerCfg.Models)+1)
	if providerCfg.Model != "" {
		ids = append(ids, providerCfg.Model)
		seen[providerCfg.Model] = struct{}{}
	}
	for id := range providerCfg.Models {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		ids = append(ids, id)
		seen[id] = struct{}{}
	}
	if providerCfg.Model == "" {
		sort.Strings(ids)
	} else if len(ids) > 1 {
		head := ids[:1]
		tail := append([]string(nil), ids[1:]...)
		sort.Strings(tail)
		ids = append(head, tail...)
	}

	out := make([]models.Model, 0, len(ids))
	for _, id := range ids {
		out = append(out, ModelFromConfig(providerName, id, providerCfg.Models[id]))
	}
	return out
}

// ModelOptionsForProvider returns the merged model options used by commands and
// TUI selection: curated registry first, then current config, then discoveries.
func ModelOptionsForProvider(cfg *Config, providerName models.ModelProvider, discovered []models.Model) []models.ModelOption {
	return models.MergeModelOptions(
		models.OrderedModelsByProvider(providerName),
		ConfiguredModelsForProvider(cfg, providerName),
		discovered,
	)
}
