package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/openscholar/openscholar/internal/llm/tools"
)

type captureUsageNotifier struct {
	events []tools.FileToolUsageEvent
}

func (n *captureUsageNotifier) OnFileToolUsage(_ string, evt tools.FileToolUsageEvent) {
	n.events = append(n.events, evt)
}

func withUsageNotifier(ctx context.Context, n *captureUsageNotifier) context.Context {
	return context.WithValue(ctx, tools.FileToolUsageNotifierContextKey, n)
}

func TestFileTools_EmitUsageNotifications(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "a.go")
	mdFile := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(goFile, []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("write go file: %v", err)
	}
	if err := os.WriteFile(mdFile, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write md file: %v", err)
	}

	tests := []struct {
		name     string
		session  string
		tool     tools.BaseTool
		input    map[string]any
		wantTool string
	}{
		{
			name:     "view",
			session:  "sess-view-notify",
			tool:     tools.NewViewTool(autoApprovePerms("sess-view-notify")),
			input:    map[string]any{"file_path": goFile},
			wantTool: "View",
		},
		{
			name:     "edit",
			session:  "sess-edit-notify",
			tool:     tools.NewEditTool(autoApprovePerms("sess-edit-notify")),
			input:    map[string]any{"file_path": mdFile, "old_string": "hello", "new_string": "world"},
			wantTool: "Edit",
		},
		{
			name:     "glob",
			session:  "sess-glob-notify",
			tool:     tools.NewGlobTool(autoApprovePerms("sess-glob-notify")),
			input:    map[string]any{"pattern": "*.go", "path": dir},
			wantTool: "Glob",
		},
		{
			name:     "grep",
			session:  "sess-grep-notify",
			tool:     tools.NewGrepTool(autoApprovePerms("sess-grep-notify")),
			input:    map[string]any{"pattern": "package", "path": dir},
			wantTool: "Grep",
		},
		{
			name:     "write",
			session:  "sess-write-notify",
			tool:     tools.NewWriteTool(autoApprovePerms("sess-write-notify")),
			input:    map[string]any{"file_path": filepath.Join(dir, "new.md"), "content": "hello"},
			wantTool: "Write",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &captureUsageNotifier{}
			ctx := withUsageNotifier(toolCtx(tt.session), notifier)
			if tt.wantTool == "Edit" {
				viewTool := tools.NewViewTool(autoApprovePerms(tt.session))
				vb, _ := json.Marshal(map[string]any{"file_path": mdFile})
				_, _ = viewTool.Run(ctx, tools.ToolCall{ID: "tc-view-seed", Name: "View", Input: string(vb)})
			}
			b, _ := json.Marshal(tt.input)
			resp, err := tt.tool.Run(ctx, tools.ToolCall{ID: "tc-notify", Name: tt.wantTool, Input: string(b)})
			if err != nil {
				t.Fatalf("run tool: %v", err)
			}
			if resp.IsError {
				t.Fatalf("tool returned error response: %s", resp.Content)
			}
			if len(notifier.events) == 0 {
				t.Fatalf("expected usage notification for %s", tt.wantTool)
			}
			found := false
			for _, evt := range notifier.events {
				if evt.ToolName == tt.wantTool {
					found = true
					if len(evt.Paths) == 0 {
						t.Fatalf("expected non-empty paths in notification")
					}
					break
				}
			}
			if !found {
				t.Fatalf("tool name mismatch: wanted %s, events=%v", tt.wantTool, notifier.events)
			}
		})
	}
}
