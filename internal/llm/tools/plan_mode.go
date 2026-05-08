package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/plan"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/session"
)

// PlanApprovalEvent is emitted by ExitPlanMode for a dedicated plan approval UI.
type PlanApprovalEvent struct {
	ID         string
	SessionID  string
	PlanPath   string
	Plan       string
	PlanBody   string
	ResponseCh chan PlanApprovalResponse
}

// PlanApprovalResponse carries the user's decision on the proposed plan.
type PlanApprovalResponse struct {
	Approved   bool
	Feedback   string
	EditedPlan string
	TargetMode permission.Mode
}

type enterPlanModeTool struct {
	permissions permission.Service
	plans       plan.Service
	sessions    session.Service
}

type exitPlanModeTool struct {
	permissions permission.Service
	plans       plan.Service
	broker      *pubsub.Broker[PlanApprovalEvent]
}

func NewEnterPlanModeTool(perms permission.Service, plans plan.Service, sessions session.Service) BaseTool {
	return &enterPlanModeTool{permissions: perms, plans: plans, sessions: sessions}
}

func NewExitPlanModeTool(perms permission.Service, plans plan.Service, broker *pubsub.Broker[PlanApprovalEvent]) BaseTool {
	return &exitPlanModeTool{permissions: perms, plans: plans, broker: broker}
}

func (t *enterPlanModeTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "EnterPlanMode",
		Description: "Enter plan mode for the current main session. Creates or restores the session plan file and returns the plan-mode workflow instructions.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func (t *enterPlanModeTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	sessionID, _ := GetContextValues(ctx)
	if sessionID == "" {
		return NewTextErrorResponse("EnterPlanMode requires an active session."), nil
	}
	if t.permissions == nil || t.plans == nil {
		return NewTextErrorResponse("Plan mode is not available in this runtime."), nil
	}
	if t.sessions != nil {
		sess, err := t.sessions.Get(ctx, sessionID)
		if err == nil && strings.TrimSpace(sess.ParentSessionID) != "" {
			return NewTextErrorResponse("EnterPlanMode is only available in the main session, not child agent sessions."), nil
		}
	}

	pf, err := t.plans.Ensure(ctx, sessionID)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to create plan file: %v", err)), nil
	}
	t.permissions.EnterPlanMode(sessionID)

	return NewTextResponse(formatPlanModeInstructions(pf.Path, pf.Exists)), nil
}

func (t *exitPlanModeTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "ExitPlanMode",
		Description: "Submit the current session plan file for user approval. This is the only formal way to leave plan mode and begin implementation.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target_mode": map[string]any{
					"type":        "string",
					"enum":        []string{"restore", "default", "auto", "research"},
					"description": "Preferred mode after approval. The user can override this in the approval dialog.",
				},
			},
		},
	}
}

func (t *exitPlanModeTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	sessionID, _ := GetContextValues(ctx)
	if sessionID == "" {
		return NewTextErrorResponse("ExitPlanMode requires an active session."), nil
	}
	if t.permissions == nil || t.plans == nil || t.broker == nil {
		return NewTextErrorResponse("Plan approval is not available in this runtime."), nil
	}
	if t.permissions.SessionMode(sessionID) != permission.ModePlan {
		return NewTextErrorResponse("ExitPlanMode can only be used while the current session is in plan mode."), nil
	}

	var params struct {
		TargetMode string `json:"target_mode"`
	}
	if strings.TrimSpace(call.Input) != "" {
		if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
			return NewTextErrorResponse(fmt.Sprintf("Invalid parameters: %v", err)), nil
		}
	}
	suggestedMode := parseExitTargetMode(params.TargetMode)

	pf, content, err := t.plans.Read(ctx, sessionID)
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("Failed to read plan file: %v", err)), nil
	}
	body := plan.NormalizeBody(content)
	if strings.TrimSpace(body) == "" {
		return NewTextErrorResponse(fmt.Sprintf("The plan file is empty. Write the plan to %s before calling ExitPlanMode.", pf.Path)), nil
	}

	responseCh := make(chan PlanApprovalResponse, 1)
	event := PlanApprovalEvent{
		ID:         uuid.New().String(),
		SessionID:  sessionID,
		PlanPath:   pf.Path,
		Plan:       plan.WrapProposed(body),
		PlanBody:   plan.DisplayBody(body),
		ResponseCh: responseCh,
	}
	t.broker.Publish(pubsub.CreatedEvent, event)

	select {
	case resp := <-responseCh:
		if !resp.Approved {
			if strings.TrimSpace(resp.Feedback) == "" {
				return NewTextResponse("Plan rejected. Stay in plan mode, revise the plan file, then call ExitPlanMode again."), nil
			}
			return NewTextResponse("Plan rejected with feedback:\n\n" + resp.Feedback + "\n\nStay in plan mode, revise the plan file, then call ExitPlanMode again."), nil
		}

		approvedBody := body
		shouldWriteApprovedBody := content != body
		if strings.TrimSpace(resp.EditedPlan) != "" {
			approvedBody = plan.NormalizeBody(resp.EditedPlan)
			shouldWriteApprovedBody = true
		}
		if shouldWriteApprovedBody {
			if _, err := t.plans.Write(ctx, sessionID, approvedBody); err != nil {
				return NewTextErrorResponse(fmt.Sprintf("Failed to save edited plan: %v", err)), nil
			}
		}
		targetMode := resp.TargetMode
		if targetMode == "" {
			targetMode = suggestedMode
		}
		t.permissions.ExitPlanMode(sessionID, targetMode)

		return NewTextResponse(fmt.Sprintf("Plan approved. Exit plan mode and implement the approved plan.\n\nApproved plan file: %s\n\n%s", pf.Path, plan.WrapApproved(approvedBody))), nil
	case <-ctx.Done():
		return ToolResponse{}, ctx.Err()
	}
}

func parseExitTargetMode(raw string) permission.Mode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "restore":
		return permission.ModeRestore
	case "auto":
		return permission.ModeAuto
	case "research":
		return permission.ModeResearch
	default:
		return permission.ModeDefault
	}
}

func formatPlanModeInstructions(path string, exists bool) string {
	state := "does not exist yet"
	if exists {
		state = "already exists"
	}
	return fmt.Sprintf(`Plan mode is active.

Plan file: %s (%s)

Workflow:
1. Explore only with read-only tools.
2. Keep the plan updated in the plan file. Write/Edit may only target that exact file.
3. Use AskUser only for clarification, not for plan approval.
4. Keep the plan file as Markdown body only (no protocol wrapper tags).
5. When the plan is ready, call ExitPlanMode. It will submit a canonical <proposed_plan>...</proposed_plan> for approval. Do not ask for approval in ordinary text.`, path, state)
}
