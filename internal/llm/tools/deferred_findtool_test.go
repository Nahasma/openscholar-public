package tools

import (
	"context"
	"testing"
)

// mockBaseTool is a minimal BaseTool implementation for testing.
type mockBaseTool struct {
	name string
}

func (m *mockBaseTool) Info() ToolInfo {
	return ToolInfo{Name: m.name, Description: "mock " + m.name}
}

func (m *mockBaseTool) Run(_ context.Context, _ ToolCall) (ToolResponse, error) {
	return NewTextResponse("ok"), nil
}

func TestFindTool_DoesNotActivate(t *testing.T) {
	core := []BaseTool{&mockBaseTool{name: "View"}}
	deferred := []BaseTool{
		&mockBaseTool{name: "kb_query"},
		&mockBaseTool{name: "scholar_search"},
	}

	r := NewDeferredRegistry(core, deferred)

	// FindTool should find a deferred tool without activating it
	tool, found := r.FindTool("kb_query")
	if !found {
		t.Fatal("expected to find kb_query")
	}
	if tool.Info().Name != "kb_query" {
		t.Fatalf("expected kb_query, got %s", tool.Info().Name)
	}

	// Verify it was NOT activated
	r.mu.RLock()
	_, isActive := r.active["kb_query"]
	r.mu.RUnlock()
	if isActive {
		t.Fatal("FindTool should not activate deferred tools")
	}

	// ActiveTools should not include kb_query
	for _, at := range r.ActiveTools() {
		if at.Info().Name == "kb_query" {
			t.Fatal("kb_query should not appear in ActiveTools after FindTool")
		}
	}
}

func TestFindTool_CoreToolsAlwaysFound(t *testing.T) {
	core := []BaseTool{&mockBaseTool{name: "View"}, &mockBaseTool{name: "Edit"}}
	r := NewDeferredRegistry(core, nil)

	tool, found := r.FindTool("View")
	if !found || tool.Info().Name != "View" {
		t.Fatal("expected to find core tool View")
	}

	tool, found = r.FindTool("Edit")
	if !found || tool.Info().Name != "Edit" {
		t.Fatal("expected to find core tool Edit")
	}
}

func TestFindTool_NotFound(t *testing.T) {
	r := NewDeferredRegistry(nil, nil)

	_, found := r.FindTool("nonexistent")
	if found {
		t.Fatal("should not find nonexistent tool")
	}
}

func TestActivateTool_Success(t *testing.T) {
	deferred := []BaseTool{&mockBaseTool{name: "kb_query"}}
	r := NewDeferredRegistry(nil, deferred)

	err := r.ActivateTool(WithLocalKBIntent(context.Background(), true), "kb_query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it was activated
	r.mu.RLock()
	_, isActive := r.active["kb_query"]
	r.mu.RUnlock()
	if !isActive {
		t.Fatal("ActivateTool should activate the tool")
	}
}

func TestActivateTool_NotFound(t *testing.T) {
	r := NewDeferredRegistry(nil, nil)

	err := r.ActivateTool(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}

func TestSearch_UsesActivateTool(t *testing.T) {
	deferred := []BaseTool{
		&mockBaseTool{name: "kb_query"},
		&mockBaseTool{name: "scholar_search"},
	}
	r := NewDeferredRegistry(nil, deferred)

	// Search should activate matched tools
	results := r.SearchFiltered(WithLocalKBIntent(context.Background(), true), "kb", 3, nil)
	if len(results) == 0 {
		t.Fatal("expected search results")
	}

	// Verify activation happened through ActivateTool
	r.mu.RLock()
	_, isActive := r.active["kb_query"]
	r.mu.RUnlock()
	if !isActive {
		t.Fatal("Search should activate matched tools via ActivateTool")
	}
}

func TestBeginTurn_ClearsTurnScopedActiveToolsOnly(t *testing.T) {
	deferred := []BaseTool{
		&mockBaseTool{name: "KBQuery"},
		&mockBaseTool{name: "ScholarSearch"},
	}
	r := NewDeferredRegistry(nil, deferred)
	if err := r.ActivateTool(WithLocalKBIntent(context.Background(), true), "KBQuery"); err != nil {
		t.Fatalf("activate KBQuery: %v", err)
	}
	if err := r.ActivateTool(context.Background(), "ScholarSearch"); err != nil {
		t.Fatalf("activate ScholarSearch: %v", err)
	}
	r.BeginTurn()
	active := map[string]bool{}
	for _, tool := range r.ActiveTools() {
		active[tool.Info().Name] = true
	}
	if active["KBQuery"] {
		t.Fatalf("KBQuery should be cleared at new turn")
	}
	if !active["ScholarSearch"] {
		t.Fatalf("ScholarSearch should keep session activation")
	}
}
