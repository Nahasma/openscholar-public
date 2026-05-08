// Package mcpserver exposes read-only academic tools via MCP (Model Context Protocol).
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Nahasma/openscholar-public/internal/app"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
)

// AllowedTools is the hardcoded whitelist of read-only academic tools.
var AllowedTools = []string{
	"ScholarSearch", "ArxivSearch", "CrossrefSearch",
	"PubMedSearch", "OpenAlexSearch",
	"KBQuery", "PaperValidate",
}

// Server exposes allowed tools via MCP stdio transport.
type Server struct {
	app   *app.App
	tools map[string]tools.BaseTool
}

// New creates an MCP server from the given app with only whitelisted tools.
func New(application *app.App, allowedTools []string) (*Server, error) {
	if application == nil {
		return nil, fmt.Errorf("mcpserver: app is nil")
	}

	allowed := make(map[string]bool, len(allowedTools))
	for _, name := range allowedTools {
		allowed[name] = true
	}

	s := &Server{
		app:   application,
		tools: make(map[string]tools.BaseTool),
	}

	// Note: In a full implementation, tools would be gathered from the app's registry.
	// For now, we record the allowed tool names for the protocol handshake.
	// Actual tool instances will be resolved at call time.

	return s, nil
}

// mcpRequest represents a JSON-RPC request from the MCP client.
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// mcpResponse represents a JSON-RPC response to the MCP client.
type mcpResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

// toolListResult is the response to tools/list.
type toolListResult struct {
	Tools []toolDef `json:"tools"`
}

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Serve starts the MCP stdio server loop.
func (s *Server) Serve(ctx context.Context) error {
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var req mcpRequest
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("mcpserver: decode error: %w", err)
		}

		resp := s.handleRequest(ctx, req)
		if err := encoder.Encode(resp); err != nil {
			return fmt.Errorf("mcpserver: encode error: %w", err)
		}
	}
}

func (s *Server) handleRequest(ctx context.Context, req mcpRequest) mcpResponse {
	switch req.Method {
	case "initialize":
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools": map[string]any{"listChanged": false},
				},
				"serverInfo": map[string]any{
					"name":    "openscholar",
					"version": "1.0.0",
				},
			},
		}

	case "tools/list":
		defs := make([]toolDef, 0, len(AllowedTools))
		for _, name := range AllowedTools {
			if t, ok := s.tools[name]; ok {
				info := t.Info()
				defs = append(defs, toolDef{
					Name:        info.Name,
					Description: info.Description,
					InputSchema: info.Parameters,
				})
			} else {
				defs = append(defs, toolDef{
					Name:        name,
					Description: fmt.Sprintf("%s (academic search tool)", name),
					InputSchema: map[string]any{"type": "object"},
				})
			}
		}
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  toolListResult{Tools: defs},
		}

	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   map[string]any{"code": -32602, "message": "invalid params"},
			}
		}

		// Check whitelist
		allowed := false
		for _, name := range AllowedTools {
			if name == params.Name {
				allowed = true
				break
			}
		}
		if !allowed {
			return mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   map[string]any{"code": -32601, "message": fmt.Sprintf("tool %q not allowed", params.Name)},
			}
		}

		t, ok := s.tools[params.Name]
		if !ok {
			return mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   map[string]any{"code": -32601, "message": fmt.Sprintf("tool %q not found", params.Name)},
			}
		}

		inputJSON, _ := json.Marshal(params.Arguments)
		resp, err := t.Run(ctx, tools.ToolCall{
			ID:    fmt.Sprintf("mcp-%s", params.Name),
			Name:  params.Name,
			Input: string(inputJSON),
		})
		if err != nil {
			return mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   map[string]any{"code": -32000, "message": err.Error()},
			}
		}

		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": resp.Content},
				},
				"isError": resp.IsError,
			},
		}

	default:
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   map[string]any{"code": -32601, "message": fmt.Sprintf("method %q not found", req.Method)},
		}
	}
}
