package codeagent

import (
	"context"
	"testing"
)

// mockProvider is a test double for CodeAgentProvider.
type mockProvider struct {
	name      string
	available bool
	result    *CodeResponse
	err       error
}

func (m *mockProvider) Name() string      { return m.name }
func (m *mockProvider) Available() bool    { return m.available }
func (m *mockProvider) Execute(ctx context.Context, req CodeRequest) (*CodeResponse, error) {
	return m.result, m.err
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	// All 5 built-in providers should be registered
	for _, name := range []string{"claude", "gemini", "codex", "qwen-code", "trae"} {
		if _, ok := r.providers[name]; !ok {
			t.Errorf("expected provider %q to be registered", name)
		}
	}
}

func TestRegistryGetSpecific(t *testing.T) {
	r := &Registry{
		providers: make(map[string]CodeAgentProvider),
		configs:   make(map[string]ProviderConfig),
	}
	r.Register(&mockProvider{name: "test-agent", available: true})

	p, err := r.Get("test-agent")
	if err != nil {
		t.Fatalf("Get(test-agent) returned error: %v", err)
	}
	if p.Name() != "test-agent" {
		t.Errorf("expected name test-agent, got %s", p.Name())
	}
}

func TestRegistryGetUnknown(t *testing.T) {
	r := &Registry{
		providers: make(map[string]CodeAgentProvider),
		configs:   make(map[string]ProviderConfig),
	}

	_, err := r.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRegistryGetUnavailable(t *testing.T) {
	r := &Registry{
		providers: make(map[string]CodeAgentProvider),
		configs:   make(map[string]ProviderConfig),
	}
	r.Register(&mockProvider{name: "offline", available: false})

	_, err := r.Get("offline")
	if err == nil {
		t.Fatal("expected error for unavailable provider")
	}
}

func TestRegistryGetPreferred(t *testing.T) {
	r := &Registry{
		providers: make(map[string]CodeAgentProvider),
		configs:   make(map[string]ProviderConfig),
	}
	r.Register(&mockProvider{name: "a", available: true})
	r.Register(&mockProvider{name: "b", available: true})
	r.SetPreferred("b")

	p, err := r.Get("")
	if err != nil {
		t.Fatalf("Get('') returned error: %v", err)
	}
	if p.Name() != "b" {
		t.Errorf("expected preferred provider b, got %s", p.Name())
	}
}

func TestRegistryGetFallback(t *testing.T) {
	r := &Registry{
		providers: make(map[string]CodeAgentProvider),
		configs:   make(map[string]ProviderConfig),
	}
	// Register only one of the priority-order providers as available
	r.Register(&mockProvider{name: "codex", available: true})

	p, err := r.Get("")
	if err != nil {
		t.Fatalf("Get('') returned error: %v", err)
	}
	if p.Name() != "codex" {
		t.Errorf("expected fallback to codex, got %s", p.Name())
	}
}

func TestRegistryGetNoneAvailable(t *testing.T) {
	r := &Registry{
		providers: make(map[string]CodeAgentProvider),
		configs:   make(map[string]ProviderConfig),
	}

	_, err := r.Get("")
	if err == nil {
		t.Fatal("expected error when no providers available")
	}
}

func TestRegistryAvailable(t *testing.T) {
	r := &Registry{
		providers: make(map[string]CodeAgentProvider),
		configs:   make(map[string]ProviderConfig),
	}
	r.Register(&mockProvider{name: "claude", available: true})
	r.Register(&mockProvider{name: "gemini", available: false})
	r.Register(&mockProvider{name: "codex", available: true})

	avail := r.Available()
	if len(avail) != 2 {
		t.Fatalf("expected 2 available, got %d: %v", len(avail), avail)
	}
	// Should be in priority order
	if avail[0] != "claude" || avail[1] != "codex" {
		t.Errorf("expected [claude codex], got %v", avail)
	}
}

func TestRegistryProviderConfig(t *testing.T) {
	r := &Registry{
		providers: make(map[string]CodeAgentProvider),
		configs:   make(map[string]ProviderConfig),
	}
	r.SetProviderConfig("claude", ProviderConfig{AllowedTools: "Read,Write", MaxTurns: 15})

	cfg, ok := r.GetConfig("claude")
	if !ok {
		t.Fatal("expected config for claude")
	}
	if cfg.MaxTurns != 15 {
		t.Errorf("expected MaxTurns=15, got %d", cfg.MaxTurns)
	}

	_, ok = r.GetConfig("nonexistent")
	if ok {
		t.Error("expected no config for nonexistent provider")
	}
}
