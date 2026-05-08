package tui

import (
	"context"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/app"
	"github.com/Nahasma/openscholar-public/internal/llm/tools"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

type fakeResumeService struct {
	list      []session.Session
	listCalls int
}

func (f *fakeResumeService) Resolve(context.Context, session.ResumeRequest) (session.ResumeResolveResult, error) {
	return session.ResumeResolveResult{}, nil
}

func (f *fakeResumeService) List(context.Context, string, int) ([]session.Session, error) {
	f.listCalls++
	return f.list, nil
}

func (f *fakeResumeService) Load(context.Context, string) (session.ResumeLoadResult, error) {
	return session.ResumeLoadResult{}, nil
}

func (f *fakeResumeService) NormalizeMessagesForResume(msgs []message.Message) []message.Message {
	return msgs
}

func (f *fakeResumeService) Search(context.Context, string, string, int) ([]session.Session, error) {
	return nil, nil
}

func TestLoadLastSessionCmdNoSelectorOpensPicker(t *testing.T) {
	resume := &fakeResumeService{list: []session.Session{{ID: "sess-1", Title: "one"}}}
	m := newTestModel()
	m.app = &app.App{Resume: resume}
	m.resumeSession = true

	msg := m.loadLastSessionCmd()()
	list, ok := msg.(sessionListMsg)
	if !ok {
		t.Fatalf("expected sessionListMsg, got %T", msg)
	}
	if resume.listCalls != 1 {
		t.Fatalf("resume list calls = %d, want 1", resume.listCalls)
	}
	if len(list) != 1 || list[0].ID != "sess-1" {
		t.Fatalf("unexpected picker list: %+v", list)
	}
}

func TestApplyPlanApprovalUIStatePersistsRestoredMode(t *testing.T) {
	_, q := testutil.SetupTestDB(t)
	sessions := session.NewService(q)
	perms := permission.NewPermissionService()
	ctx := context.Background()

	sess, err := sessions.Create(ctx, "plan session")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	perms.TransitionSessionMode(sess.ID, permission.ModeAuto)
	perms.TransitionSessionMode(sess.ID, permission.ModePlan)
	sess.Mode = "plan"
	if _, err := sessions.Save(ctx, sess); err != nil {
		t.Fatalf("save plan mode: %v", err)
	}

	m := newTestModel()
	m.ctx = ctx
	m.app = &app.App{Sessions: sessions, Permissions: perms}
	m.sessionID = sess.ID
	m.status.mode = "plan"

	m.applyPlanApprovalUIState(sess.ID, tools.PlanApprovalResponse{
		Approved:   true,
		TargetMode: permission.ModeRestore,
	})

	if m.status.mode != "auto" {
		t.Fatalf("status mode = %q, want auto", m.status.mode)
	}
	got, err := sessions.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.Mode != "auto" {
		t.Fatalf("persisted mode = %q, want auto", got.Mode)
	}
}
