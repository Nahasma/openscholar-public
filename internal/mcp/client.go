package mcp

import (
	"context"
	"fmt"
	"log"

	mcpClient "github.com/mark3labs/mcp-go/client"
	mcpTypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/openscholar/openscholar/internal/config"
)

// Client manages connections to external MCP servers.
type Client struct {
	servers map[string]*serverConn
}

type serverConn struct {
	config config.MCPServerConfig
	client *mcpClient.Client
	tools  []mcpTypes.Tool
}

// New creates a new MCP client manager.
func New() *Client {
	return &Client{servers: make(map[string]*serverConn)}
}

// Connect establishes connection to an MCP server.
func (c *Client) Connect(ctx context.Context, cfg config.MCPServerConfig) error {
	if cfg.Command != "" {
		return c.connectStdio(ctx, cfg)
	}
	return fmt.Errorf("MCP server %s: must specify command (stdio mode)", cfg.Name)
}

func (c *Client) connectStdio(ctx context.Context, cfg config.MCPServerConfig) error {
	cli, err := mcpClient.NewStdioMCPClient(cfg.Command, cfg.Env, cfg.Args...)
	if err != nil {
		return fmt.Errorf("failed to start MCP server %s: %w", cfg.Name, err)
	}

	// Initialize
	initResult, err := cli.Initialize(ctx, mcpTypes.InitializeRequest{})
	if err != nil {
		cli.Close()
		return fmt.Errorf("MCP server %s initialize failed: %w", cfg.Name, err)
	}

	// Discover tools
	toolsResult, err := cli.ListTools(ctx, mcpTypes.ListToolsRequest{})
	if err != nil {
		cli.Close()
		return fmt.Errorf("MCP server %s list tools failed: %w", cfg.Name, err)
	}

	c.servers[cfg.Name] = &serverConn{
		config: cfg,
		client: cli,
		tools:  toolsResult.Tools,
	}

	log.Printf("MCP: connected to %s (%s) — %d tools",
		cfg.Name, initResult.ServerInfo.Name, len(toolsResult.Tools))

	return nil
}

// ConnectAll connects to all configured MCP servers.
func (c *Client) ConnectAll(ctx context.Context, configs []config.MCPServerConfig) {
	for _, cfg := range configs {
		if err := c.Connect(ctx, cfg); err != nil {
			log.Printf("MCP: failed to connect to %s: %v", cfg.Name, err)
		}
	}
}

// ListTools returns all tools from all connected servers.
func (c *Client) ListTools() map[string][]mcpTypes.Tool {
	result := make(map[string][]mcpTypes.Tool)
	for name, conn := range c.servers {
		result[name] = conn.tools
	}
	return result
}

// CallTool invokes a tool on the appropriate MCP server.
func (c *Client) CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (*mcpTypes.CallToolResult, error) {
	conn, ok := c.servers[serverName]
	if !ok {
		return nil, fmt.Errorf("MCP server %s not connected", serverName)
	}

	return conn.client.CallTool(ctx, mcpTypes.CallToolRequest{
		Params: mcpTypes.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	})
}

// ServerNames returns the names of all connected servers.
func (c *Client) ServerNames() []string {
	names := make([]string, 0, len(c.servers))
	for name := range c.servers {
		names = append(names, name)
	}
	return names
}

// Close disconnects all servers.
func (c *Client) Close() {
	for _, conn := range c.servers {
		if conn.client != nil {
			conn.client.Close()
		}
	}
}
