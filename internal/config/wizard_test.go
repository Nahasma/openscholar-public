package config

import (
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/hooks"
	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

func TestConfigWizardBuildConfig_PreservesExtendedFields(t *testing.T) {
	orig := &Config{
		WorkingDir:      "/tmp/wd",
		Data:            DataConfig{Directory: ".openscholar"},
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: {
				APIKey: "k1",
				Model:  "gpt-4.1",
				Kind:   string(models.ProviderKindNativeOpenAI),
				Models: map[string]ModelConfig{
					"custom": {Name: "Custom", ContextWindow: 1234},
				},
			},
		},
		Agents: map[AgentName]Agent{
			AgentCoder: {Provider: models.ProviderOpenAI, Model: "gpt-4.1"},
		},
		CodeAgent:  &CodeAgentConfig{Preferred: "codex"},
		Harness:    DefaultHarnessConfig(),
		Experiment: DefaultExperimentConfig(),
		SubagentOrchestration: SubagentOrchestrationConfig{
			Enabled:             true,
			GlobalMaxConcurrent: 3,
			Profiles: map[string]SubagentProfileConfig{
				"general": {PermissionMode: "plan"},
			},
		},
		Scholar: ScholarConfig{ContactEmail: "a@b.com"},
		Hooks:   []hooks.HookConfig{{Event: hooks.PostToolUse, Command: "echo ok"}},
		Web:     WebConfig{SearchMaxUses: 5},
	}
	w := NewConfigWizard(orig)
	got := w.BuildConfig()
	if got.CodeAgent == nil || got.CodeAgent.Preferred != "codex" {
		t.Fatal("code_agent not preserved")
	}
	if got.Web.SearchMaxUses != 5 {
		t.Fatal("web config not preserved")
	}
	if !got.SubagentOrchestration.Enabled || got.SubagentOrchestration.GlobalMaxConcurrent != 3 || got.SubagentOrchestration.Profiles["general"].PermissionMode != "plan" {
		t.Fatal("subagent orchestration config not preserved")
	}
	if len(got.Hooks) != 1 {
		t.Fatal("hooks not preserved")
	}
	if got.Providers[models.ProviderOpenAI].Kind != string(models.ProviderKindNativeOpenAI) {
		t.Fatal("provider kind not preserved")
	}
	if got.Providers[models.ProviderOpenAI].Models["custom"].ContextWindow != 1234 {
		t.Fatal("provider model config not preserved")
	}
}

func TestConfigWizardConfiguredProviders_UsesCatalogAuthMode(t *testing.T) {
	orig := &Config{
		DefaultProvider: models.ProviderOpenAICompatible,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAICompatible: {BaseURL: "http://localhost:9000/v1", Model: "custom-model"},
		},
	}
	w := NewConfigWizard(orig)

	var hasCompatible, hasOllama bool
	for _, prov := range w.ConfiguredProviders() {
		if prov == models.ProviderOpenAICompatible {
			hasCompatible = true
		}
		if prov == models.ProviderOllama {
			hasOllama = true
		}
	}
	if !hasCompatible {
		t.Fatal("openai_compatible with baseURL/model should be configured without an API key")
	}
	if hasOllama {
		t.Fatal("absent ollama provider should not be selectable as configured")
	}
}

func TestConfigWizardOpenAICompatibleBaseURLWithoutAPIKey(t *testing.T) {
	orig := &Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: {APIKey: "k1", Model: "gpt-4.1"},
		},
	}
	w := NewConfigWizard(orig)

	w.Apply(string(models.ProviderOpenAICompatible))
	if step := w.CurrentStep(); step == nil || step.Type != CWStepProviderKey {
		t.Fatalf("expected provider key step, got %+v", step)
	}
	w.Apply("")
	if step := w.CurrentStep(); step == nil || step.Type != CWStepProviderBaseURL {
		t.Fatalf("expected baseURL step after blank optional key, got %+v", step)
	}
	w.Apply("http://localhost:9000/v1")

	var hasCompatible bool
	for _, prov := range w.ConfiguredProviders() {
		if prov == models.ProviderOpenAICompatible {
			hasCompatible = true
		}
	}
	if !hasCompatible {
		t.Fatal("openai_compatible should be configured after baseURL-only entry")
	}
	got := w.BuildConfig()
	if got.Providers[models.ProviderOpenAICompatible].BaseURL != "http://localhost:9000/v1" {
		t.Fatalf("baseURL not saved: %+v", got.Providers[models.ProviderOpenAICompatible])
	}
}

func TestConfigWizardOpenAICompatibleRequiresBaseURL(t *testing.T) {
	orig := &Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: {APIKey: "k1", Model: "gpt-4.1"},
		},
	}
	w := NewConfigWizard(orig)

	w.Apply(string(models.ProviderOpenAICompatible))
	w.Apply("compat-key")
	w.Apply("")

	for _, prov := range w.ConfiguredProviders() {
		if prov == models.ProviderOpenAICompatible {
			t.Fatal("openai_compatible should not be configured without a baseURL")
		}
	}
}

func TestConfigWizardProviderKeyStep_IsSensitiveInput(t *testing.T) {
	orig := &Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: {APIKey: "sk-test"},
		},
	}
	w := NewConfigWizard(orig)
	w.Apply(string(models.ProviderOpenAI))

	step := w.CurrentStep()
	if step == nil {
		t.Fatal("expected provider key step, got nil")
	}
	if step.Type != CWStepProviderKey {
		t.Fatalf("expected provider key step, got %v", step.Type)
	}
	if step.InputKind != CWInputKindSensitive {
		t.Fatalf("expected sensitive input kind, got %v", step.InputKind)
	}
}

func TestConfigWizardProviderMenuUsesStructuredDisplayMetadata(t *testing.T) {
	cfg := &Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: {APIKey: "sk-test-123456"},
		},
	}
	w := NewConfigWizard(cfg)
	step := w.CurrentStep()
	if step == nil || step.Type != CWStepProviderMenu {
		t.Fatalf("expected provider menu step, got %#v", step)
	}
	found := false
	for _, opt := range step.Options {
		if opt.Value != string(models.ProviderOpenAI) {
			continue
		}
		found = true
		if opt.Display == nil {
			t.Fatal("expected structured display metadata")
		}
		if opt.Display.Primary == "" || opt.Display.Secondary == "" {
			t.Fatalf("display metadata incomplete: %#v", opt.Display)
		}
		if strings.Contains(opt.Display.Secondary, "sk-test-123456") {
			t.Fatalf("display leaked raw key: %#v", opt.Display)
		}
	}
	if !found {
		t.Fatal("openai option not found")
	}
}

func TestConfigWizardAgentModelStep_UsesDeterministicMiniMaxOrder(t *testing.T) {
	cfg := &Config{
		DefaultProvider: models.ProviderMiniMax,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderMiniMax: {APIKey: "k"},
		},
		Agents: map[AgentName]Agent{
			AgentCoder: {Provider: models.ProviderMiniMax, Model: string(models.MiniMaxM27)},
		},
	}
	w := NewConfigWizard(cfg)
	w.selectedAgent = AgentCoder
	w.agentProvChoice = models.ProviderMiniMax

	step := w.agentModelStep()
	if step == nil {
		t.Fatal("agent model step is nil")
	}

	var got []string
	for _, opt := range step.Options {
		got = append(got, opt.Value)
	}
	want := []string{
		string(models.MiniMaxM27),
		string(models.MiniMaxM27HighSpeed),
		string(models.MiniMaxM25),
		string(models.MiniMaxM25HighSpeed),
	}
	if len(got) < len(want) {
		t.Fatalf("options too short: got=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("option %d = %q, want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestConfigWizardAgentModelStep_IncludesConfiguredProviderModels(t *testing.T) {
	cfg := &Config{
		DefaultProvider: models.ProviderOpenAI,
		Providers: map[models.ModelProvider]Provider{
			models.ProviderOpenAI: {
				APIKey: "k",
				Model:  "future-model",
				Models: map[string]ModelConfig{
					"future-model": {Name: "Future Model"},
				},
			},
		},
		Agents: map[AgentName]Agent{
			AgentCoder: {Provider: models.ProviderOpenAI, Model: "future-model"},
		},
	}
	w := NewConfigWizard(cfg)
	w.selectedAgent = AgentCoder
	w.agentProvChoice = models.ProviderOpenAI

	step := w.agentModelStep()
	for _, opt := range step.Options {
		if opt.Value == "future-model" {
			return
		}
	}
	t.Fatalf("configured model not found in options: %+v", step.Options)
}
