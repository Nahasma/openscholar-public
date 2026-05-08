package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/task"
)

type researchAdvanceCall struct {
	pipelineID     string
	skipCheckpoint bool
}

type mockResearchController struct {
	pipeline  ResearchPipelineView
	phases    []ResearchPhaseView
	advances  []researchAdvanceCall
	modeCalls []string
}

func (m *mockResearchController) GetBySession(sessionID string) (ResearchPipelineView, error) {
	return m.pipeline, nil
}

func (m *mockResearchController) GetPhasesView(pipelineID string) ([]ResearchPhaseView, error) {
	return append([]ResearchPhaseView(nil), m.phases...), nil
}

func (m *mockResearchController) AdvancePipeline(pipelineID string) error {
	m.advances = append(m.advances, researchAdvanceCall{pipelineID: pipelineID})
	return nil
}

func (m *mockResearchController) AdvancePipelineWithOptions(pipelineID string, opts ResearchAdvanceOptions) error {
	m.advances = append(m.advances, researchAdvanceCall{
		pipelineID:     pipelineID,
		skipCheckpoint: opts.SkipCheckpoint,
	})
	return nil
}

func (m *mockResearchController) PausePipeline(pipelineID string) error {
	return nil
}

func (m *mockResearchController) SetPipelineMode(pipelineID, mode string) error {
	m.modeCalls = append(m.modeCalls, mode)
	return nil
}

type fakeSessionService struct {
	*pubsub.Broker[session.Session]
	sessions map[string]session.Session
}

func newFakeSessionService() *fakeSessionService {
	return &fakeSessionService{
		Broker:   pubsub.NewBroker[session.Session](),
		sessions: map[string]session.Session{},
	}
}

func (s *fakeSessionService) Create(ctx context.Context, title string) (session.Session, error) {
	sess := session.Session{ID: "parent", Title: title}
	s.sessions[sess.ID] = sess
	return sess, nil
}

func (s *fakeSessionService) CreateTitleSession(ctx context.Context, parentSessionID string) (session.Session, error) {
	return session.Session{ID: "title-" + parentSessionID, ParentSessionID: parentSessionID, Title: "title"}, nil
}

func (s *fakeSessionService) CreateTaskSession(ctx context.Context, toolCallID, parentSessionID, title string) (session.Session, error) {
	sess := session.Session{ID: toolCallID, ParentSessionID: parentSessionID, Title: title}
	s.sessions[sess.ID] = sess
	return sess, nil
}

func (s *fakeSessionService) Get(ctx context.Context, id string) (session.Session, error) {
	return s.sessions[id], nil
}

func (s *fakeSessionService) List(ctx context.Context) ([]session.Session, error) {
	out := make([]session.Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	return out, nil
}

func (s *fakeSessionService) Save(ctx context.Context, sess session.Session) (session.Session, error) {
	s.sessions[sess.ID] = sess
	return sess, nil
}

func (s *fakeSessionService) Delete(ctx context.Context, id string) error {
	delete(s.sessions, id)
	return nil
}

type fakeMessageService struct {
	*pubsub.Broker[message.Message]
}

func newFakeMessageService() *fakeMessageService {
	return &fakeMessageService{Broker: pubsub.NewBroker[message.Message]()}
}

func (m *fakeMessageService) Create(ctx context.Context, sessionID string, params message.CreateMessageParams) (message.Message, error) {
	return message.Message{}, nil
}

func (m *fakeMessageService) Update(ctx context.Context, msg message.Message) error {
	return nil
}

func (m *fakeMessageService) Get(ctx context.Context, id string) (message.Message, error) {
	return message.Message{}, nil
}

func (m *fakeMessageService) List(ctx context.Context, sessionID string) ([]message.Message, error) {
	return nil, nil
}

func (m *fakeMessageService) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *fakeMessageService) DeleteSessionMessages(ctx context.Context, sessionID string) error {
	return nil
}

func newResearchPipelineToolForTest(
	t *testing.T,
	ctrl *mockResearchController,
	runAgent AgentRunner,
	broker *pubsub.Broker[CheckpointEvent],
	workDir string,
) (*researchPipelineTool, context.Context, *task.Registry) {
	t.Helper()

	sessions := newFakeSessionService()
	messages := newFakeMessageService()
	perms := permission.NewPermissionService()
	parent, _ := sessions.Create(context.Background(), "parent")

	reg := task.NewRegistry()
	SetTaskRuntime(reg, nil)
	t.Cleanup(func() {
		SetTaskRuntime(nil, nil)
	})

	tool := &researchPipelineTool{
		ctrl:             ctrl,
		checkpointBroker: broker,
		permissions:      perms,
		runAgent:         runAgent,
		sessions:         sessions,
		messages:         messages,
	}

	ctx := context.WithValue(context.Background(), SessionIDContextKey, parent.ID)
	ctx = context.WithValue(ctx, ResearchWorkDirContextKey, workDir)
	ctx = context.WithValue(ctx, ResearchRootSessionContextKey, parent.ID)
	return tool, ctx, reg
}

func prepareResearchDeliverable(t *testing.T, workDir, filename string) {
	t.Helper()
	handoffDir := filepath.Join(workDir, ".handoff")
	if err := os.MkdirAll(handoffDir, 0o755); err != nil {
		t.Fatalf("mkdir handoff: %v", err)
	}
	if err := os.WriteFile(filepath.Join(handoffDir, filename), []byte("deliverable"), 0o644); err != nil {
		t.Fatalf("write handoff: %v", err)
	}
}

func TestResearchPipelineAdvance_AutoModeUsesVerifyTaskAndAdvances(t *testing.T) {
	workDir := t.TempDir()
	prepareResearchDeliverable(t, workDir, "01-literature.md")

	ctrl := &mockResearchController{
		pipeline: ResearchPipelineView{ID: "pipe-1", Mode: "auto", WorkDir: workDir, Template: "survey"},
		phases: []ResearchPhaseView{{
			ID:         "phase-1",
			Order:      1,
			Name:       "survey",
			Status:     "running",
			Checkpoint: true,
		}},
	}

	var gotAgent config.AgentName
	tool, ctx, reg := newResearchPipelineToolForTest(t, ctrl, func(ctx context.Context, agentName config.AgentName, sessionID, prompt string, agentTools []BaseTool) (string, error) {
		gotAgent = agentName
		return `{"score":8,"report":"looks good"}`, nil
	}, nil, workDir)

	resp, err := tool.handleAdvance(ctx, ctrl.pipeline, "summary")
	if err != nil {
		t.Fatalf("handleAdvance err: %v", err)
	}
	if resp.IsError {
		t.Fatalf("expected success response, got error: %s", resp.Content)
	}
	if gotAgent != config.AgentVerify {
		t.Fatalf("expected verify agent, got %s", gotAgent)
	}
	if len(ctrl.advances) != 1 || ctrl.advances[0].pipelineID != "pipe-1" || ctrl.advances[0].skipCheckpoint {
		t.Fatalf("unexpected advance calls: %#v", ctrl.advances)
	}
	if len(reg.All()) == 0 {
		t.Fatal("expected verify task to be tracked in task registry")
	}
}

func TestResearchPipelineAdvance_DefaultCheckpointRequiresApproval(t *testing.T) {
	workDir := t.TempDir()
	prepareResearchDeliverable(t, workDir, "01-literature.md")

	ctrl := &mockResearchController{
		pipeline: ResearchPipelineView{ID: "pipe-2", Mode: "default", WorkDir: workDir, Template: "survey"},
		phases: []ResearchPhaseView{{
			ID:         "phase-1",
			Order:      1,
			Name:       "survey",
			Status:     "running",
			Checkpoint: true,
		}},
	}

	broker := pubsub.NewBroker[CheckpointEvent]()
	t.Cleanup(broker.Shutdown)
	tool, ctx, _ := newResearchPipelineToolForTest(t, ctrl, func(ctx context.Context, agentName config.AgentName, sessionID, prompt string, agentTools []BaseTool) (string, error) {
		return `{"score":9,"report":"approved"}`, nil
	}, broker, workDir)

	subCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events := broker.Subscribe(subCtx)

	done := make(chan ToolResponse, 1)
	go func() {
		resp, _ := tool.handleAdvance(ctx, ctrl.pipeline, "summary")
		done <- resp
	}()

	select {
	case evt := <-events:
		evt.Payload.ResponseCh <- CheckpointResponse{Approved: true}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for checkpoint event")
	}

	select {
	case resp := <-done:
		if resp.IsError {
			t.Fatalf("expected success response, got %s", resp.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handleAdvance")
	}

	if len(ctrl.advances) != 1 || !ctrl.advances[0].skipCheckpoint {
		t.Fatalf("expected checkpoint-skipping advance after approval, got %#v", ctrl.advances)
	}
}

func TestResearchPipelineAdvance_StrictRequiresApprovalEvenWithoutCheckpoint(t *testing.T) {
	workDir := t.TempDir()
	prepareResearchDeliverable(t, workDir, "01-literature.md")

	ctrl := &mockResearchController{
		pipeline: ResearchPipelineView{ID: "pipe-3", Mode: "strict", WorkDir: workDir, Template: "survey"},
		phases: []ResearchPhaseView{{
			ID:         "phase-1",
			Order:      1,
			Name:       "survey",
			Status:     "running",
			Checkpoint: false,
		}},
	}

	broker := pubsub.NewBroker[CheckpointEvent]()
	t.Cleanup(broker.Shutdown)
	tool, ctx, _ := newResearchPipelineToolForTest(t, ctrl, func(ctx context.Context, agentName config.AgentName, sessionID, prompt string, agentTools []BaseTool) (string, error) {
		return `{"score":9,"report":"approved"}`, nil
	}, broker, workDir)

	subCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events := broker.Subscribe(subCtx)

	done := make(chan ToolResponse, 1)
	go func() {
		resp, _ := tool.handleAdvance(ctx, ctrl.pipeline, "summary")
		done <- resp
	}()

	select {
	case evt := <-events:
		evt.Payload.ResponseCh <- CheckpointResponse{Approved: true}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for strict-mode checkpoint event")
	}

	select {
	case resp := <-done:
		if resp.IsError {
			t.Fatalf("expected success response, got %s", resp.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handleAdvance")
	}

	if len(ctrl.advances) != 1 || !ctrl.advances[0].skipCheckpoint {
		t.Fatalf("expected strict mode to require approved advance, got %#v", ctrl.advances)
	}
}

func TestBuildARISReviewPrompt_DoesNotLeakSummaryOrHandoffBody(t *testing.T) {
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, ".handoff"), 0o755); err != nil {
		t.Fatal(err)
	}
	secret := "LEADER_SUMMARY_SHOULD_NOT_APPEAR"
	if err := os.WriteFile(filepath.Join(workDir, ".handoff", "01-literature.md"), []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt := buildARISReviewPrompt(workDir, "aris_empirical", &ResearchPhaseView{Order: 1, Name: "文献调研"})
	for _, forbidden := range []string{
		secret,
		"changes since last round",
		"摘要",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("aris review prompt leaked forbidden text %q\nprompt:\n%s", forbidden, prompt)
		}
	}
	if !strings.Contains(prompt, ".handoff/01-literature.md") {
		t.Fatalf("expected file path in prompt, got:\n%s", prompt)
	}
}

func TestResearchPipelineAdvance_ARISReviewUsesPipelineWorkDir(t *testing.T) {
	workDir := t.TempDir()
	for _, rel := range []string{
		"idea-stage/IDEA_REPORT.md",
		"idea-stage/IDEA_CANDIDATES.md",
		".handoff/01-literature.md",
	} {
		path := filepath.Join(workDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("deliverable"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ctrl := &mockResearchController{
		pipeline: ResearchPipelineView{ID: "pipe-aris", Mode: "auto", WorkDir: workDir, Template: "aris_empirical"},
		phases: []ResearchPhaseView{{
			ID:         "phase-1",
			Order:      1,
			Name:       "Idea Discovery",
			Status:     "running",
			Checkpoint: true,
		}},
	}

	var prompt string
	tool, ctx, _ := newResearchPipelineToolForTest(t, ctrl, func(ctx context.Context, agentName config.AgentName, sessionID, gotPrompt string, agentTools []BaseTool) (string, error) {
		prompt = gotPrompt
		return `{"score":8,"report":"ok"}`, nil
	}, nil, "")

	resp, err := tool.handleAdvance(ctx, ctrl.pipeline, "summary should not appear")
	if err != nil {
		t.Fatalf("handleAdvance err: %v", err)
	}
	if resp.IsError {
		t.Fatalf("expected success, got %s", resp.Content)
	}
	if !strings.Contains(prompt, "idea-stage/IDEA_REPORT.md") || !strings.Contains(prompt, ".handoff/01-literature.md") {
		t.Fatalf("expected ARIS review prompt to use pipeline workdir deliverables, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "summary should not appear") {
		t.Fatalf("ARIS review prompt leaked summary:\n%s", prompt)
	}
}

func TestResearchPipelineAdvance_DefaultCheckpointFailsClosedWithoutBroker(t *testing.T) {
	workDir := t.TempDir()
	prepareResearchDeliverable(t, workDir, "01-literature.md")

	ctrl := &mockResearchController{
		pipeline: ResearchPipelineView{ID: "pipe-4", Mode: "default", WorkDir: workDir, Template: "survey"},
		phases: []ResearchPhaseView{{
			ID:         "phase-1",
			Order:      1,
			Name:       "survey",
			Status:     "running",
			Checkpoint: true,
		}},
	}

	tool, ctx, _ := newResearchPipelineToolForTest(t, ctrl, func(ctx context.Context, agentName config.AgentName, sessionID, prompt string, agentTools []BaseTool) (string, error) {
		return `{"score":8,"report":"looks good"}`, nil
	}, nil, workDir)

	resp, err := tool.handleAdvance(ctx, ctrl.pipeline, "summary")
	if err != nil {
		t.Fatalf("handleAdvance err: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected error response without checkpoint broker, got: %s", resp.Content)
	}
	if len(ctrl.advances) != 0 {
		t.Fatalf("expected pipeline not to advance, got %#v", ctrl.advances)
	}
}

func TestResearchPipelineAdvance_InvalidVerifyOutputDoesNotAdvance(t *testing.T) {
	workDir := t.TempDir()
	prepareResearchDeliverable(t, workDir, "01-literature.md")

	ctrl := &mockResearchController{
		pipeline: ResearchPipelineView{ID: "pipe-5", Mode: "auto", WorkDir: workDir, Template: "survey"},
		phases: []ResearchPhaseView{{
			ID:         "phase-1",
			Order:      1,
			Name:       "survey",
			Status:     "running",
			Checkpoint: true,
		}},
	}

	tool, ctx, reg := newResearchPipelineToolForTest(t, ctrl, func(ctx context.Context, agentName config.AgentName, sessionID, prompt string, agentTools []BaseTool) (string, error) {
		return "not-json", nil
	}, nil, workDir)

	resp, err := tool.handleAdvance(ctx, ctrl.pipeline, "summary")
	if err != nil {
		t.Fatalf("handleAdvance err: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected error response for invalid verify output, got: %s", resp.Content)
	}
	if len(ctrl.advances) != 0 {
		t.Fatalf("expected pipeline not to advance, got %#v", ctrl.advances)
	}
	if len(reg.All()) == 0 {
		t.Fatal("expected verify task to be tracked in task registry")
	}
}

func TestRunReviewer_CancellationStopsAndNotifiesVerifyTask(t *testing.T) {
	sessions := newFakeSessionService()
	messages := newFakeMessageService()
	perms := permission.NewPermissionService()
	parent, _ := sessions.Create(context.Background(), "parent")

	reg := task.NewRegistry()
	SetTaskRuntime(reg, nil)
	t.Cleanup(func() {
		SetTaskRuntime(nil, nil)
	})

	started := make(chan struct{})
	tool := &researchPipelineTool{
		permissions: perms,
		runAgent: func(ctx context.Context, agentName config.AgentName, sessionID, prompt string, agentTools []BaseTool) (string, error) {
			close(started)
			<-ctx.Done()
			return "", ctx.Err()
		},
		sessions: sessions,
		messages: messages,
	}

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), SessionIDContextKey, parent.ID))
	done := make(chan error, 1)
	go func() {
		_, _, err := tool.runReviewer(ctx, `{"score":8,"report":"ok"}`)
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("verify task did not start")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runReviewer did not return after cancellation")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		all := reg.All()
		if len(all) == 1 {
			meta := all[0].TaskMeta()
			if meta.Status == task.StatusCanceled && meta.Notified {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	all := reg.All()
	if len(all) != 1 {
		t.Fatalf("expected one verify task, got %d", len(all))
	}
	meta := all[0].TaskMeta()
	t.Fatalf("expected canceled+notified verify task, got status=%s notified=%v", meta.Status, meta.Notified)
}
