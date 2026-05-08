package config

import (
	"testing"

	"github.com/openscholar/openscholar/internal/llm/models"
)

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		DefaultProvider: models.ProviderAnthropic,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderAnthropic: {APIKey: "sk-test"},
		},
		Agents: map[AgentName]Agent{
			AgentCoder: {Provider: models.ProviderAnthropic, Model: "claude-4", MaxTokens: 8192},
		},
		PaperType: "research",
		Harness:   HarnessConfig{AutoCompactThresholdRatio: 0.8},
	}

	errs := Validate(cfg)
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestValidate_UnknownProvider(t *testing.T) {
	cfg := &Config{
		DefaultProvider: "fake_provider",
		Providers:       map[models.ModelProvider]Provider{},
		Agents:          map[AgentName]Agent{},
	}

	errs := Validate(cfg)
	found := false
	for _, e := range errs {
		if e.Path == "default_provider" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected validation error for unknown defaultProvider")
	}
}

func TestValidate_AgentReferencesUnknownProvider(t *testing.T) {
	cfg := &Config{
		Providers: map[models.ModelProvider]Provider{
			models.ProviderAnthropic: {APIKey: "sk-test"},
		},
		Agents: map[AgentName]Agent{
			AgentCoder: {Provider: "nonexistent"},
		},
	}

	errs := Validate(cfg)
	found := false
	for _, e := range errs {
		if e.Path == "agents.coder.provider" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected validation error for agent referencing unknown provider")
	}
}

func TestValidate_NegativeMaxTokens(t *testing.T) {
	cfg := &Config{
		Providers: map[models.ModelProvider]Provider{},
		Agents: map[AgentName]Agent{
			AgentCoder: {Provider: models.ProviderAnthropic, MaxTokens: -1},
		},
	}

	errs := Validate(cfg)
	found := false
	for _, e := range errs {
		if e.Path == "agents.coder.max_tokens" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected validation error for negative maxTokens")
	}
}

func TestValidate_MCPServerMissingCommandAndURL(t *testing.T) {
	cfg := &Config{
		Providers: map[models.ModelProvider]Provider{},
		Agents:    map[AgentName]Agent{},
		MCPServers: []MCPServerConfig{
			{Name: "test"},
		},
	}

	errs := Validate(cfg)
	found := false
	for _, e := range errs {
		if e.Path == "mcp_servers[0]" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected validation error for MCP server without command or url")
	}
}

func TestValidate_MCPServerBothCommandAndURL(t *testing.T) {
	cfg := &Config{
		Providers: map[models.ModelProvider]Provider{},
		Agents:    map[AgentName]Agent{},
		MCPServers: []MCPServerConfig{
			{Name: "test", Command: "cmd", URL: "http://localhost"},
		},
	}

	errs := Validate(cfg)
	found := false
	for _, e := range errs {
		if e.Path == "mcp_servers[0]" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected validation error for MCP server with both command and url")
	}
}

func TestValidate_InvalidHarnessThreshold(t *testing.T) {
	cfg := &Config{
		Providers: map[models.ModelProvider]Provider{},
		Agents:    map[AgentName]Agent{},
		Harness:   HarnessConfig{AutoCompactThresholdRatio: 1.5},
	}

	errs := Validate(cfg)
	found := false
	for _, e := range errs {
		if e.Path == "harness.auto_compact_threshold_ratio" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected validation error for invalid harness threshold")
	}
}

func TestValidate_InvalidHarnessBuffers(t *testing.T) {
	cfg := &Config{
		Providers: map[models.ModelProvider]Provider{},
		Agents:    map[AgentName]Agent{},
		Harness: HarnessConfig{
			AutoCompactBufferTokens:     -1,
			CompactBlockingBufferTokens: -2,
		},
	}
	errs := Validate(cfg)
	foundAuto := false
	foundBlocking := false
	for _, e := range errs {
		if e.Path == "harness.auto_compact_buffer_tokens" {
			foundAuto = true
		}
		if e.Path == "harness.compact_blocking_buffer_tokens" {
			foundBlocking = true
		}
	}
	if !foundAuto || !foundBlocking {
		t.Fatalf("expected harness buffer validation errors, got %v", errs)
	}
}

func TestValidate_InvalidSubagentOrchestrationNumbers(t *testing.T) {
	cfg := &Config{
		Providers: map[models.ModelProvider]Provider{},
		Agents:    map[AgentName]Agent{},
		SubagentOrchestration: SubagentOrchestrationConfig{
			GlobalMaxConcurrent:   -1,
			MaxNestedDepth:        -1,
			DefaultResultMaxChars: -1,
			NotificationMaxChars:  -1,
		},
	}

	errs := Validate(cfg)
	foundGlobal := false
	foundDepth := false
	foundResult := false
	foundNotification := false
	for _, e := range errs {
		switch e.Path {
		case "subagent_orchestration.global_max_concurrent":
			foundGlobal = true
		case "subagent_orchestration.max_nested_depth":
			foundDepth = true
		case "subagent_orchestration.default_result_max_chars":
			foundResult = true
		case "subagent_orchestration.notification_max_chars":
			foundNotification = true
		}
	}
	if !foundGlobal || !foundDepth || !foundResult || !foundNotification {
		t.Fatalf("expected subagent orchestration validation errors, got %v", errs)
	}
}

func TestValidate_InvalidPaperType(t *testing.T) {
	cfg := &Config{
		Providers: map[models.ModelProvider]Provider{},
		Agents:    map[AgentName]Agent{},
		PaperType: "invalid_type",
	}

	errs := Validate(cfg)
	found := false
	for _, e := range errs {
		if e.Path == "paper_type" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected validation error for invalid paper type")
	}
}

func TestValidate_ProviderMissingAPIKey(t *testing.T) {
	cfg := &Config{
		Providers: map[models.ModelProvider]Provider{
			models.ProviderAnthropic: {APIKey: ""},
		},
		Agents: map[AgentName]Agent{},
	}

	errs := Validate(cfg)
	found := false
	for _, e := range errs {
		if e.Path == "providers.anthropic.api_key" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected validation error for missing API key")
	}
}

func TestValidate_OllamaNoAPIKeyRequired(t *testing.T) {
	cfg := &Config{
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOllama: {APIKey: ""},
		},
		Agents: map[AgentName]Agent{},
	}

	errs := Validate(cfg)
	for _, e := range errs {
		if e.Path == "providers.ollama.apiKey" {
			t.Fatal("should not require API key for Ollama provider")
		}
	}
}

func TestValidate_AggregatesErrors(t *testing.T) {
	cfg := &Config{
		DefaultProvider: "bad_provider",
		Providers:       map[models.ModelProvider]Provider{},
		Agents: map[AgentName]Agent{
			AgentCoder: {MaxTokens: -1},
		},
		PaperType: "invalid",
		Harness:   HarnessConfig{AutoCompactThresholdRatio: 2.0},
	}

	errs := Validate(cfg)
	if len(errs) < 3 {
		t.Fatalf("expected at least 3 aggregated errors, got %d: %v", len(errs), errs)
	}
}
