package config

import (
	"context"
	"testing"

	"github.com/openscholar/openscholar/internal/llm/models"
)

func TestResolveAPIKey_EnvOverridesConfig(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-key-value")

	providerCfg := Provider{APIKey: "config-key-value"}
	key, source, err := ResolveAPIKey(models.ProviderAnthropic, providerCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "env-key-value" {
		t.Errorf("expected env key %q, got %q", "env-key-value", key)
	}
	if source != AuthEnv {
		t.Errorf("expected source %q, got %q", AuthEnv, source)
	}
}

func TestResolveAPIKey_ConfigFallback(t *testing.T) {
	// Ensure env var is not set
	t.Setenv("OPENAI_API_KEY", "")

	providerCfg := Provider{APIKey: "my-config-key"}
	key, source, err := ResolveAPIKey(models.ProviderOpenAI, providerCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "my-config-key" {
		t.Errorf("expected config key %q, got %q", "my-config-key", key)
	}
	if source != AuthConfig {
		t.Errorf("expected source %q, got %q", AuthConfig, source)
	}
}

func TestResolveAPIKey_NoKey(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")

	providerCfg := Provider{}
	_, _, err := ResolveAPIKey(models.ProviderDeepSeek, providerCfg)
	if err == nil {
		t.Fatal("expected error when no key available, got nil")
	}
}

func TestProviderEnvKey(t *testing.T) {
	tests := []struct {
		provider models.ModelProvider
		expected string
	}{
		{models.ProviderAnthropic, "ANTHROPIC_API_KEY"},
		{models.ProviderOpenAI, "OPENAI_API_KEY"},
		{models.ProviderDeepSeek, "DEEPSEEK_API_KEY"},
		{models.ProviderMiniMax, "MINIMAX_API_KEY"},
		{models.ProviderGLM, "GLM_API_KEY"},
		{models.ProviderSiliconFlow, "SILICONFLOW_API_KEY"},
		{models.ProviderOllama, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			got := providerEnvKey(tt.provider)
			if got != tt.expected {
				t.Errorf("providerEnvKey(%q) = %q, want %q", tt.provider, got, tt.expected)
			}
		})
	}
}

func TestExecuteKeyHelper_Success(t *testing.T) {
	ctx := context.Background()
	key, err := ExecuteKeyHelper(ctx, "echo test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "test-key" {
		t.Errorf("expected %q, got %q", "test-key", key)
	}
}

func TestExecuteKeyHelper_Timeout(t *testing.T) {
	ctx := context.Background()
	_, err := ExecuteKeyHelper(ctx, "sleep 10")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestExecuteKeyHelper_EmptyOutput(t *testing.T) {
	ctx := context.Background()
	_, err := ExecuteKeyHelper(ctx, "echo ''")
	if err == nil {
		t.Fatal("expected error for empty output, got nil")
	}
}
