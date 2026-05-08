package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcpTypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
)

// mcpBridgeTool wraps an MCP tool as a BaseTool for the Agent tool registry.
type mcpBridgeTool struct {
	serverName string
	tool       mcpTypes.Tool
	client     *Client
}

// BridgeTools converts all MCP tools from a server into BaseTool implementations.
func (c *Client) BridgeTools(serverName string) []tools.BaseTool {
	conn, ok := c.servers[serverName]
	if !ok {
		return nil
	}

	bridged := make([]tools.BaseTool, 0, len(conn.tools))
	for _, t := range conn.tools {
		bridged = append(bridged, &mcpBridgeTool{
			serverName: serverName,
			tool:       t,
			client:     c,
		})
	}
	return bridged
}

// BridgeAllTools converts all MCP tools from all connected servers into BaseTools.
func (c *Client) BridgeAllTools() []tools.BaseTool {
	var all []tools.BaseTool
	for name := range c.servers {
		all = append(all, c.BridgeTools(name)...)
	}
	return all
}

func (t *mcpBridgeTool) Info() tools.ToolInfo {
	return tools.ToolInfo{
		Name:        fmt.Sprintf("mcp_%s_%s", t.serverName, t.tool.Name),
		Description: t.tool.Description,
		Parameters:  t.tool.InputSchema.Properties,
	}
}

func (t *mcpBridgeTool) Run(ctx context.Context, call tools.ToolCall) (tools.ToolResponse, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Input), &args); err != nil {
		return tools.NewTextErrorResponse(fmt.Sprintf("Invalid MCP tool args: %v", err)), nil
	}

	result, err := t.client.CallTool(ctx, t.serverName, t.tool.Name, args)
	if err != nil {
		return tools.NewTextErrorResponse(fmt.Sprintf("MCP tool error: %v", err)), nil
	}

	// Convert MCP result to text response
	var output string
	for _, content := range result.Content {
		if textContent, ok := content.(mcpTypes.TextContent); ok {
			output += textContent.Text + "\n"
		}
	}
	if result.IsError {
		return tools.NewTextErrorResponse(output), nil
	}
	return tools.NewTextResponse(output), nil
}
