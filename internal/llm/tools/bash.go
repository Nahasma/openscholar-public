package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/permission"
)

type bashTool struct {
	permissions permission.Service
}

type bashParams struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

func NewBashTool(perms permission.Service) BaseTool {
	return &bashTool{permissions: perms}
}

func (t *bashTool) Info() ToolInfo {
	return ToolInfo{
		Name:           "Bash",
		MaxResultBytes: 20 * 1024, // 20 KB
		Description: "Executes a bash command and returns its output. " +
			"IMPORTANT: Avoid using this tool to run cat, head, tail, sed, awk, echo, find, grep, or rg commands. " +
			"Instead use the dedicated tool: View (read files), Edit (modify files), Write (create files), Glob (find files), Grep (search content). " +
			"Reserve Bash for system commands (compilation, git, tests, package management) that require shell execution.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The bash command to execute",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in milliseconds (default 120000)",
				},
			},
			"required": []string{"command"},
		},
		Required: []string{"command"},
	}
}

func (t *bashTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params bashParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	if params.Command == "" {
		return NewTextErrorResponse("command is required"), nil
	}
	if shouldRejectNoopWait(params.Command) {
		resp := NewTextErrorResponse("Rejected no-op wait command. Do not use Bash sleep to wait for external API cooldowns; use available results or retry later.")
		return WithResponseMetadata(resp, map[string]any{
			"error_kind": "noop_wait_rejected",
		}), nil
	}

	// All commands go through the rule-based permission engine.
	// The engine evaluates deny → allow → ask; safe commands are auto-approved,
	// dangerous commands are blocked, and unknown commands trigger TUI dialog.
	if t.permissions != nil {
		sessionID, _ := GetContextValues(ctx)
		allowed := t.permissions.Request(permission.CreatePermissionRequest{
			SessionID:   sessionID,
			ToolName:    "Bash",
			Description: params.Command,
			Action:      "execute",
			Params:      params,
		})
		if !allowed {
			return ToolResponse{}, permission.ErrorPermissionDenied
		}
	}

	timeout := 120 * time.Second
	if params.Timeout > 0 {
		timeout = time.Duration(params.Timeout) * time.Millisecond
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Research mode: Bash is not allowed (not in ResearchAllowedTools).
	// This is defense-in-depth; the agent execution layer also blocks it.
	if IsResearchMode(ctx) {
		return NewTextErrorResponse("Bash is not available in research mode. Use Write/Edit for file operations, or CodeAgent for coding tasks."), nil
	}

	cmd := exec.CommandContext(cmdCtx, "bash", "-c", params.Command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}

	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return NewTextErrorResponse(fmt.Sprintf("Command timed out after %dms\n%s", timeout.Milliseconds(), output)), nil
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return NewTextErrorResponse(fmt.Sprintf("Command canceled\n%s", output)), nil
		}
		return NewTextErrorResponse(fmt.Sprintf("Command failed: %v\n%s", err, output)), nil
	}

	return NewTextResponse(truncateOutput(output, 100000)), nil
}

var noopWaitPattern = regexp.MustCompile(`^\s*sleep\s+([0-9]+(?:\.[0-9]+)?)\s*(?:&&\s*echo\b.*)?\s*$`)

func shouldRejectNoopWait(cmd string) bool {
	m := noopWaitPattern.FindStringSubmatch(strings.TrimSpace(cmd))
	if len(m) != 2 {
		return false
	}
	secs, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return false
	}
	return secs > 5
}
