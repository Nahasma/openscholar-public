package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openscholar/openscholar/internal/llm/tools"
)

func TestEditTool_ApplyDiff(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "test.go")
	os.WriteFile(fpath, []byte("func main() {\n\tfmt.Println(\"hello\")\n}\n"), 0644)

	perms := autoApprovePerms("sess-edit")
	tool := tools.NewEditTool(perms)
	ctx := toolCtx("sess-edit")
	view := tools.NewViewTool(perms)
	vin, _ := json.Marshal(map[string]any{"file_path": fpath})
	_, _ = view.Run(ctx, tools.ToolCall{ID: "tc-v", Name: "View", Input: string(vin)})

	input, _ := json.Marshal(map[string]any{
		"file_path":  fpath,
		"old_string": "hello",
		"new_string": "world",
	})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-e1", Name: "Edit", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)

	content, _ := os.ReadFile(fpath)
	assert.Contains(t, string(content), "world")
	assert.NotContains(t, string(content), "hello")
}

func TestEditTool_FileNotFound(t *testing.T) {
	perms := autoApprovePerms("sess-edit2")
	tool := tools.NewEditTool(perms)
	ctx := toolCtx("sess-edit2")

	input, _ := json.Marshal(map[string]any{
		"file_path":  "/nonexistent/file.go",
		"old_string": "foo",
		"new_string": "bar",
	})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-e2", Name: "Edit", Input: string(input)})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
}

func TestEditTool_OldStringNotFound(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "test2.go")
	os.WriteFile(fpath, []byte("some content here"), 0644)

	perms := autoApprovePerms("sess-edit3")
	tool := tools.NewEditTool(perms)
	ctx := toolCtx("sess-edit3")
	view := tools.NewViewTool(perms)
	vin, _ := json.Marshal(map[string]any{"file_path": fpath})
	_, _ = view.Run(ctx, tools.ToolCall{ID: "tc-v", Name: "View", Input: string(vin)})

	input, _ := json.Marshal(map[string]any{
		"file_path":  fpath,
		"old_string": "nonexistent text",
		"new_string": "replacement",
	})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-e3", Name: "Edit", Input: string(input)})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
}

func TestEditTool_ReplaceAll(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "multi.txt")
	os.WriteFile(fpath, []byte("foo bar foo baz foo"), 0644)

	perms := autoApprovePerms("sess-edit4")
	tool := tools.NewEditTool(perms)
	ctx := toolCtx("sess-edit4")
	view := tools.NewViewTool(perms)
	vin, _ := json.Marshal(map[string]any{"file_path": fpath})
	_, _ = view.Run(ctx, tools.ToolCall{ID: "tc-v", Name: "View", Input: string(vin)})

	input, _ := json.Marshal(map[string]any{
		"file_path":   fpath,
		"old_string":  "foo",
		"new_string":  "qux",
		"replace_all": true,
	})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-e4", Name: "Edit", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)

	content, _ := os.ReadFile(fpath)
	assert.Equal(t, "qux bar qux baz qux", string(content))
}
