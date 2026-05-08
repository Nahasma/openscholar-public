package codeagent

import (
	"context"
	"fmt"
	"testing"
)

// mockMCPCaller implements MCPCaller for testing.
type mockMCPCaller struct {
	servers   []string
	toolName  string // last tool called
	response  MCPResult
	err       error
	callCount int
}

func (m *mockMCPCaller) ServerNames() []string {
	return m.servers
}

func (m *mockMCPCaller) CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (MCPResult, error) {
	m.callCount++
	m.toolName = toolName
	return m.response, m.err
}

func TestMCPProvider_Available(t *testing.T) {
	caller := &mockMCPCaller{servers: []string{"claude-code", "codex"}}

	p := &mcpProvider{name: "claude", serverName: "claude-code", caller: caller}
	if !p.Available() {
		t.Error("expected available when server is connected")
	}

	p2 := &mcpProvider{name: "gemini", serverName: "gemini-mcp", caller: caller}
	if p2.Available() {
		t.Error("expected unavailable when server is not connected")
	}
}

func TestMCPProvider_Execute_Success(t *testing.T) {
	caller := &mockMCPCaller{
		servers: []string{"claude-code"},
		response: MCPResult{
			Content: []MCPContent{{Text: "Code written successfully"}},
			IsError: false,
		},
	}

	p := &mcpProvider{name: "claude", serverName: "claude-code", caller: caller}
	resp, err := p.Execute(context.Background(), CodeRequest{
		Prompt:  "write a hello world",
		WorkDir: "/tmp/test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success")
	}
	if resp.Result != "Code written successfully" {
		t.Errorf("unexpected result: %s", resp.Result)
	}
}

func TestMCPProvider_Execute_Error(t *testing.T) {
	caller := &mockMCPCaller{
		servers: []string{"claude-code"},
		err:     fmt.Errorf("connection lost"),
	}

	p := &mcpProvider{name: "claude", serverName: "claude-code", caller: caller}
	_, err := p.Execute(context.Background(), CodeRequest{Prompt: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMCPProvider_Execute_MCPError(t *testing.T) {
	caller := &mockMCPCaller{
		servers: []string{"claude-code"},
		response: MCPResult{
			Content: []MCPContent{{Text: "tool not found"}},
			IsError: true,
		},
	}

	p := &mcpProvider{name: "claude", serverName: "claude-code", caller: caller}
	resp, err := p.Execute(context.Background(), CodeRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Error("expected failure when MCP returns error")
	}
}

func TestRegisterMCPProviders(t *testing.T) {
	r := NewEmptyRegistry()
	caller := &mockMCPCaller{servers: []string{"claude-code", "codex"}}

	r.RegisterMCPProviders(caller)

	// "claude-code" should map to "claude" provider
	p, err := r.Get("claude")
	if err != nil {
		t.Fatalf("Get(claude) returned error: %v", err)
	}
	if p.Name() != "claude" {
		t.Errorf("expected claude, got %s", p.Name())
	}

	// "codex" maps to "codex"
	p, err = r.Get("codex")
	if err != nil {
		t.Fatalf("Get(codex) returned error: %v", err)
	}
	if p.Name() != "codex" {
		t.Errorf("expected codex, got %s", p.Name())
	}
}

func TestMCPPriority_OverBash(t *testing.T) {
	r := NewEmptyRegistry()

	// Register Bash provider
	r.Register(&mockProvider{name: "claude", available: true, result: &CodeResponse{Result: "bash"}})

	// Register MCP provider (should take priority)
	caller := &mockMCPCaller{
		servers:  []string{"claude-code"},
		response: MCPResult{Content: []MCPContent{{Text: "mcp"}}},
	}
	r.RegisterMCPProviders(caller)

	// Get without name should prefer MCP
	p, err := r.Get("")
	if err != nil {
		t.Fatalf("Get('') returned error: %v", err)
	}

	resp, _ := p.Execute(context.Background(), CodeRequest{Prompt: "test"})
	if resp.Result != "mcp" {
		t.Errorf("expected MCP provider (result='mcp'), got result='%s'", resp.Result)
	}
}

func TestMCPPriority_ExplicitName(t *testing.T) {
	r := NewEmptyRegistry()

	// Both Bash and MCP registered for "claude"
	r.Register(&mockProvider{name: "claude", available: true, result: &CodeResponse{Result: "bash"}})
	caller := &mockMCPCaller{
		servers:  []string{"claude-code"},
		response: MCPResult{Content: []MCPContent{{Text: "mcp"}}},
	}
	r.RegisterMCPProviders(caller)

	// Explicit name should also prefer MCP
	p, err := r.Get("claude")
	if err != nil {
		t.Fatalf("Get(claude) returned error: %v", err)
	}
	resp, _ := p.Execute(context.Background(), CodeRequest{Prompt: "test"})
	if resp.Result != "mcp" {
		t.Errorf("expected MCP priority, got '%s'", resp.Result)
	}
}

func TestMapServerToProvider(t *testing.T) {
	tests := []struct {
		server   string
		expected string
	}{
		{"claude-code", "claude"},
		{"claude", "claude"},
		{"gemini", "gemini"},
		{"gemini-cli", "gemini"},
		{"codex", "codex"},
		{"qwen-code", "qwen-code"},
		{"trae", "trae"},
		{"trae-cli", "trae"},
		{"custom-agent", "custom-agent"}, // no mapping → use as-is
	}
	for _, tc := range tests {
		got := mapServerToProvider(tc.server)
		if got != tc.expected {
			t.Errorf("mapServerToProvider(%s) = %s, want %s", tc.server, got, tc.expected)
		}
	}
}

func TestRegisterMCPProviders_Nil(t *testing.T) {
	r := NewEmptyRegistry()
	r.RegisterMCPProviders(nil) // should not panic
}
