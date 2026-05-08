package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/fileop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/permission"
)

func toolCtx(sessionID string) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, tools.SessionIDContextKey, sessionID)
	ctx = context.WithValue(ctx, tools.MessageIDContextKey, "msg-test")
	ctx = context.WithValue(ctx, tools.ReadStateContextKey, fileop.NewReadState())
	return ctx
}

func autoApprovePerms(sessionID string) permission.Service {
	perms := permission.NewPermissionService()
	perms.AutoApproveSession(sessionID)
	return perms
}

func TestViewTool_ReadFile(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "hello.txt")
	os.WriteFile(fpath, []byte("line1\nline2\nline3\n"), 0644)

	perms := autoApprovePerms("sess-view")
	tool := tools.NewViewTool(perms)
	ctx := toolCtx("sess-view")

	input, _ := json.Marshal(map[string]any{"file_path": fpath})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-v1", Name: "View", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, resp.Content, "line1")
	assert.Contains(t, resp.Content, "line2")
	assert.Contains(t, resp.Content, "line3")
}

func TestViewTool_ReadFile_WithOffset(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "multi.txt")
	content := ""
	for i := 1; i <= 20; i++ {
		content += "line " + string(rune('A'-1+i)) + "\n"
	}
	os.WriteFile(fpath, []byte(content), 0644)

	perms := autoApprovePerms("sess-view2")
	tool := tools.NewViewTool(perms)
	ctx := toolCtx("sess-view2")

	input, _ := json.Marshal(map[string]any{"file_path": fpath, "offset": 5, "limit": 3})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-v2", Name: "View", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
}

func TestViewTool_FileNotFound(t *testing.T) {
	perms := autoApprovePerms("sess-view3")
	tool := tools.NewViewTool(perms)
	ctx := toolCtx("sess-view3")

	input, _ := json.Marshal(map[string]any{"file_path": "/nonexistent/path/file.txt"})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-v3", Name: "View", Input: string(input)})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
}

func TestViewTool_InvalidJSON(t *testing.T) {
	perms := autoApprovePerms("sess-view4")
	tool := tools.NewViewTool(perms)
	ctx := toolCtx("sess-view4")

	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-v4", Name: "View", Input: "not json"})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
}
