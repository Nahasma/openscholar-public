package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/skillbank"
)

func TestInvokeSkillTool_InlineRequiresModelInvocable(t *testing.T) {
	conn := setupSkillQueryTestDB(t)
	userDir := t.TempDir()
	writeSkillFileForQueryTest(t, filepath.Join(userDir, "workflow", "invoke_target.md"), `---
name: "invoke target"
description: "inline invoke target"
category: "workflow"
exposure: implicit
---
Use $ARGUMENTS carefully.
`)
	writeSkillFileForQueryTest(t, filepath.Join(userDir, "workflow", "explicit_only.md"), `---
name: "explicit only"
description: "blocked invoke target"
category: "workflow"
exposure: explicit
---
Do not load by model.
`)

	svc := skillbank.NewService(conn, userDir, nil)
	tool := NewInvokeSkillTool(svc, nil, nil, nil, nil, nil)

	resp, err := tool.Run(context.Background(), ToolCall{
		Name:  "InvokeSkill",
		Input: mustJSON(t, map[string]any{"id": "workflow/invoke_target", "args": "the evidence"}),
	})
	if err != nil {
		t.Fatalf("invoke run error: %v", err)
	}
	if resp.IsError {
		t.Fatalf("unexpected invoke error: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "Use the evidence carefully.") {
		t.Fatalf("inline invoke should expand arguments, got:\n%s", resp.Content)
	}

	blocked, err := tool.Run(context.Background(), ToolCall{
		Name:  "InvokeSkill",
		Input: mustJSON(t, map[string]any{"id": "workflow/explicit_only"}),
	})
	if err != nil {
		t.Fatalf("blocked invoke run error: %v", err)
	}
	if !blocked.IsError || !strings.Contains(blocked.Content, "not model-invocable") {
		t.Fatalf("explicit-only skill should be blocked, got:\n%s", blocked.Content)
	}
}
