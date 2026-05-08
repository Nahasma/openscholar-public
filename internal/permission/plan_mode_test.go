package permission

import (
	"context"
	"testing"
)

type stubPlanResolver struct {
	allowedSession string
	allowedPath    string
}

func (s stubPlanResolver) IsPlanFile(_ context.Context, sessionID string, path string) bool {
	return sessionID == s.allowedSession && path == s.allowedPath
}

func TestPlanModeWriteEditOnlyAllowedForCurrentPlanFile(t *testing.T) {
	svc := NewPermissionService()
	svc.SetPlanFileResolver(stubPlanResolver{allowedSession: "sess-plan", allowedPath: "/workspace/plan.md"})

	blocked := svc.RequestDecision(CreatePermissionRequest{
		SessionID: "sess-plan",
		ToolName:  "Write",
		Action:    "write",
		Path:      "/workspace/other.md",
		Mode:      ModePlan,
	})
	if blocked.Allowed {
		t.Fatalf("expected non-plan write path to be denied")
	}

	allowed := svc.RequestDecision(CreatePermissionRequest{
		SessionID: "sess-plan",
		ToolName:  "Edit",
		Action:    "edit",
		Path:      "/workspace/plan.md",
		Mode:      ModePlan,
	})
	if !allowed.Allowed {
		t.Fatalf("expected exact current plan file edit to be allowed")
	}
}

func TestPlanModeBlocksSideEffectActionsBeforeAutoApprove(t *testing.T) {
	svc := NewPermissionService()
	svc.AutoApproveSession("sess-plan-auto")

	res := svc.RequestDecision(CreatePermissionRequest{
		SessionID: "sess-plan-auto",
		ToolName:  "ImageGen",
		Action:    "export",
		Mode:      ModePlan,
	})
	if res.Allowed {
		t.Fatalf("expected side-effect action to be denied in plan mode")
	}
	if res.Source != SourceMode {
		t.Fatalf("expected SourceMode denial, got %q", res.Source)
	}
}

func TestEnterExitAndTransitionPlanModeState(t *testing.T) {
	svc := NewPermissionService()

	svc.TransitionSessionMode("sess", ModeResearch)
	if got := svc.SessionMode("sess"); got != ModeResearch {
		t.Fatalf("expected research mode, got %q", got)
	}

	svc.EnterPlanMode("sess")
	state := svc.SessionModeState("sess")
	if state.Mode != ModePlan || state.PrePlanMode != ModeResearch {
		t.Fatalf("unexpected state after enter: %+v", state)
	}

	svc.ExitPlanMode("sess", ModeDefault)
	state = svc.SessionModeState("sess")
	if state.Mode != ModeDefault || state.PrePlanMode != ModeDefault {
		t.Fatalf("unexpected state after exit: %+v", state)
	}

	svc.TransitionSessionMode("sess", ModeAuto)
	if got := svc.SessionMode("sess"); got != ModeAuto {
		t.Fatalf("expected auto mode, got %q", got)
	}

	svc.EnterPlanMode("sess")
	svc.ExitPlanMode("sess", ModeRestore)
	if got := svc.SessionMode("sess"); got != ModeAuto {
		t.Fatalf("expected restore to return to pre-plan auto mode, got %q", got)
	}
}
