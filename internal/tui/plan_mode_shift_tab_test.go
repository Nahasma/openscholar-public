package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/app"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/testutil"
)

func TestTransitionSessionMode_PlanToResearchRequiresApproval(t *testing.T) {
	m := newTestModel()
	m.status.mode = "plan"

	m.transitionSessionMode(permission.ModeResearch)

	if m.status.mode != "plan" {
		t.Fatalf("status mode = %q, want plan", m.status.mode)
	}
	if m.status.notice.Text != "Plan mode requires ExitPlanMode approval to leave" {
		t.Fatalf("notice = %q", m.status.notice.Text)
	}
}

func TestShiftTabCarousel_ExitsPlanToResearchAndClearsPrePlanMode(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessions := session.NewService(q)
	perms := permission.NewPermissionService()
	ctx := context.Background()

	sess, err := sessions.Create(ctx, "shift-tab mode cycle")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	perms.TransitionSessionMode(sess.ID, permission.ModeAuto)
	perms.TransitionSessionMode(sess.ID, permission.ModePlan)
	sess.Mode = "plan"
	if _, err := sessions.Save(ctx, sess); err != nil {
		t.Fatalf("save session: %v", err)
	}

	m := newTestModel()
	m.ctx = ctx
	m.app = &app.App{
		Sessions:    sessions,
		Permissions: perms,
	}
	m.sessionID = sess.ID
	m.status.mode = "plan"

	_, _ = m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyShiftTab})

	if m.status.mode != "research" {
		t.Fatalf("status mode = %q, want research", m.status.mode)
	}
	if got := perms.SessionMode(sess.ID); got != permission.ModeResearch {
		t.Fatalf("permission mode = %q, want research", got)
	}
	if state := perms.SessionModeState(sess.ID); state.PrePlanMode != permission.ModeDefault {
		t.Fatalf("prePlanMode = %q, want default/cleared", state.PrePlanMode)
	}
	persisted, err := sessions.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if persisted.Mode != "research" {
		t.Fatalf("persisted mode = %q, want research", persisted.Mode)
	}
	if m.status.notice.Text == "Plan mode requires ExitPlanMode approval to leave" {
		t.Fatalf("unexpected plan-exit warning notice")
	}

	perms.TransitionSessionMode(sess.ID, permission.ModePlan)
	perms.ExitPlanMode(sess.ID, permission.ModeRestore)
	if got := perms.SessionMode(sess.ID); got != permission.ModeResearch {
		t.Fatalf("restore mode = %q, want research", got)
	}
}
