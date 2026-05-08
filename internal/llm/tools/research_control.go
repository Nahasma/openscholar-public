package tools

// research_control.go — compatibility wrapper for the legacy "ResearchControl" tool.
//
// Deprecated: The 12 actions previously handled here have been split into three
// narrower tools:
//   - ResearchPipeline  (advance, status, pause, set_mode)
//   - ResearchTask      (create_task, claim_task, complete_task, list_tasks)
//   - ResearchMessage   (send_finding, send_request, broadcast, read_messages)
//
// This wrapper remains so that existing agent prompts that reference
// "ResearchControl" continue to work without modification.  It will be removed
// once all callers have been updated to use the narrow tools.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
	"github.com/Nahasma/openscholar-public/internal/session"
)

type researchControlTool struct {
	pipelineTool BaseTool
	taskTool     BaseTool
	messageTool  BaseTool
}

// NewResearchControlTool creates the legacy ResearchControl tool backed by the
// three narrow tools.  All four constructor parameters are forwarded to
// NewResearchPipelineTool; the task and message tools only need the controller.
func NewResearchControlTool(
	ctrl ResearchController,
	broker *pubsub.Broker[CheckpointEvent],
	perms permission.Service,
	runAgent AgentRunner,
	sessions session.Service,
	messages message.Service,
) BaseTool {
	return &researchControlTool{
		pipelineTool: NewResearchPipelineTool(ctrl, broker, perms, runAgent, sessions, messages),
		taskTool:     NewResearchTaskTool(ctrl),
		messageTool:  NewResearchMessageTool(ctrl),
	}
}

func (t *researchControlTool) Info() ToolInfo {
	return ToolInfo{
		Name: "ResearchControl",
		Description: "Manage research pipeline phases and inter-agent handoff protocol. " +
			"Phase control: advance, status, pause, set_mode. " +
			"Task management: create_task (Leader creates subtask), claim_task (Worker claims), complete_task (Worker finishes), list_tasks (view tasks). " +
			"Messaging: send_finding (share a finding), send_request (request help), broadcast (broadcast to all agents), read_messages (read incoming messages). " +
			"Deprecated: prefer ResearchPipeline, ResearchTask, ResearchMessage for new code.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string",
					"enum": []string{
						"advance", "status", "pause", "set_mode",
						"create_task", "claim_task", "complete_task", "list_tasks",
						"send_finding", "send_request", "broadcast", "read_messages",
					},
					"description": "Action to perform. Phase: advance/status/pause/set_mode. Tasks: create_task/claim_task/complete_task/list_tasks. Messages: send_finding/send_request/broadcast/read_messages.",
				},
				"summary": map[string]any{
					"type":        "string",
					"description": "Checkpoint summary for the completed phase (used with 'advance')",
				},
				"mode": map[string]any{
					"type":        "string",
					"enum":        []string{"default", "auto", "strict"},
					"description": "Target automation mode (used with 'set_mode')",
				},
				"phase_id": map[string]any{
					"type":        "string",
					"description": "Phase ID for task scoping (used with 'create_task' and 'list_tasks')",
				},
				"task_id": map[string]any{
					"type":        "string",
					"description": "Task ID (used with 'claim_task' and 'complete_task')",
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "Short subject/title for the task (used with 'create_task')",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Detailed description of the task (used with 'create_task')",
				},
				"owner": map[string]any{
					"type":        "string",
					"description": "Agent name claiming the task (used with 'claim_task')",
				},
				"result": map[string]any{
					"type":        "string",
					"description": "Task result summary (used with 'complete_task')",
				},
				"sender": map[string]any{
					"type":        "string",
					"description": "Sender agent name (used with send_finding/send_request/broadcast)",
				},
				"receiver": map[string]any{
					"type":        "string",
					"description": "Receiver agent name, or 'all' for broadcast (used with send_finding/send_request/broadcast/read_messages)",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Message content (used with send_finding/send_request/broadcast)",
				},
			},
			"required": []string{"action"},
		},
		Required: []string{"action"},
	}
}

// Run delegates to the appropriate narrow tool based on the action field.
func (t *researchControlTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	// Peek at the action to decide which narrow tool handles this call.
	var peek struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal([]byte(call.Input), &peek); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
	}

	switch peek.Action {
	case "advance", "status", "pause", "set_mode":
		return t.pipelineTool.Run(ctx, call)
	case "create_task", "claim_task", "complete_task", "list_tasks":
		return t.taskTool.Run(ctx, call)
	case "send_finding", "send_request", "broadcast", "read_messages":
		return t.messageTool.Run(ctx, call)
	default:
		return NewTextErrorResponse(fmt.Sprintf("Unknown action: %s.", peek.Action)), nil
	}
}
