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

func TestGrepTool_FindPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("func main() {\n\tfmt.Println(\"hello\")\n}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("func helper() {\n\treturn nil\n}\n"), 0644)

	perms := autoApprovePerms("sess-grep")
	tool := tools.NewGrepTool(perms)
	ctx := toolCtx("sess-grep")

	input, _ := json.Marshal(map[string]any{"pattern": "func.*main", "path": dir})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-gr1", Name: "Grep", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, resp.Content, "func main()")
}

func TestGrepTool_WithInclude(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("TODO: fix this"), 0644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("TODO: update docs"), 0644)

	perms := autoApprovePerms("sess-grep2")
	tool := tools.NewGrepTool(perms)
	ctx := toolCtx("sess-grep2")

	input, _ := json.Marshal(map[string]any{"pattern": "TODO", "path": dir, "include": "*.go"})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-gr2", Name: "Grep", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, resp.Content, "code.go")
	assert.NotContains(t, resp.Content, "readme.md")
}

func TestGrepTool_NoMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("nothing special"), 0644)

	perms := autoApprovePerms("sess-grep3")
	tool := tools.NewGrepTool(perms)
	ctx := toolCtx("sess-grep3")

	input, _ := json.Marshal(map[string]any{"pattern": "nonexistent_pattern_xyz", "path": dir})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-gr3", Name: "Grep", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
}
