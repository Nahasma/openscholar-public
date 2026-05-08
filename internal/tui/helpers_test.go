package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/openscholar/openscholar/internal/app"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/research"
	"github.com/openscholar/openscholar/internal/testutil"
)

func TestBuildResearchContext_UsesValidResearchPipelineActions(t *testing.T) {
	conn, _ := testutil.SetupTestDB(t)
	if _, err := conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('sess-research-context', 'context', 1, 1)`); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	broker := pubsub.NewBroker[research.ResearchEvent]()
	t.Cleanup(broker.Shutdown)

	engine := research.NewEngine(research.NewStore(conn), broker)
	pipeline, err := engine.Create("sess-research-context", "research context topic", "survey", research.ModeAuto, 8, t.TempDir())
	if err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	phases, err := engine.GetPhases(pipeline.ID)
	if err != nil {
		t.Fatalf("get phases: %v", err)
	}
	if len(phases) == 0 {
		t.Fatal("expected at least one phase")
	}
	if err := engine.StartPhase(pipeline.ID, phases[0].ID); err != nil {
		t.Fatalf("start phase: %v", err)
	}

	model := &Model{
		ctx: context.Background(),
		app: &app.App{ResearchEngine: engine},
	}
	model.status.researchPipelineID = pipeline.ID

	researchCtx := model.buildResearchContext()
	if strings.Contains(researchCtx, "ResearchPipeline(checkpoint/advance)") {
		t.Fatalf("research context should not reference invalid ResearchPipeline action: %s", researchCtx)
	}
	if !strings.Contains(researchCtx, `ResearchPipeline(action="advance")`) {
		t.Fatalf("research context should reference ResearchPipeline(action=\"advance\"): %s", researchCtx)
	}
	if !strings.Contains(researchCtx, "checkpoint dialog") {
		t.Fatalf("research context should describe checkpoint dialog flow: %s", researchCtx)
	}
}
