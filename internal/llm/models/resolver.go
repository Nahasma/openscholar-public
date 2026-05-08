package models

import (
	"fmt"
	"strings"
)

type ModelSource string

const (
	ModelSourceCurated       ModelSource = "curated"
	ModelSourceProviderList  ModelSource = "provider_list"
	ModelSourceCurrentConfig ModelSource = "current_config"
	ModelSourceCustom        ModelSource = "custom"
	ModelSourceFallback      ModelSource = "fallback"
)

type ModelRef struct {
	Provider      ModelProvider
	ModelID       string
	APIModel      string
	OwnerProvider ModelProvider
	Metadata      *Model
	Source        ModelSource
	IsRouter      bool
}

func ResolveModelRef(input string) (ModelRef, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return ModelRef{}, fmt.Errorf("model input is empty")
	}
	if p, m, ok := splitProviderModel(input); ok {
		return ResolveModelRefForProvider(p, m)
	}
	m, ok := ResolveModel(input)
	if !ok {
		return ModelRef{}, fmt.Errorf("model %q not found", input)
	}
	return modelRefFromModel(m, m.Provider, ModelSourceCurated), nil
}

func ResolveModelRefFallback(input string) (ModelRef, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return ModelRef{}, fmt.Errorf("model input is empty")
	}
	if p, m, ok := splitProviderModel(input); ok {
		return ResolveModelRefForProviderFallback(p, m)
	}
	return ResolveModelRef(input)
}

func ResolveModelRefForProvider(provider ModelProvider, input string) (ModelRef, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return ModelRef{}, fmt.Errorf("model input is empty")
	}
	if m, ok := SupportedModels[ModelID(input)]; ok && m.Provider == provider {
		return modelRefFromModel(m, provider, ModelSourceCurated), nil
	}
	if m, ok := ResolveModel(input); ok && m.Provider == provider {
		return modelRefFromModel(m, provider, ModelSourceCurated), nil
	}
	if !ProviderAllowsArbitraryModel(provider) {
		return ModelRef{}, fmt.Errorf("model %q is unknown for strict provider %q", input, provider)
	}

	res, err := ResolveWithFallback(provider, input)
	if err != nil {
		return ModelRef{}, err
	}
	return ModelRef{
		Provider:      provider,
		ModelID:       string(res.Model.ID),
		APIModel:      res.Model.APIModel,
		OwnerProvider: res.Model.Provider,
		Metadata:      &res.Model,
		Source:        ModelSourceFallback,
		IsRouter:      ProviderIsRouter(provider),
	}, nil
}

func ResolveModelRefForProviderFallback(provider ModelProvider, input string) (ModelRef, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return ModelRef{}, fmt.Errorf("model input is empty")
	}
	res, err := ResolveWithFallback(provider, input)
	if err != nil {
		return ModelRef{}, err
	}
	return ModelRef{
		Provider:      provider,
		ModelID:       string(res.Model.ID),
		APIModel:      res.Model.APIModel,
		OwnerProvider: res.Model.Provider,
		Metadata:      &res.Model,
		Source:        ModelSourceFallback,
		IsRouter:      ProviderIsRouter(provider),
	}, nil
}

func modelRefFromModel(m Model, provider ModelProvider, source ModelSource) ModelRef {
	return ModelRef{
		Provider:      provider,
		ModelID:       string(m.ID),
		APIModel:      m.APIModel,
		OwnerProvider: m.Provider,
		Metadata:      &m,
		Source:        source,
		IsRouter:      ProviderIsRouter(provider),
	}
}

func splitProviderModel(input string) (ModelProvider, string, bool) {
	idx := strings.Index(input, ":")
	if idx <= 0 {
		return "", "", false
	}
	p, ok := ResolveProvider(input[:idx])
	if !ok {
		return "", "", false
	}
	modelID := strings.TrimSpace(input[idx+1:])
	if modelID == "" {
		return "", "", false
	}
	return p, modelID, true
}
