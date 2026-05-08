package tools

import (
	"context"
	"fmt"
)

// CompactTool exposes the agent's context-compaction capability as a tool
// that the LLM can call explicitly (e.g. /compact slash command delegate).
//
// The actual compaction logic lives in the agent package; this tool calls
// back into it via the compactFn callback to avoid a circular import.
type CompactTool struct {
	compactFn func(ctx context.Context, focus string) error
}

// NewCompactTool creates a CompactTool backed by the supplied callback.
// compactFn should summarise the current conversation; focus is an
// optional hint about what to preserve.
func NewCompactTool(compactFn func(ctx context.Context, focus string) error) *CompactTool {
	return &CompactTool{compactFn: compactFn}
}

func (t *CompactTool) Info() ToolInfo {
	return ToolInfo{
		Name: "Compact",
		Description: "Compress the conversation history to free up context window space. " +
			"Call this when the conversation is getting long and you need more space for complex tasks. " +
			"An optional focus hint tells the summarizer what information to preserve.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"focus": map[string]any{
					"type":        "string",
					"description": "Optional hint about what information is most important to preserve in the summary.",
				},
			},
		},
		Required: []string{},
	}
}

func (t *CompactTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	if t.compactFn == nil {
		return NewTextErrorResponse("compact: no compaction function configured"), nil
	}

	// Extract optional focus parameter
	focus := extractJSONStringField(call.Input, "focus")

	if err := t.compactFn(ctx, focus); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("compaction failed: %v", err)), nil
	}

	return NewTextResponse("Conversation history compacted successfully. Continuing with summarised context."), nil
}
