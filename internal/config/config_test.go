package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/openscholar/openscholar/internal/hooks"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAgentName(t *testing.T) {
	tests := []struct {
		input    string
		expected AgentName
		ok       bool
	}{
		{"coder", AgentCoder, true},
		{"Coder", AgentCoder, true},
		{"summarizer", AgentSummarizer, true},
		{"task", AgentTask, true},
		{"title", AgentTitle, true},
		{"general", AgentGeneral, true},
		{"explore", AgentExplore, true},
		{"leader", AgentLeader, true},
		{"plan", AgentPlan, true},
		{"verify", AgentVerify, true},
		{"coordinator", AgentCoordinator, true},
		{"invalid", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseAgentName(tt.input)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestConfigFilePath(t *testing.T) {
	path := ConfigFilePath("/home/user/project")
	assert.Equal(t, filepath.Join("/home/user/project", ".openscholar", "config.json"), path)
}

func TestLoad_WithConfigFile(t *testing.T) {
	Reset() // clear singleton
	defer Reset()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))

	cfgData := Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: {
				APIKey: "test-key",
				Model:  string(models.GPT41),
			},
		},
	}
	data, _ := json.MarshalIndent(cfgData, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644))

	loaded, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, models.ProviderOpenAI, loaded.DefaultProvider)
	assert.Equal(t, "test-key", loaded.Providers[models.ProviderOpenAI].APIKey)
}

func TestLoad_EnvOverride(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))

	// Write config file with one provider
	cfgData := Config{
		Providers: map[models.ModelProvider]Provider{
			models.ProviderAnthropic: {
				APIKey: "file-key",
				Model:  string(models.Claude4Sonnet),
			},
		},
	}
	data, _ := json.MarshalIndent(cfgData, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644))

	// Set env to override
	t.Setenv("ANTHROPIC_API_KEY", "env-key")

	loaded, err := Load(dir)
	require.NoError(t, err)
	// Env should take precedence
	assert.Equal(t, "env-key", loaded.Providers[models.ProviderAnthropic].APIKey)
}

func TestLoad_NoProvider_Error(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	// Clear all provider env vars
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("MINIMAX_API_KEY", "")
	t.Setenv("GLM_API_KEY", "")
	t.Setenv("OPENAI_COMPATIBLE_API_KEY", "")

	_, err := Load(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider configured")
}

func TestGetAndReset(t *testing.T) {
	Reset()
	assert.Nil(t, Get())
}

func TestDefaultMaxTokensForAgent(t *testing.T) {
	assert.Equal(t, int64(80), defaultMaxTokensForAgent(AgentTitle))
	assert.Equal(t, int64(8192), defaultMaxTokensForAgent(AgentSummarizer))
	assert.Equal(t, int64(8192), defaultMaxTokensForAgent(AgentLeader))
	assert.Equal(t, int64(8192), defaultMaxTokensForAgent(AgentPlan))
	assert.Equal(t, int64(8192), defaultMaxTokensForAgent(AgentVerify))
	assert.Equal(t, int64(8192), defaultMaxTokensForAgent(AgentCoordinator))
	assert.Equal(t, int64(16384), defaultMaxTokensForAgent(AgentCoder))
}

func TestDefaultModelForProvider(t *testing.T) {
	assert.Equal(t, string(models.Claude4Sonnet), defaultModelForProvider(models.ProviderAnthropic, AgentCoder))
	assert.Equal(t, string(models.GPT41Mini), defaultModelForProvider(models.ProviderOpenAI, AgentSummarizer))
	assert.Equal(t, string(models.GPT41Mini), defaultModelForProvider(models.ProviderOpenAI, AgentTask))
	assert.Equal(t, string(models.GPT41), defaultModelForProvider(models.ProviderOpenAI, AgentCoder))
	assert.Equal(t, "", defaultModelForProvider(models.ProviderOpenAICompatible, AgentCoder))
}

func TestLoad_OpenAISummarizerUsesSmallFastDefault(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))

	cfgData := Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: {APIKey: "test-key", Model: string(models.GPT41)},
		},
	}
	data, _ := json.MarshalIndent(cfgData, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644))

	loaded, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, string(models.GPT41), loaded.Agents[AgentCoder].Model)
	assert.Equal(t, string(models.GPT41Mini), loaded.Agents[AgentSummarizer].Model)
}

func TestLoad_OpenAISummarizerInheritsCustomProviderModel(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))

	cfgData := Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: {APIKey: "test-key", Model: "future-model"},
		},
	}
	data, _ := json.MarshalIndent(cfgData, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644))

	loaded, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "future-model", loaded.Agents[AgentSummarizer].Model)
}

func TestLoad_ConfigMergesExperimentAndCodeAgent(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))

	cfgData := Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: {APIKey: "test-key", Model: string(models.GPT41)},
		},
		CodeAgent: &CodeAgentConfig{
			Preferred:      "codex",
			TimeoutSeconds: 900,
			Providers: map[string]CodeAgentProviderConfig{
				"codex": {AllowedTools: "bash,edit", MaxTurns: 7},
			},
		},
		Experiment: ExperimentConfig{
			OrchestratorEnabled: true,
			StructuredFirst:     false,
			DefaultTimeoutSec:   321,
			WorkspaceVersion:    "v2",
		},
	}
	data, _ := json.MarshalIndent(cfgData, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644))

	loaded, err := Load(dir)
	require.NoError(t, err)
	require.NotNil(t, loaded.CodeAgent)
	assert.Equal(t, "codex", loaded.CodeAgent.Preferred)
	assert.Equal(t, 900, loaded.CodeAgent.TimeoutSeconds)
	assert.Equal(t, 7, loaded.CodeAgent.Providers["codex"].MaxTurns)
	assert.True(t, loaded.Experiment.OrchestratorEnabled)
	assert.False(t, loaded.Experiment.StructuredFirst)
	assert.Equal(t, 321, loaded.Experiment.DefaultTimeoutSec)
	assert.Equal(t, "v2", loaded.Experiment.WorkspaceVersion)
}

func TestLoad_ConfigWithoutExperimentKeepsExperimentDefaults(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))

	data := []byte(`{
  "defaultProvider": "openai",
  "providers": {
    "openai": {
      "apiKey": "test-key",
      "model": "gpt-4.1"
    }
  }
}`)
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644))

	loaded, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, DefaultExperimentConfig(), loaded.Experiment)
}

func TestLoad_ConfigWithoutSubagentOrchestrationKeepsDefaults(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))

	data := []byte(`{
  "defaultProvider": "openai",
  "providers": {
    "openai": {
      "apiKey": "test-key",
      "model": "gpt-4.1"
    }
  }
}`)
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644))

	loaded, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, DefaultSubagentOrchestrationConfig(), loaded.SubagentOrchestration)
}

func TestLoad_ConfigMergesSubagentOrchestration(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))

	data := []byte(`{
  "defaultProvider": "openai",
  "providers": {
    "openai": {
      "apiKey": "test-key",
      "model": "gpt-4.1"
    }
  },
  "subagent_orchestration": {
    "enabled": true,
    "global_max_concurrent": 7,
    "profiles": {
      "custom": {
        "permission_mode": "plan"
      }
    }
  }
}`)
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644))

	loaded, err := Load(dir)
	require.NoError(t, err)
	assert.True(t, loaded.SubagentOrchestration.Enabled)
	assert.Equal(t, 7, loaded.SubagentOrchestration.GlobalMaxConcurrent)
	assert.Equal(t, 2, loaded.SubagentOrchestration.MaxNestedDepth)
	assert.Equal(t, "plan", loaded.SubagentOrchestration.Profiles["custom"].PermissionMode)
}

func TestLoad_ConfigMergesWebProxyConfig(t *testing.T) {
	Reset()
	defer Reset()

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".openscholar")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))

	data := []byte(`{
  "defaultProvider": "openai",
  "providers": {
    "openai": {
      "apiKey": "test-key",
      "model": "gpt-4.1"
    }
  },
  "web": {
    "searchMaxUses": 3,
    "proxy": {
      "mode": "explicit",
      "url": "http://127.0.0.1:7890",
      "fakeIPCIDRs": ["198.18.0.0/15"],
      "allowLocalProxy": true
    }
  }
}`)
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0o644))

	loaded, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 3, loaded.Web.SearchMaxUses)
	assert.Equal(t, "explicit", loaded.Web.Proxy.Mode)
	assert.Equal(t, "http://127.0.0.1:7890", loaded.Web.Proxy.URL)
	assert.Equal(t, []string{"198.18.0.0/15"}, loaded.Web.Proxy.FakeIPCIDRs)
	if loaded.Web.Proxy.AllowLocalProxy == nil || !*loaded.Web.Proxy.AllowLocalProxy {
		t.Fatalf("expected allowLocalProxy=true, got %#v", loaded.Web.Proxy.AllowLocalProxy)
	}
}

func TestValidateHooks(t *testing.T) {
	errs := Validate(&Config{
		Hooks: []hooks.HookConfig{
			{Event: hooks.PreToolUse, Command: "echo ok", Timeout: 1},
			{Command: "missing event"},
			{Event: hooks.PostToolUse, Timeout: -1},
			{Event: hooks.Event("post-tool-use"), Command: "echo typo"},
		},
	})
	var paths []string
	for _, err := range errs {
		paths = append(paths, err.Path)
	}
	assert.Contains(t, paths, "hooks[1].event")
	assert.Contains(t, paths, "hooks[2].command")
	assert.Contains(t, paths, "hooks[2].timeout")
	assert.Contains(t, paths, "hooks[3].event")
}

func TestProviderProfileAuthModeJSONRoundTrip(t *testing.T) {
	in := Config{
		Providers: map[models.ModelProvider]Provider{
			models.ProviderMiniMax: {
				APIKey:   "test-key",
				BaseURL:  "https://api.minimaxi.com/anthropic",
				Profile:  "token-plan",
				AuthMode: "bearer",
				Model:    string(models.MiniMaxM27),
			},
		},
	}

	data, err := json.Marshal(in)
	require.NoError(t, err)

	var out Config
	require.NoError(t, json.Unmarshal(data, &out))
	got := out.Providers[models.ProviderMiniMax]
	assert.Equal(t, "token-plan", got.Profile)
	assert.Equal(t, "bearer", got.AuthMode)
}
