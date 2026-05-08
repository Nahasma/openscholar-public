package tools

import (
	"context"
	"strings"
	"testing"
)

// mockTool is a minimal BaseTool for testing.
type mockTool struct {
	name string
	desc string
}

func (m *mockTool) Info() ToolInfo {
	return ToolInfo{Name: m.name, Description: m.desc}
}

func (m *mockTool) Run(_ context.Context, _ ToolCall) (ToolResponse, error) {
	return NewTextResponse("mock result"), nil
}

func TestToolSearchSelectFastPath(t *testing.T) {
	kbQuery := &mockTool{name: "KBQuery", desc: "Query knowledge base"}
	kbTree := &mockTool{name: "KBTree", desc: "Show KB tree"}
	unrelated := &mockTool{name: "ImageGen", desc: "Generate image"}

	reg := NewDeferredRegistry(nil, []BaseTool{kbQuery, kbTree, unrelated})

	ts := &toolSearchTool{registry: reg}
	call := ToolCall{
		ID:    "test-1",
		Name:  "ToolSearch",
		Input: `{"query": "select:KBQuery,KBTree"}`,
	}

	resp, err := ts.Run(WithLocalKBIntent(context.Background(), true), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError {
		t.Fatalf("unexpected error response: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "KBQuery") {
		t.Errorf("response should mention KBQuery, got: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "KBTree") {
		t.Errorf("response should mention KBTree, got: %s", resp.Content)
	}
	if strings.Contains(resp.Content, "ImageGen") {
		t.Errorf("response should not mention ImageGen (not selected), got: %s", resp.Content)
	}

	// Verify tools are now active.
	active := reg.ActiveTools()
	activeNames := make(map[string]bool)
	for _, tool := range active {
		activeNames[tool.Info().Name] = true
	}
	if !activeNames["KBQuery"] {
		t.Error("KBQuery should be active after select:")
	}
	if !activeNames["KBTree"] {
		t.Error("KBTree should be active after select:")
	}
	if activeNames["ImageGen"] {
		t.Error("ImageGen should NOT be active (was not selected)")
	}
}

func TestToolSearchSelectFastPath_RequiresLocalKBIntentForKBTools(t *testing.T) {
	reg := NewDeferredRegistry(nil, []BaseTool{
		&mockTool{name: "KBQuery", desc: "Query knowledge base"},
	})
	ts := &toolSearchTool{registry: reg}
	call := ToolCall{ID: "test-kb-select", Name: "ToolSearch", Input: `{"query":"select:KBQuery"}`}

	resp, err := ts.Run(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resp.Content, "No tools found") {
		t.Fatalf("expected no-tools due to missing local intent, got: %s", resp.Content)
	}

	resp, err = ts.Run(WithLocalKBIntent(context.Background(), true), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resp.Content, "KBQuery") {
		t.Fatalf("expected KBQuery activation with local intent, got: %s", resp.Content)
	}
}

func TestToolSearchKeywordQuery_CanSatisfyLocalKBIntent(t *testing.T) {
	reg := NewDeferredRegistry(nil, []BaseTool{
		&mockTool{name: "KBList", desc: "List knowledge base papers"},
	})
	ts := &toolSearchTool{registry: reg}
	call := ToolCall{ID: "test-kb-query", Name: "ToolSearch", Input: `{"query":"list my papers from local kb"}`}
	resp, err := ts.Run(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resp.Content, "KBList") {
		t.Fatalf("expected KBList activation from query-intent, got: %s", resp.Content)
	}
}

func TestToolSearchNoMatch_AvailableListFiltersIntentGatedTools(t *testing.T) {
	reg := NewDeferredRegistry(nil, []BaseTool{
		&mockTool{name: "KBSearch", desc: "Search local kb"},
		&mockTool{name: "ScholarSearch", desc: "Search papers online"},
	})
	ts := &toolSearchTool{registry: reg}
	call := ToolCall{ID: "test-no-match", Name: "ToolSearch", Input: `{"query":"diagram renderer"}`}
	resp, err := ts.Run(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(resp.Content, "KBSearch") {
		t.Fatalf("available list should hide KBSearch without local intent: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "ScholarSearch") {
		t.Fatalf("available list should include non-gated tools: %s", resp.Content)
	}
}

func TestToolSearchSelectUnknownTool(t *testing.T) {
	reg := NewDeferredRegistry(nil, []BaseTool{
		&mockTool{name: "KBQuery", desc: "Query"},
	})
	ts := &toolSearchTool{registry: reg}
	call := ToolCall{
		ID:    "test-2",
		Name:  "ToolSearch",
		Input: `{"query": "select:NonExistentTool"}`,
	}

	resp, err := ts.Run(WithLocalKBIntent(context.Background(), true), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError {
		t.Fatalf("select: unknown tool should return informational message, not error: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "No tools found") {
		t.Errorf("expected 'No tools found' message, got: %s", resp.Content)
	}
}

func TestToolSearchSelectWhitespaceHandling(t *testing.T) {
	tool1 := &mockTool{name: "KBAdd", desc: "Add to KB"}
	tool2 := &mockTool{name: "KBList", desc: "List KB"}
	reg := NewDeferredRegistry(nil, []BaseTool{tool1, tool2})
	ts := &toolSearchTool{registry: reg}

	// Names with spaces around commas.
	call := ToolCall{
		ID:    "test-3",
		Name:  "ToolSearch",
		Input: `{"query": "select:KBAdd , KBList"}`,
	}

	resp, err := ts.Run(WithLocalKBIntent(context.Background(), true), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError {
		t.Fatalf("unexpected error response: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "KBAdd") {
		t.Errorf("expected KBAdd in response, got: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "KBList") {
		t.Errorf("expected KBList in response, got: %s", resp.Content)
	}
}
