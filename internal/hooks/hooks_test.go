package hooks

import (
	"context"
	"testing"
	"time"
)

func TestToolCondition(t *testing.T) {
	tests := []struct {
		pattern  string
		toolName string
		want     bool
	}{
		{"", "Bash", true},
		{"*", "Bash", true},
		{"Bash", "Bash", true},
		{"bash", "Bash", true}, // case insensitive
		{"Edit", "Bash", false},
	}
	for _, tt := range tests {
		c := ToolCondition{Pattern: tt.pattern}
		got := c.Match(Input{ToolName: tt.toolName})
		if got != tt.want {
			t.Errorf("ToolCondition{%q}.Match(tool=%q) = %v, want %v", tt.pattern, tt.toolName, got, tt.want)
		}
	}
}

func TestPathCondition(t *testing.T) {
	tests := []struct {
		pattern string
		input   map[string]any
		want    bool
	}{
		{"*.go", map[string]any{"file_path": "internal/foo.go"}, true},
		{"*.go", map[string]any{"file_path": "internal/foo.py"}, false},
		{"internal/**", map[string]any{"file_path": "internal/hooks/types.go"}, true},
		{"*", map[string]any{"file_path": "anything"}, true},
		{"**", map[string]any{"file_path": "anything"}, true},
		{"", map[string]any{"file_path": "anything"}, true},
		{"*.go", map[string]any{"count": 42}, false}, // no path keys
		{"*.go", nil, false},
		{"*.go", map[string]any{"path": "foo.go"}, true},             // "path" key
		{"*.go", map[string]any{"prompt": "internal/foo.go"}, false}, // non-path key ignored
	}
	for _, tt := range tests {
		c := PathCondition{Pattern: tt.pattern}
		got := c.Match(Input{ToolInput: tt.input})
		if got != tt.want {
			t.Errorf("PathCondition{%q}.Match(%v) = %v, want %v", tt.pattern, tt.input, got, tt.want)
		}
	}
}

func TestAndCondition(t *testing.T) {
	c := andCondition{
		a: ToolCondition{Pattern: "Bash"},
		b: PathCondition{Pattern: "*.sh"},
	}
	// Both match
	if !c.Match(Input{ToolName: "Bash", ToolInput: map[string]any{"file_path": "run.sh"}}) {
		t.Error("expected match when both conditions are true")
	}
	// Tool matches, path doesn't
	if c.Match(Input{ToolName: "Bash", ToolInput: map[string]any{"file_path": "run.go"}}) {
		t.Error("expected no match when path doesn't match")
	}
}

func TestBuildCondition(t *testing.T) {
	// nil config → alwaysCondition
	c := buildCondition(nil)
	if !c.Match(Input{}) {
		t.Error("nil config should match everything")
	}

	// tool only
	c = buildCondition(&IfConfig{Tool: "Edit"})
	if !c.Match(Input{ToolName: "Edit"}) {
		t.Error("should match Edit")
	}
	if c.Match(Input{ToolName: "Bash"}) {
		t.Error("should not match Bash")
	}

	// tool + path
	c = buildCondition(&IfConfig{Tool: "Edit", Path: "*.go"})
	if !c.Match(Input{ToolName: "Edit", ToolInput: map[string]any{"file_path": "foo.go"}}) {
		t.Error("should match Edit + *.go")
	}
	if c.Match(Input{ToolName: "Edit", ToolInput: map[string]any{"file_path": "foo.py"}}) {
		t.Error("should not match Edit + *.py")
	}
}

func TestMetadataAndArtifactConditions(t *testing.T) {
	c := buildCondition(&IfConfig{
		Tool:         "Scholar*",
		Provider:     "semantic_*",
		Source:       "semantic_scholar",
		ErrorKind:    "rate_*",
		ProgressKind: "none",
		GoalType:     "academic_download",
	})
	if !c.Match(Input{
		ToolName:     "ScholarSearch",
		Provider:     "semantic_scholar",
		Source:       "semantic_scholar",
		ErrorKind:    "rate_limited",
		ProgressKind: "none",
		GoalType:     "academic_download",
	}) {
		t.Fatal("expected metadata condition to match")
	}
	if c.Match(Input{ToolName: "ScholarSearch", Provider: "openalex", Source: "openalex", ErrorKind: "rate_limited"}) {
		t.Fatal("expected provider/source mismatch")
	}

	artifact := buildCondition(&IfConfig{Artifact: "**/*.pdf"})
	if !artifact.Match(Input{ArtifactPaths: []string{".openscholar/papers/a.pdf"}}) {
		t.Fatal("expected artifact path to match")
	}
	if artifact.Match(Input{ArtifactPaths: []string{"notes.txt"}}) {
		t.Fatal("expected non-PDF artifact to miss")
	}
}

func TestValidEvent(t *testing.T) {
	if !ValidEvent(ProviderCooldown) {
		t.Fatal("expected ProviderCooldown to be valid")
	}
	if ValidEvent(Event("provider-cooldown")) {
		t.Fatal("expected typo event to be invalid")
	}
}

func TestRunnerOnceHook(t *testing.T) {
	runner := newHookRunner([]HookConfig{
		{Event: PreToolUse, Command: "exit 1", Once: true},
	})

	if err := runner.runBlocking(context.Background(), PreToolUse, Input{Timestamp: time.Now()}); err == nil {
		t.Fatal("first once hook execution should run and fail")
	}
	if err := runner.runBlocking(context.Background(), PreToolUse, Input{Timestamp: time.Now()}); err != nil {
		t.Fatalf("second once hook execution should be skipped, got %v", err)
	}
}

func TestHookDepthSkip(t *testing.T) {
	ctx := context.Background()

	// depth 0: tool events should not be skipped
	if skip(ctx, PreToolUse) {
		t.Error("depth 0 should not skip PreToolUse")
	}

	// depth 1: tool events should be skipped
	ctx1 := withDepth(ctx) // depth = 1
	if !skip(ctx1, PreToolUse) {
		t.Error("depth 1 should skip PreToolUse")
	}
	if !skip(ctx1, PostToolUse) {
		t.Error("depth 1 should skip PostToolUse")
	}

	// depth 1: non-tool events should NOT be skipped
	if skip(ctx1, UserPromptSubmit) {
		t.Error("depth 1 should not skip UserPromptSubmit")
	}
	if skip(ctx1, SessionStart) {
		t.Error("depth 1 should not skip SessionStart")
	}

	// depth 1: allowlisted events should NOT be skipped
	if skip(ctx1, TaskCreated) {
		t.Error("depth 1 should not skip TaskCreated (allowlisted)")
	}
	if skip(ctx1, TaskCompleted) {
		t.Error("depth 1 should not skip TaskCompleted (allowlisted)")
	}
}

func TestRunnerBlocking(t *testing.T) {
	runner := newHookRunner([]HookConfig{
		{Event: PreToolUse, Command: "echo hello", Timeout: 2},
		{Event: PostToolUse, Command: "echo world", Async: true}, // should be skipped by runBlocking
	})

	ctx := context.Background()
	err := runner.runBlocking(ctx, PreToolUse, Input{Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("runBlocking failed: %v", err)
	}
}

func TestRunnerBlockingError(t *testing.T) {
	runner := newHookRunner([]HookConfig{
		{Event: PreToolUse, Command: "exit 1", Timeout: 2},
	})

	ctx := context.Background()
	err := runner.runBlocking(ctx, PreToolUse, Input{Timestamp: time.Now()})
	if err == nil {
		t.Fatal("expected error from exit 1")
	}
}

func TestRunnerConditionFilters(t *testing.T) {
	runner := newHookRunner([]HookConfig{
		{Event: PreToolUse, Command: "exit 1", If: &IfConfig{Tool: "Edit"}},
	})

	// Should NOT fail because tool doesn't match
	ctx := context.Background()
	err := runner.runBlocking(ctx, PreToolUse, Input{ToolName: "Bash", Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("should skip non-matching hook: %v", err)
	}

	// Should fail because tool matches
	err = runner.runBlocking(ctx, PreToolUse, Input{ToolName: "Edit", Timestamp: time.Now()})
	if err == nil {
		t.Fatal("expected error when tool matches")
	}
}

func TestParseConfigNoHooks(t *testing.T) {
	data := []byte(`{"debug": true}`)
	svc, err := parseConfig(data, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return a no-op service that doesn't error
	if err := svc.Run(context.Background(), PreToolUse, Input{}); err != nil {
		t.Fatalf("no-op service should not error: %v", err)
	}
}

func TestParseConfigWithHooks(t *testing.T) {
	data := []byte(`{
		"hooks": [
			{"event": "pre_tool_use", "command": "echo ok", "timeout": 2},
			{"event": "", "command": "echo bad"},
			{"event": "post_tool_use", "command": ""},
			{"event": "post-tool-use", "command": "echo typo"}
		]
	}`)
	svc, err := parseConfig(data, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only the first hook should survive validation
	impl := svc.(*serviceImpl)
	impl.mu.RLock()
	count := len(impl.runner.hooks)
	impl.mu.RUnlock()
	if count != 1 {
		t.Fatalf("expected 1 valid hook, got %d", count)
	}
}

func TestNewConstructor(t *testing.T) {
	svc := New([]HookConfig{
		{Event: PreToolUse, Command: "echo test", Timeout: 2},
		{Event: "", Command: "skip me"},   // invalid: no event
		{Event: PostToolUse, Command: ""}, // invalid: no command
		{Event: Event("post-tool-use"), Command: "skip me"},
	})

	impl := svc.(*serviceImpl)
	impl.mu.RLock()
	count := len(impl.runner.hooks)
	impl.mu.RUnlock()
	if count != 1 {
		t.Fatalf("expected 1 valid hook, got %d", count)
	}
}

func TestParseConfigInvalidJSON(t *testing.T) {
	data := []byte(`{invalid json}`)
	_, err := parseConfig(data, "test")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestServiceToolHookExecutesAtTopLevel(t *testing.T) {
	// Verify that a PreToolUse hook fires at depth 0 (top-level call).
	svc := New([]HookConfig{
		{Event: PreToolUse, Command: "echo ok", Timeout: 2},
	})

	ctx := context.Background() // depth 0
	err := svc.RunBlocking(ctx, PreToolUse, Input{ToolName: "Bash", Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("top-level PreToolUse hook should execute: %v", err)
	}
}

func TestServiceToolHookBlockedAtDepth1(t *testing.T) {
	// Verify that a PreToolUse hook is skipped when already inside a hook (depth >= 1).
	svc := New([]HookConfig{
		{Event: PreToolUse, Command: "exit 1", Timeout: 2}, // would fail if executed
	})

	// Simulate depth 1: as if we're already inside a hook execution
	ctx := context.WithValue(context.Background(), hookDepthKey{}, 1)
	err := svc.RunBlocking(ctx, PreToolUse, Input{ToolName: "Bash", Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("hook at depth 1 should be skipped, got error: %v", err)
	}
}

func TestServiceAllowlistedEventPassesAtDepth1(t *testing.T) {
	// TaskCreated/TaskCompleted are allowlisted — should execute even at depth 1.
	svc := New([]HookConfig{
		{Event: TaskCreated, Command: "echo allowed", Timeout: 2},
	})

	ctx := context.WithValue(context.Background(), hookDepthKey{}, 1)
	err := svc.RunBlocking(ctx, TaskCreated, Input{Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("allowlisted event should execute at depth 1: %v", err)
	}
}

func TestPathConditionOnlyChecksPathKeys(t *testing.T) {
	// Ensure non-path fields like "content" are NOT matched
	c := PathCondition{Pattern: "*.go"}

	// Non-path key should not match
	input := Input{ToolInput: map[string]any{"content": "internal/foo.go"}}
	if c.Match(input) {
		t.Error("should not match non-path key 'content'")
	}

	// Path key should match
	input = Input{ToolInput: map[string]any{"file_path": "internal/foo.go"}}
	if !c.Match(input) {
		t.Error("should match path key 'file_path'")
	}
}
