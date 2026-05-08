package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nahasma/openscholar-public/internal/llm/tools"
)

func TestWriteTool_CreateFile(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "new_file.txt")

	perms := autoApprovePerms("sess-write")
	tool := tools.NewWriteTool(perms)
	ctx := toolCtx("sess-write")

	input, _ := json.Marshal(map[string]any{
		"file_path": fpath,
		"content":   "Hello, World!\n",
	})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-w1", Name: "Write", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)

	content, err := os.ReadFile(fpath)
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!\n", string(content))
}

func TestWriteTool_CreateNestedDirs(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "a", "b", "c", "deep.txt")

	perms := autoApprovePerms("sess-write2")
	tool := tools.NewWriteTool(perms)
	ctx := toolCtx("sess-write2")

	input, _ := json.Marshal(map[string]any{
		"file_path": fpath,
		"content":   "deep content",
	})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-w2", Name: "Write", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)

	content, _ := os.ReadFile(fpath)
	assert.Equal(t, "deep content", string(content))
}

func TestWriteTool_OverwriteFile(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "overwrite.txt")
	os.WriteFile(fpath, []byte("original"), 0644)

	perms := autoApprovePerms("sess-write3")
	tool := tools.NewWriteTool(perms)
	ctx := toolCtx("sess-write3")
	view := tools.NewViewTool(perms)
	vin, _ := json.Marshal(map[string]any{"file_path": fpath})
	_, _ = view.Run(ctx, tools.ToolCall{ID: "tc-v", Name: "View", Input: string(vin)})

	input, _ := json.Marshal(map[string]any{
		"file_path": fpath,
		"content":   "replaced",
	})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-w3", Name: "Write", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)

	content, _ := os.ReadFile(fpath)
	assert.Equal(t, "replaced", string(content))
}

func TestWriteTool_OverwriteRelativePathAfterView(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "relative.txt")
	os.WriteFile(fpath, []byte("original"), 0644)

	perms := autoApprovePerms("sess-write-relative")
	write := tools.NewWriteTool(perms)
	view := tools.NewViewTool(perms)
	ctx := context.WithValue(toolCtx("sess-write-relative"), tools.WorkspaceDirContextKey, dir)

	vin, _ := json.Marshal(map[string]any{"file_path": "relative.txt"})
	vresp, err := view.Run(ctx, tools.ToolCall{ID: "tc-v-rel", Name: "View", Input: string(vin)})
	require.NoError(t, err)
	require.False(t, vresp.IsError, vresp.Content)

	input, _ := json.Marshal(map[string]any{
		"file_path": "relative.txt",
		"content":   "replaced",
	})
	resp, err := write.Run(ctx, tools.ToolCall{ID: "tc-w-rel", Name: "Write", Input: string(input)})
	require.NoError(t, err)
	require.False(t, resp.IsError, resp.Content)

	content, _ := os.ReadFile(fpath)
	assert.Equal(t, "replaced", string(content))
}
