package builtin

import (
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/command"
)

type mcpCmd struct{}

func (c *mcpCmd) Name() string        { return "mcp" }
func (c *mcpCmd) Description() string { return "List connected MCP servers and their tools" }

func (c *mcpCmd) Execute(ctx command.Context) command.Result {
	if ctx.App.MCPClient == nil {
		return command.Result{Output: "MCP: no client initialized. Configure mcp_servers in .openscholar/config.json"}
	}

	allTools := ctx.App.MCPClient.ListTools()
	if len(allTools) == 0 {
		return command.Result{Output: "MCP: no servers connected. Configure mcp_servers in .openscholar/config.json"}
	}

	var sb strings.Builder
	sb.WriteString("## Connected MCP Servers\n\n")
	for serverName, tools := range allTools {
		sb.WriteString(fmt.Sprintf("### %s (%d tools)\n", serverName, len(tools)))
		for _, t := range tools {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", t.Name, t.Description))
		}
		sb.WriteString("\n")
	}

	return command.Result{Output: sb.String()}
}
