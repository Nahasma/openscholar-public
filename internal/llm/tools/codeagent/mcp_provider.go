package codeagent

import (
	"context"
	"fmt"
	"strings"
)

// MCPCaller abstracts MCP client operations to avoid import cycles.
// Implemented by mcp.Client in the wiring layer (app.go).
type MCPCaller interface {
	ServerNames() []string
	CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (MCPResult, error)
}

// MCPResult abstracts the MCP tool call result.
type MCPResult struct {
	Content []MCPContent
	IsError bool
}

// MCPContent represents a single content block from MCP response.
type MCPContent struct {
	Text string
}

// mcpProvider implements CodeAgentProvider via MCP protocol.
type mcpProvider struct {
	name       string    // provider name ("claude", "gemini", etc.)
	serverName string    // MCP server name as configured
	caller     MCPCaller // MCP client abstraction
}

func (p *mcpProvider) Name() string { return p.name }

func (p *mcpProvider) Available() bool {
	for _, s := range p.caller.ServerNames() {
		if s == p.serverName {
			return true
		}
	}
	return false
}

func (p *mcpProvider) Execute(ctx context.Context, req CodeRequest) (*CodeResponse, error) {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	args := map[string]any{
		"prompt": req.Prompt,
	}
	if req.WorkDir != "" {
		args["workdir"] = req.WorkDir
	}
	if req.MaxTurns > 0 {
		args["max_turns"] = req.MaxTurns
	}
	if req.SessionID != "" {
		args["session_id"] = req.SessionID
	}

	// Try known tool names until one succeeds
	toolNames := []string{"code", "run_task", "execute", "prompt"}
	var lastErr error
	for _, toolName := range toolNames {
		result, err := p.caller.CallTool(ctx, p.serverName, toolName, args)
		if err != nil {
			lastErr = err
			continue
		}

		var outputParts []string
		for _, c := range result.Content {
			if c.Text != "" {
				outputParts = append(outputParts, c.Text)
			}
		}

		output := strings.Join(outputParts, "\n")
		if output == "" {
			output = fmt.Sprintf("MCP tool %s completed (no text output)", toolName)
		}

		return &CodeResponse{
			Result:  output,
			Success: !result.IsError,
		}, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("MCP %s: all tool calls failed, last error: %w", p.serverName, lastErr)
	}
	return nil, fmt.Errorf("MCP %s: no suitable tool found", p.serverName)
}

// mcpServerMapping maps common MCP server names to CodeAgent provider names.
var mcpServerMapping = map[string]string{
	"claude-code": "claude",
	"claude":      "claude",
	"gemini":      "gemini",
	"gemini-cli":  "gemini",
	"codex":       "codex",
	"qwen-code":   "qwen-code",
	"qwen":        "qwen-code",
	"trae":        "trae",
	"trae-cli":    "trae",
}

// mapServerToProvider maps an MCP server name to a CodeAgent provider name.
func mapServerToProvider(serverName string) string {
	if mapped, ok := mcpServerMapping[serverName]; ok {
		return mapped
	}
	for prefix, provider := range mcpServerMapping {
		if strings.HasPrefix(serverName, prefix) {
			return provider
		}
	}
	return serverName
}
