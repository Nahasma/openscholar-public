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

func TestGlobTool_MatchFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("go"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("go"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("txt"), 0644)

	perms := autoApprovePerms("sess-glob")
	tool := tools.NewGlobTool(perms)
	ctx := toolCtx("sess-glob")

	input, _ := json.Marshal(map[string]any{"pattern": "*.go", "path": dir})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-g1", Name: "Glob", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, resp.Content, "a.go")
	assert.Contains(t, resp.Content, "b.go")
	assert.NotContains(t, resp.Content, "c.txt")
}

func TestGlobTool_RecursiveMatch(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(dir, "root.go"), []byte("go"), 0644)
	os.WriteFile(filepath.Join(subdir, "nested.go"), []byte("go"), 0644)

	perms := autoApprovePerms("sess-glob2")
	tool := tools.NewGlobTool(perms)
	ctx := toolCtx("sess-glob2")

	input, _ := json.Marshal(map[string]any{"pattern": "**/*.go", "path": dir})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-g2", Name: "Glob", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, resp.Content, "nested.go")
}

func TestGlobTool_NoMatch(t *testing.T) {
	dir := t.TempDir()

	perms := autoApprovePerms("sess-glob3")
	tool := tools.NewGlobTool(perms)
	ctx := toolCtx("sess-glob3")

	input, _ := json.Marshal(map[string]any{"pattern": "*.xyz", "path": dir})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-g3", Name: "Glob", Input: string(input)})
	require.NoError(t, err)
	// No match should still return without error
	assert.False(t, resp.IsError)
}

func TestGlobTool_RejectsPatternTraversal(t *testing.T) {
	dir := t.TempDir()

	perms := autoApprovePerms("sess-glob-traversal")
	tool := tools.NewGlobTool(perms)
	ctx := toolCtx("sess-glob-traversal")

	input, _ := json.Marshal(map[string]any{"pattern": "../*.txt", "path": dir})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-g-traversal", Name: "Glob", Input: string(input)})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
}

func TestGlobTool_FiltersSymlinkEscapes(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	perms := autoApprovePerms("sess-glob-symlink")
	tool := tools.NewGlobTool(perms)
	ctx := toolCtx("sess-glob-symlink")

	input, _ := json.Marshal(map[string]any{"pattern": "link/*.txt", "path": workspace})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-g-symlink", Name: "Glob", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.NotContains(t, resp.Content, "secret.txt")
	assert.Equal(t, "No files found", resp.Content)
}
