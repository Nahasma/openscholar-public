package tools

// research_task.go — "ResearchTask" tool
// Handles handoff task management: create_task, claim_task, complete_task, list_tasks.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openscholar/openscholar/internal/research"
)

type researchTaskTool struct {
	ctrl ResearchController
}

type researchTaskParams struct {
	Action      string `json:"action"`
	PhaseID     string `json:"phase_id,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Description string `json:"description,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Result      string `json:"result,omitempty"`
}

// NewResearchTaskTool creates the ResearchTask tool for handoff task management.
func NewResearchTaskTool(ctrl ResearchController) BaseTool {
	return &researchTaskTool{ctrl: ctrl}
}

func (t *researchTaskTool) Info() ToolInfo {
	return ToolInfo{
		Name: "ResearchTask",
		Description: "Manage handoff tasks between Leader and Worker agents in a research pipeline. " +
			"Leader uses 'create_task' to dispatch work; Workers use 'claim_task' to take ownership " +
			"and 'complete_task' to report results. Use 'list_tasks' to view task status.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"create_task", "claim_task", "complete_task", "list_tasks"},
					"description": "Task action to perform.",
				},
				"phase_id": map[string]any{
					"type":        "string",
					"description": "Phase ID for task scoping (used with 'create_task' and 'list_tasks').",
				},
				"task_id": map[string]any{
					"type":        "string",
					"description": "Task ID (required for 'claim_task' and 'complete_task').",
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "Short subject/title for the task (required for 'create_task').",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Detailed description of the task (used with 'create_task').",
				},
				"owner": map[string]any{
					"type":        "string",
					"description": "Agent name claiming the task (required for 'claim_task').",
				},
				"result": map[string]any{
					"type":        "string",
					"description": "Task result summary (used with 'complete_task').",
				},
			},
			"required": []string{"action"},
		},
		Required: []string{"action"},
	}
}

func (t *researchTaskTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	if !IsResearchMode(ctx) {
		return NewTextErrorResponse("ResearchTask is only available in research mode."), nil
	}

	var params researchTaskParams
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
	case "create_task":
		return t.handleCreateTask(pipeline, params.PhaseID, params.Subject, params.Description)
	case "claim_task":
		return t.handleClaimTask(pipeline, params.TaskID, params.Owner)
	case "complete_task":
		return t.handleCompleteTask(pipeline, params.TaskID, params.Result)
	case "list_tasks":
		return t.handleListTasks(pipeline, params.PhaseID)
	default:
		return NewTextErrorResponse(fmt.Sprintf("Unknown action: %s. Valid actions: create_task, claim_task, complete_task, list_tasks.", params.Action)), nil
	}
}

func (t *researchTaskTool) handoffManager(pipeline ResearchPipelineView) *research.HandoffManager {
	return research.NewHandoffManager(pipeline.WorkDir)
}

func (t *researchTaskTool) handleCreateTask(pipeline ResearchPipelineView, phaseID, subject, desc string) (ToolResponse, error) {
	if subject == "" {
		return NewTextErrorResponse("'subject' is required for create_task."), nil
	}
	hm := t.handoffManager(pipeline)
	task, err := hm.CreateTask(phaseID, subject, desc)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to create task: %v", err)), nil
	}
	return NewTextResponse(fmt.Sprintf("Task created: id=%s subject=%q phase=%s status=pending", task.ID, task.Subject, task.PhaseID)), nil
}

func (t *researchTaskTool) handleClaimTask(pipeline ResearchPipelineView, taskID, owner string) (ToolResponse, error) {
	if taskID == "" || owner == "" {
		return NewTextErrorResponse("'task_id' and 'owner' are required for claim_task."), nil
	}
	hm := t.handoffManager(pipeline)
	task, err := hm.ClaimTask(taskID, owner)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to claim task: %v", err)), nil
	}
	return NewTextResponse(fmt.Sprintf("Task %s claimed by %s.", task.ID, task.Owner)), nil
}

func (t *researchTaskTool) handleCompleteTask(pipeline ResearchPipelineView, taskID, result string) (ToolResponse, error) {
	if taskID == "" {
		return NewTextErrorResponse("'task_id' is required for complete_task."), nil
	}
	hm := t.handoffManager(pipeline)
	if err := hm.CompleteTask(taskID, result); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to complete task: %v", err)), nil
	}
	return NewTextResponse(fmt.Sprintf("Task %s marked as completed.", taskID)), nil
}

func (t *researchTaskTool) handleListTasks(pipeline ResearchPipelineView, phaseID string) (ToolResponse, error) {
	hm := t.handoffManager(pipeline)
	tasks, err := hm.LoadTasks()
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to load tasks: %v", err)), nil
	}
	if len(tasks) == 0 {
		return NewTextResponse("No tasks found."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Handoff Tasks\n\n")
	fmt.Fprintf(&sb, "%-8s %-10s %-12s %-15s %s\n", "ID", "Phase", "Status", "Owner", "Subject")
	fmt.Fprintf(&sb, "%s\n", strings.Repeat("-", 70))
	for _, task := range tasks {
		if phaseID != "" && task.PhaseID != phaseID {
			continue
		}
		fmt.Fprintf(&sb, "%-8s %-10s %-12s %-15s %s\n",
			task.ID, task.PhaseID, task.Status, task.Owner, task.Subject)
	}
	return NewTextResponse(sb.String()), nil
}
