package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/openscholar/openscholar/internal/llm/tools/codeagent"
)

// mockCodeProvider implements codeagent.CodeAgentProvider for testing.
type mockCodeProvider struct {
	name      string
	available bool
	response  *codeagent.CodeResponse
	err       error
}

func (m *mockCodeProvider) Name() string      { return m.name }
func (m *mockCodeProvider) Available() bool    { return m.available }
func (m *mockCodeProvider) Execute(ctx context.Context, req codeagent.CodeRequest) (*codeagent.CodeResponse, error) {
	return m.response, m.err
}

func newTestRegistry(providers ...codeagent.CodeAgentProvider) *codeagent.Registry {
	r := codeagent.NewEmptyRegistry()
	for _, p := range providers {
		r.Register(p)
	}
	return r
}

func TestCodeAgentToolInfo(t *testing.T) {
	reg := newTestRegistry(&mockCodeProvider{name: "test-cli", available: true})
	tool := NewCodeAgentTool(reg)

	info := tool.Info()
	if info.Name != "CodeAgent" {
		t.Errorf("expected name CodeAgent, got %s", info.Name)
	}
	if len(info.Required) != 1 || info.Required[0] != "prompt" {
		t.Errorf("expected required=[prompt], got %v", info.Required)
	}
}

func TestCodeAgentToolAvailable(t *testing.T) {
	// With available provider — use "claude" which is in the priority order
	reg := newTestRegistry(&mockCodeProvider{name: "claude", available: true})
	tool := NewCodeAgentTool(reg)
	checker := tool.(AvailabilityChecker)
	avail, _ := checker.Available()
	if !avail {
		t.Error("expected Available()=true when provider is installed")
	}

	// With no available provider
	reg2 := newTestRegistry(&mockCodeProvider{name: "claude", available: false})
	tool2 := NewCodeAgentTool(reg2)
	checker2 := tool2.(AvailabilityChecker)
	avail2, reason := checker2.Available()
	if avail2 {
		t.Error("expected Available()=false when no provider installed")
	}
	if reason == "" {
		t.Error("expected non-empty reason when unavailable")
	}
}

func TestCodeAgentToolRunMissingPrompt(t *testing.T) {
	reg := newTestRegistry(&mockCodeProvider{name: "test-cli", available: true})
	tool := NewCodeAgentTool(reg)

	input, _ := json.Marshal(map[string]string{})
	resp, err := tool.Run(context.Background(), ToolCall{
		ID:    "test-1",
		Name:  "CodeAgent",
		Input: string(input),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsError {
		t.Error("expected error response for missing prompt")
	}
}

func TestCodeAgentToolRunSuccess(t *testing.T) {
	provider := &mockCodeProvider{
		name:      "test-cli",
		available: true,
		response: &codeagent.CodeResponse{
			Result:    "code generated successfully",
			SessionID: "sess-123",
			Success:   true,
		},
	}
	reg := newTestRegistry(provider)
	tool := NewCodeAgentTool(reg)

	input, _ := json.Marshal(map[string]string{
		"prompt":   "write a test script",
		"provider": "test-cli",
	})
	resp, err := tool.Run(context.Background(), ToolCall{
		ID:    "test-2",
		Name:  "CodeAgent",
		Input: string(input),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError {
		t.Errorf("unexpected error response: %s", resp.Content)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response content")
	}
}

func TestCodeAgentToolResearchModeSandbox(t *testing.T) {
	provider := &mockCodeProvider{
		name:      "test-cli",
		available: true,
		response:  &codeagent.CodeResponse{Result: "ok", Success: true},
	}
	reg := newTestRegistry(provider)
	tool := NewCodeAgentTool(reg)

	// Create research mode context with workspace
	ctx := context.WithValue(context.Background(), ResearchModeContextKey, true)
	ctx = context.WithValue(ctx, ResearchWorkDirContextKey, "/tmp/research-workspace")

	// Workdir outside workspace should be rejected
	input, _ := json.Marshal(map[string]any{
		"prompt":  "write code",
		"workdir": "/etc/evil",
	})
	resp, err := tool.Run(ctx, ToolCall{
		ID:    "test-3",
		Name:  "CodeAgent",
		Input: string(input),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsError {
		t.Error("expected error when workdir is outside research workspace")
	}
}
