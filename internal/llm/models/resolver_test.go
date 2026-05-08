package models

import "testing"

func TestResolveModelRef_ProviderColonModel(t *testing.T) {
	ref, err := ResolveModelRef("openai:gpt-4.1")
	if err != nil {
		t.Fatalf("ResolveModelRef error: %v", err)
	}
	if ref.Provider != ProviderOpenAI || ref.ModelID != "gpt-4.1" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}

func TestResolveModelRefForProvider_ProviderModelFlow(t *testing.T) {
	ref, err := ResolveModelRefForProvider(ProviderAnthropic, "sonnet")
	if err != nil {
		t.Fatalf("ResolveModelRefForProvider error: %v", err)
	}
	if ref.Provider != ProviderAnthropic || ref.ModelID != string(Claude46Sonnet) {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}

func TestResolveModelRefForProvider_CustomFallback(t *testing.T) {
	ref, err := ResolveModelRefForProvider(ProviderOllama, "qwen2.5:14b")
	if err != nil {
		t.Fatalf("ResolveModelRefForProvider error: %v", err)
	}
	if ref.Provider != ProviderOllama || ref.ModelID != "qwen2.5:14b" || ref.Source != ModelSourceFallback {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}

func TestResolveModelRefForProvider_StrictProviderUnknownErrors(t *testing.T) {
	ref, err := ResolveModelRefForProvider(ProviderOpenAI, string(Claude4Sonnet))
	if err == nil {
		t.Fatalf("expected strict provider error, got ref: %+v", ref)
	}
}

func TestResolveModelRefForProviderFallback_CrossProviderExactIDUsesRequestedProvider(t *testing.T) {
	ref, err := ResolveModelRefForProviderFallback(ProviderOpenAI, string(Claude4Sonnet))
	if err != nil {
		t.Fatalf("ResolveModelRefForProviderFallback error: %v", err)
	}
	if ref.Provider != ProviderOpenAI || ref.ModelID != string(Claude4Sonnet) || ref.Source != ModelSourceFallback {
		t.Fatalf("unexpected ref: %+v", ref)
	}
	if ref.Metadata == nil || ref.Metadata.Provider != ProviderOpenAI {
		t.Fatalf("unexpected metadata: %+v", ref.Metadata)
	}
}

func TestResolveModelRefForProvider_RouterContextKeepsRawModel(t *testing.T) {
	ref, err := ResolveModelRefForProvider(ProviderOpenRouter, "anthropic:claude-future")
	if err != nil {
		t.Fatalf("ResolveModelRefForProvider error: %v", err)
	}
	if ref.Provider != ProviderOpenRouter || ref.ModelID != "anthropic:claude-future" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
	if !ref.IsRouter {
		t.Fatalf("expected router ref: %+v", ref)
	}
}

func TestResolveModelRef_TopLevelRouterModel(t *testing.T) {
	ref, err := ResolveModelRef("openrouter:anthropic/claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("ResolveModelRef error: %v", err)
	}
	if ref.Provider != ProviderOpenRouter || ref.ModelID != "anthropic/claude-sonnet-4-20250514" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}
