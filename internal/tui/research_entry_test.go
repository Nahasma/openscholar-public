package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/app"
	"github.com/Nahasma/openscholar-public/internal/command"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
	"github.com/Nahasma/openscholar-public/internal/research"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

func newResearchEntryModel(t *testing.T) *Model {
	t.Helper()
	conn, q := testutil.SetupTestDB(t)
	broker := pubsub.NewBroker[research.ResearchEvent]()
	t.Cleanup(broker.Shutdown)

	m := newTestModel()
	m.ctx = context.Background()
	m.app = &app.App{
		Sessions:       session.NewService(q),
		Messages:       message.NewService(q),
		Permissions:    permission.NewPermissionService(),
		ResearchEngine: research.NewEngine(research.NewStore(conn), broker),
		ResearchEvents: broker,
	}
	return m
}

type staticCommand struct {
	name   string
	result command.Result
}

func (c staticCommand) Name() string        { return c.name }
func (c staticCommand) Description() string { return c.name }
func (c staticCommand) Execute(ctx command.Context) command.Result {
	return c.result
}

func TestConfirmTemplateSelectionCreatesSessionWhenMissing(t *testing.T) {
	m := newResearchEntryModel(t)
	m.dlg.template.options = []string{"survey"}
	m.dlg.template.idx = 0
	m.dlg.template.folderName = t.TempDir()
	m.dlg.template.pendingText = "runtime topic"

	model, _ := m.confirmTemplateSelection()
	got, ok := model.(*Model)
	if !ok {
		t.Fatalf("expected *Model, got %T", model)
	}
	if got.sessionID == "" {
		t.Fatal("expected confirmTemplateSelection to create a session")
	}
	if got.status.researchPipelineID == "" {
		t.Fatal("expected confirmTemplateSelection to set researchPipelineID")
	}
	pipeline, err := got.app.ResearchEngine.GetBySession(got.sessionID)
	if err != nil {
		t.Fatalf("expected pipeline for created session: %v", err)
	}
	if pipeline.SessionID != got.sessionID {
		t.Fatalf("pipeline session_id = %q, want %q", pipeline.SessionID, got.sessionID)
	}
	if !strings.Contains(got.composer.commandOutput, "后台 leader 任务") {
		t.Fatalf("expected success output to mention background leader task, got: %s", got.composer.commandOutput)
	}
}

func TestApplyTemplateChoiceCreatesSessionWhenMissing(t *testing.T) {
	m := newResearchEntryModel(t)
	choice := TemplateChoice{
		Template:    "survey",
		FolderName:  t.TempDir(),
		PendingText: "runtime topic",
	}

	model, _ := m.applyTemplateChoice(choice)
	got, ok := model.(*Model)
	if !ok {
		t.Fatalf("expected *Model, got %T", model)
	}
	if got.sessionID == "" {
		t.Fatal("expected applyTemplateChoice to create a session")
	}
	if got.status.researchPipelineID == "" {
		t.Fatal("expected applyTemplateChoice to set researchPipelineID")
	}
	pipeline, err := got.app.ResearchEngine.GetBySession(got.sessionID)
	if err != nil {
		t.Fatalf("expected pipeline for created session: %v", err)
	}
	if pipeline.SessionID != got.sessionID {
		t.Fatalf("pipeline session_id = %q, want %q", pipeline.SessionID, got.sessionID)
	}
	if !strings.Contains(got.composer.commandOutput, "后台 leader 任务") {
		t.Fatalf("expected success output to mention background leader task, got: %s", got.composer.commandOutput)
	}
}

func TestResearchStartCommandFailureDoesNotEnterResearchMode(t *testing.T) {
	m := newResearchEntryModel(t)
	registry := command.NewRegistry()
	registry.Register(staticCommand{
		name: "research",
		result: command.Result{
			Output: "已创建研究流水线，但第 1 阶段启动失败。",
		},
	})
	m.dispatcher = command.NewDispatcher(registry)
	m.composer.input.SetValue(`/research start "runtime topic"`)

	model, _ := m.sendMessage()
	got, ok := model.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", model)
	}
	if got.sessionID == "" {
		t.Fatal("expected /research start command path to create a session")
	}
	if got.status.mode == "research" {
		t.Fatalf("expected failed /research start not to enter research mode")
	}
	if got.status.researchPipelineID != "" {
		t.Fatalf("expected failed /research start not to set researchPipelineID, got %q", got.status.researchPipelineID)
	}
}
