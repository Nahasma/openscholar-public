package research

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openscholar/openscholar/internal/pubsub"
)

// TestPipelineLifecycle_FullFlow exercises the key state transitions of a
// Pipeline from creation through phase advancement to completion.
//
// It uses an in-package test (not kb_test pattern) so it can reuse the
// setupTestConn helper defined in pipeline_test.go.
func TestPipelineLifecycle_FullFlow(t *testing.T) {
	conn := setupTestConn(t)
	conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('sess-full', 'full-flow', 1, 1)`)

	store := NewStore(conn)
	broker := pubsub.NewBroker[ResearchEvent]()
	t.Cleanup(broker.Shutdown)
	engine := NewEngine(store, broker)

	// Use a temp directory as cwd so workspace directories are created there.
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	defer os.Chdir(origDir)

	// --- Create ---
	p, err := engine.Create("sess-full", "integration test topic", "survey", ModeDefault, 20, "")
	require.NoError(t, err)
	assert.NotEmpty(t, p.ID)
	assert.Equal(t, StatusPlanning, p.Status, "new pipeline should be in planning state")

	// Workspace directories must be created.
	assert.DirExists(t, filepath.Join(p.WorkDir, ".handoff"))
	assert.DirExists(t, filepath.Join(p.WorkDir, ".citations"))
	assert.DirExists(t, filepath.Join(p.WorkDir, "paper"))
	assert.DirExists(t, filepath.Join(p.WorkDir, "code"))

	// --- Phases ---
	phases, err := engine.GetPhases(p.ID)
	require.NoError(t, err)
	assert.Len(t, phases, 4, "survey template has 4 phases")

	// All phases should start as pending.
	for _, ph := range phases {
		assert.Equal(t, PhasePending, ph.Status,
			"phase %s should be pending initially", ph.Name)
	}

	// --- Start first phase ---
	require.NoError(t, engine.StartPhase(p.ID, phases[0].ID))
	refreshed, err := engine.GetPhases(p.ID)
	require.NoError(t, err)
	assert.Equal(t, PhaseRunning, refreshed[0].Status, "first phase should be running after StartPhase")

	// --- Advance ---
	// survey/ModeDefault: phase 1 has Checkpoint=true, so Advance completes phase 1
	// and pauses the pipeline waiting for approval.
	require.NoError(t, engine.Advance(p.ID))
	afterAdvance, err := engine.GetPhases(p.ID)
	require.NoError(t, err)
	assert.Equal(t, PhaseCompleted, afterAdvance[0].Status, "first phase should be completed after Advance")
	// Phase 2 is still pending — pipeline paused at checkpoint.
	assert.Equal(t, PhasePending, afterAdvance[1].Status, "second phase should be pending (checkpoint pause)")

	// Pipeline should be paused waiting for approval.
	pipelineState, err := engine.Get(p.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusPaused, pipelineState.Status, "pipeline should be paused at checkpoint")

	// --- Approve checkpoint → resumes and starts phase 2 ---
	require.NoError(t, engine.Approve(p.ID))
	afterApprove, err := engine.GetPhases(p.ID)
	require.NoError(t, err)
	assert.Equal(t, PhaseRunning, afterApprove[1].Status, "second phase should be running after Approve")

	// Pipeline should be running again.
	pipelineState, err = engine.Get(p.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, pipelineState.Status)

	// --- Pause / Resume ---
	require.NoError(t, engine.Pause(p.ID))
	paused, err := engine.Get(p.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusPaused, paused.Status)

	require.NoError(t, engine.Resume(p.ID))
	resumed, err := engine.Get(p.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, resumed.Status)
}

// TestPipelineLifecycle_BudgetExceeded verifies that exhausting the budget
// pauses the pipeline and surfaces ErrBudgetExceeded.
func TestPipelineLifecycle_BudgetExceeded(t *testing.T) {
	conn := setupTestConn(t)
	conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('sess-budget', 'budget', 1, 1)`)

	store := NewStore(conn)
	broker := pubsub.NewBroker[ResearchEvent]()
	t.Cleanup(broker.Shutdown)
	engine := NewEngine(store, broker)

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(t.TempDir()))
	defer os.Chdir(origDir)

	p, err := engine.Create("sess-budget", "budget test", "survey", ModeDefault, 5, "")
	require.NoError(t, err)

	// 80 % consumed — should warn but not error.
	require.NoError(t, engine.UpdateBudget(p.ID, 4))

	// 100 % consumed — must return ErrBudgetExceeded and pause.
	err = engine.UpdateBudget(p.ID, 5)
	assert.ErrorIs(t, err, ErrBudgetExceeded)

	state, err := engine.Get(p.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusPaused, state.Status, "pipeline should be paused when budget is exhausted")
}

// TestPipelineLifecycle_GetAll verifies that a session's pipelines can be
// listed and that the correct pipeline is returned.
func TestPipelineLifecycle_GetAll(t *testing.T) {
	conn := setupTestConn(t)
	conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('sess-list', 'list', 1, 1)`)

	store := NewStore(conn)
	broker := pubsub.NewBroker[ResearchEvent]()
	t.Cleanup(broker.Shutdown)
	engine := NewEngine(store, broker)

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(t.TempDir()))
	defer os.Chdir(origDir)

	_, err := engine.Create("sess-list", "topic-a", "survey", ModeDefault, 5, "")
	require.NoError(t, err)
	_, err = engine.Create("sess-list", "topic-b", "empirical", ModeAuto, 8, "")
	require.NoError(t, err)

	// GetBySession returns the most recently created pipeline for the session.
	latest, err := engine.GetBySession("sess-list")
	require.NoError(t, err)
	assert.NotNil(t, latest, "GetBySession should return a pipeline")
	assert.Equal(t, "sess-list", latest.SessionID)
}
