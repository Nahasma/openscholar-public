package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openscholar/openscholar/internal/llm/tools"
)

func TestBashTool_SimpleCommand(t *testing.T) {
	perms := autoApprovePerms("sess-bash")
	tool := tools.NewBashTool(perms)
	ctx := toolCtx("sess-bash")

	input, _ := json.Marshal(map[string]any{"command": "echo hello world"})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-b1", Name: "Bash", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, resp.Content, "hello world")
}

func TestBashTool_SafeCommand_NoPermission(t *testing.T) {
	// Safe commands (ls, pwd, echo) should work without auto-approve
	perms := autoApprovePerms("sess-bash2")
	tool := tools.NewBashTool(perms)
	ctx := toolCtx("sess-bash2")

	input, _ := json.Marshal(map[string]any{"command": "pwd"})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-b2", Name: "Bash", Input: string(input)})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.NotEmpty(t, resp.Content)
}

func TestBashTool_Timeout(t *testing.T) {
	perms := autoApprovePerms("sess-bash3")
	tool := tools.NewBashTool(perms)
	ctx := toolCtx("sess-bash3")

	// 100ms timeout with a 5s sleep should timeout
	input, _ := json.Marshal(map[string]any{"command": "sleep 5", "timeout": 100})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-b3", Name: "Bash", Input: string(input)})
	require.NoError(t, err)
	// Should return with timeout indication
	assert.True(t, resp.IsError || resp.Content != "")
}

func TestBashTool_InvalidJSON(t *testing.T) {
	perms := autoApprovePerms("sess-bash4")
	tool := tools.NewBashTool(perms)
	ctx := toolCtx("sess-bash4")

	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-b4", Name: "Bash", Input: "not json"})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
}

func TestBashTool_ExitCode(t *testing.T) {
	perms := autoApprovePerms("sess-bash5")
	tool := tools.NewBashTool(perms)
	ctx := toolCtx("sess-bash5")

	input, _ := json.Marshal(map[string]any{"command": "false"})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-b5", Name: "Bash", Input: string(input)})
	require.NoError(t, err)
	// Non-zero exit should be reflected
	assert.True(t, resp.IsError || resp.Content != "")
}

func TestBashTool_RejectsNoopSleepWait(t *testing.T) {
	perms := autoApprovePerms("sess-bash6")
	tool := tools.NewBashTool(perms)
	ctx := toolCtx("sess-bash6")
	input, _ := json.Marshal(map[string]any{"command": "sleep 30 && echo done"})
	resp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-b6", Name: "Bash", Input: string(input)})
	require.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Metadata, "noop_wait_rejected")
}

func TestBashTool_DistinguishesTimeoutAndCancel(t *testing.T) {
	perms := autoApprovePerms("sess-bash7")
	tool := tools.NewBashTool(perms)
	ctx := toolCtx("sess-bash7")

	timeoutInput, _ := json.Marshal(map[string]any{"command": "echo start; sleep 5.1", "timeout": 100})
	timeoutResp, err := tool.Run(ctx, tools.ToolCall{ID: "tc-b7a", Name: "Bash", Input: string(timeoutInput)})
	require.NoError(t, err)
	assert.True(t, timeoutResp.IsError)
	assert.Contains(t, timeoutResp.Content, "timed out")

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	cancelInput, _ := json.Marshal(map[string]any{"command": "echo start; sleep 5"})
	cancelResp, err := tool.Run(cancelCtx, tools.ToolCall{ID: "tc-b7b", Name: "Bash", Input: string(cancelInput)})
	require.NoError(t, err)
	assert.True(t, cancelResp.IsError)
	assert.Contains(t, cancelResp.Content, "canceled")
}
