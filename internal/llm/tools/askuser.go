package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/openscholar/openscholar/internal/pubsub"
)

// ClarificationEvent is published when the AskUser tool is called.
// The TUI subscribes to these events and renders a ClarificationDialog.
type ClarificationEvent struct {
	ID            string   `json:"id"`
	Question      string   `json:"question"`
	Options       []string `json:"options"`
	AllowFreeform bool     `json:"allow_freeform"`
	Context       string   `json:"context"`
	ResponseCh    chan ClarificationResponse `json:"-"` // not serialized, carried with the event
}

// ClarificationResponse is sent back from the TUI after the user makes a choice.
type ClarificationResponse struct {
	SelectedIndex int    `json:"selected_index"` // -1 for freeform input
	SelectedText  string `json:"selected_text"`
	Skipped       bool   `json:"skipped"` // true if user pressed Esc
}

type askuserTool struct {
	broker *pubsub.Broker[ClarificationEvent]
}

// NewAskUserTool creates the AskUser tool.
func NewAskUserTool(broker *pubsub.Broker[ClarificationEvent]) BaseTool {
	return &askuserTool{broker: broker}
}

func (t *askuserTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "AskUser",
		Description: "Ask the user a clarification question with predefined options. Use when the user's instruction is ambiguous and could be interpreted in multiple ways.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "The clarification question to ask the user",
				},
				"options": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "A JSON array of 2-5 option strings, e.g. [\"Option A\", \"Option B\"]. Must be a string array, NOT an object.",
					"minItems":    2,
					"maxItems":    5,
				},
				"allow_freeform": map[string]any{
					"type":        "boolean",
					"description": "Whether the user can type a custom answer instead of selecting an option",
					"default":     true,
				},
				"context": map[string]any{
					"type":        "string",
					"description": "Brief explanation of why clarification is needed",
				},
			},
			"required": []string{"question", "options"},
		},
		Required: []string{"question", "options"},
	}
}

func (t *askuserTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params struct {
		Question      string   `json:"question"`
		Options       []string `json:"options"`
		AllowFreeform *bool    `json:"allow_freeform"`
		Context       string   `json:"context"`
	}
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return ToolResponse{Content: fmt.Sprintf("Invalid input: %v. Note: 'options' must be a JSON array of strings, e.g. [\"Option A\", \"Option B\"], not an object.", err), IsError: true}, nil
	}

	if len(params.Options) < 2 || len(params.Options) > 5 {
		return ToolResponse{Content: "Options must contain 2-5 items", IsError: true}, nil
	}

	allowFreeform := true
	if params.AllowFreeform != nil {
		allowFreeform = *params.AllowFreeform
	}

	responseCh := make(chan ClarificationResponse, 1)
	event := ClarificationEvent{
		ID:            uuid.New().String(),
		Question:      params.Question,
		Options:       params.Options,
		AllowFreeform: allowFreeform,
		Context:       params.Context,
		ResponseCh:    responseCh,
	}

	// Publish event for TUI to display
	t.broker.Publish(pubsub.CreatedEvent, event)

	// Block until user responds or context is cancelled
	select {
	case resp := <-responseCh:
		if resp.Skipped {
			return ToolResponse{Content: "User skipped clarification. Proceed with your best judgment based on the context."}, nil
		}
		result, _ := json.Marshal(map[string]any{
			"selected_index": resp.SelectedIndex,
			"selected_text":  resp.SelectedText,
		})
		return ToolResponse{Content: string(result)}, nil
	case <-ctx.Done():
		return ToolResponse{}, ctx.Err()
	}
}
