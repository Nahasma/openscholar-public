package tools

// research_message.go — "ResearchMessage" tool
// Handles inter-agent messaging: send_finding, send_request, broadcast, read_messages.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nahasma/openscholar-public/internal/research"
)

type researchMessageTool struct {
	ctrl ResearchController
}

type researchMessageParams struct {
	Action   string `json:"action"`
	Sender   string `json:"sender,omitempty"`
	Receiver string `json:"receiver,omitempty"`
	Content  string `json:"content,omitempty"`
}

// NewResearchMessageTool creates the ResearchMessage tool for inter-agent messaging.
func NewResearchMessageTool(ctrl ResearchController) BaseTool {
	return &researchMessageTool{ctrl: ctrl}
}

func (t *researchMessageTool) Info() ToolInfo {
	return ToolInfo{
		Name: "ResearchMessage",
		Description: "Inter-agent messaging within a research pipeline. " +
			"Use 'send_finding' to share a discovery with another agent. " +
			"Use 'send_request' to request help or information. " +
			"Use 'broadcast' to send a message to all agents. " +
			"Use 'read_messages' to retrieve incoming messages for an agent.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"send_finding", "send_request", "broadcast", "read_messages"},
					"description": "Messaging action to perform.",
				},
				"sender": map[string]any{
					"type":        "string",
					"description": "Sending agent name (required for send_finding, send_request, broadcast).",
				},
				"receiver": map[string]any{
					"type":        "string",
					"description": "Receiving agent name, or 'all' for broadcast. Required for read_messages.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Message content (required for send_finding, send_request, broadcast).",
				},
			},
			"required": []string{"action"},
		},
		Required: []string{"action"},
	}
}

func (t *researchMessageTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	if !IsResearchMode(ctx) {
		return NewTextErrorResponse("ResearchMessage is only available in research mode."), nil
	}

	var params researchMessageParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	sessionID := ResearchSessionID(ctx)
	if sessionID == "" {
		return NewTextErrorResponse("No session context available."), nil
	}

	pipeline, err := t.ctrl.GetBySession(sessionID)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("No research pipeline found for this session: %v", err)), nil
	}

	switch params.Action {
	case "send_finding":
		return t.handleSendMessage(pipeline, params.Sender, params.Receiver, "finding", params.Content)
	case "send_request":
		return t.handleSendMessage(pipeline, params.Sender, params.Receiver, "request", params.Content)
	case "broadcast":
		return t.handleSendMessage(pipeline, params.Sender, "all", "broadcast", params.Content)
	case "read_messages":
		return t.handleReadMessages(pipeline, params.Receiver)
	default:
		return NewTextErrorResponse(fmt.Sprintf("Unknown action: %s. Valid actions: send_finding, send_request, broadcast, read_messages.", params.Action)), nil
	}
}

func (t *researchMessageTool) handoffManager(pipeline ResearchPipelineView) *research.HandoffManager {
	return research.NewHandoffManager(pipeline.WorkDir)
}

func (t *researchMessageTool) handleSendMessage(pipeline ResearchPipelineView, sender, receiver, msgType, content string) (ToolResponse, error) {
	if sender == "" {
		return NewTextErrorResponse("'sender' is required for message actions."), nil
	}
	if content == "" {
		return NewTextErrorResponse("'content' is required for message actions."), nil
	}
	if receiver == "" {
		receiver = "all"
	}
	hm := t.handoffManager(pipeline)
	if err := hm.SendMessage(sender, receiver, msgType, content); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to send message: %v", err)), nil
	}
	return NewTextResponse(fmt.Sprintf("Message (%s) sent from %s to %s.", msgType, sender, receiver)), nil
}

func (t *researchMessageTool) handleReadMessages(pipeline ResearchPipelineView, receiver string) (ToolResponse, error) {
	if receiver == "" {
		return NewTextErrorResponse("'receiver' is required for read_messages (use 'all' to read broadcasts)."), nil
	}
	hm := t.handoffManager(pipeline)
	msgs, err := hm.ReadMessages(receiver)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to read messages: %v", err)), nil
	}
	if len(msgs) == 0 {
		return NewTextResponse(fmt.Sprintf("No messages for %s.", receiver)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Messages for %s\n\n", receiver)
	for _, msg := range msgs {
		fmt.Fprintf(&sb, "**[%s] %s → %s**: %s\n\n", msg.MsgType, msg.Sender, msg.Receiver, msg.Content)
	}
	return NewTextResponse(sb.String()), nil
}
