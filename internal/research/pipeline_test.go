package research

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nahasma/openscholar-public/internal/db"
	"github.com/Nahasma/openscholar-public/internal/pubsub"
)

var testGooseMu sync.Mutex

func setupEngine(t *testing.T) *Engine {
	t.Helper()
	conn := setupTestConn(t)

	// 插入测试会话
	conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('sess-1', 'test', 1, 1)`)

	store := NewStore(conn)
	broker := pubsub.NewBroker[ResearchEvent]()
	t.Cleanup(broker.Shutdown)
	return NewEngine(store, broker)
}

func setupTestConn(t *testing.T) *sql.DB {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "test.db")
	conn, err := sql.Open("sqlite3", tmpFile)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	conn.Exec("PRAGMA foreign_keys = ON")
	conn.Exec("PRAGMA journal_mode = WAL")

	testGooseMu.Lock()
	goose.SetBaseFS(db.FS)
	goose.SetDialect("sqlite3")
	err = goose.Up(conn, "migrations")
	testGooseMu.Unlock()
	require.NoError(t, err)
	return conn
}

// --- Pipeline Model Tests ---

// TestNeedsCheckpoint verifies the domain-layer checkpoint logic.
func TestNeedsCheckpoint_AlwaysFalse(t *testing.T) {
	// Renamed but kept for backward-compat with test runner; now tests real logic.
	// ModeStrict: every phase is a checkpoint regardless of Checkpoint flag.
	strict := &Pipeline{Mode: ModeStrict}
	assert.True(t, strict.NeedsCheckpoint(&Phase{Checkpoint: true}))
	assert.True(t, strict.NeedsCheckpoint(&Phase{Checkpoint: false}))

	// ModeAuto: no checkpoints (reviewer agent decides).
	auto := &Pipeline{Mode: ModeAuto}
	assert.False(t, auto.NeedsCheckpoint(&Phase{Checkpoint: true}))
	assert.False(t, auto.NeedsCheckpoint(&Phase{Checkpoint: false}))

	// ModeDefault: respects the Phase.Checkpoint flag.
	def := &Pipeline{Mode: ModeDefault}
	assert.True(t, def.NeedsCheckpoint(&Phase{Checkpoint: true}))
	assert.False(t, def.NeedsCheckpoint(&Phase{Checkpoint: false}))
}

// --- Engine Tests ---

func TestEngineCreate_Empirical(t *testing.T) {
	e := setupEngine(t)
	// 切换到临时目录避免工作目录冲突
	origDir, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(origDir)

	p, err := e.Create("sess-1", "test topic", "empirical", ModeDefault, 8, "")
	require.NoError(t, err)
	assert.NotEmpty(t, p.ID)
	assert.Equal(t, StatusPlanning, p.Status)

	phases, err := e.GetPhases(p.ID)
	require.NoError(t, err)
	assert.Len(t, phases, 5)
	assert.Equal(t, "文献调研", phases[0].Name)
	assert.Equal(t, "审稿修订", phases[4].Name)

	// 验证工作目录结构
	assert.DirExists(t, filepath.Join(p.WorkDir, ".handoff"))
	assert.DirExists(t, filepath.Join(p.WorkDir, ".citations"))
	assert.DirExists(t, filepath.Join(p.WorkDir, "paper"))
	assert.DirExists(t, filepath.Join(p.WorkDir, "code"))
}

func TestEngineCreate_Survey(t *testing.T) {
	e := setupEngine(t)
	origDir, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(origDir)

	p, err := e.Create("sess-1", "survey topic", "survey", ModeAuto, 5, "")
	require.NoError(t, err)
	phases, _ := e.GetPhases(p.ID)
	assert.Len(t, phases, 4)
}

func TestEngineCreate_Theoretical(t *testing.T) {
	e := setupEngine(t)
	origDir, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(origDir)

	p, err := e.Create("sess-1", "theory topic", "theoretical", ModeStrict, 10, "")
	require.NoError(t, err)
	phases, _ := e.GetPhases(p.ID)
	assert.Len(t, phases, 4)
}

func TestEngineCreate_UnknownTemplate(t *testing.T) {
	e := setupEngine(t)
	_, err := e.Create("sess-1", "topic", "unknown", ModeDefault, 5, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown template")
}

func TestEngineCreate_ARISInitializesWorkspace(t *testing.T) {
	e := setupEngine(t)
	origDir, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(origDir)

	p, err := e.Create("sess-1", "topic", "aris_empirical", ModeDefault, 5, "")
	require.NoError(t, err)
	assert.Equal(t, ModeAuto, p.Mode)
	phases, err := e.GetPhases(p.ID)
	require.NoError(t, err)
	assert.Len(t, phases, 7)
	assert.Equal(t, "Idea Discovery", phases[0].Name)
	assert.DirExists(t, filepath.Join(p.WorkDir, ".aris"))
	assert.FileExists(t, filepath.Join(p.WorkDir, ".aris", "run_config.json"))
	assert.FileExists(t, filepath.Join(p.WorkDir, "MANIFEST.md"))
}

func TestEngineCreate_ARISHumanCheckpointKeepsCheckpointMode(t *testing.T) {
	e := setupEngine(t)
	origDir, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(origDir)

	p, err := e.CreateWithPolicy("sess-1", "topic", "aris_empirical", ModeAuto, 5, "", ResearchRunPolicy{HumanCheckpoint: true})
	require.NoError(t, err)
	assert.Equal(t, ModeDefault, p.Mode)
}

func TestEngineAdvance_Auto(t *testing.T) {
	e := setupEngine(t)
	origDir, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(origDir)

	p, _ := e.Create("sess-1", "topic", "empirical", ModeAuto, 8, "")
	phases, _ := e.GetPhases(p.ID)

	// 启动第一阶段
	e.StartPhase(p.ID, phases[0].ID)

	// 推进 — auto 模式应该自动进入下一阶段
	err := e.Advance(p.ID)
	require.NoError(t, err)

	// 第一阶段应该完成，第二阶段应该 running
	updatedPhases, _ := e.GetPhases(p.ID)
	assert.Equal(t, PhaseCompleted, updatedPhases[0].Status)
	assert.Equal(t, PhaseRunning, updatedPhases[1].Status)
}

// TestEngineAdvance_AutoMode verifies that ModeAuto never pauses at a checkpoint.
func TestEngineAdvance_AlwaysAutoAdvances(t *testing.T) {
	e := setupEngine(t)
	origDir, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(origDir)

	// ModeAuto: NeedsCheckpoint always returns false, so Engine.Advance never pauses.
	p, _ := e.Create("sess-1", "topic", "empirical", ModeAuto, 8, "")
	phases, _ := e.GetPhases(p.ID)

	e.StartPhase(p.ID, phases[0].ID)
	err := e.Advance(p.ID)
	require.NoError(t, err)

	// Should auto-advance to phase 2 without pausing.
	updated, _ := e.Get(p.ID)
	assert.Equal(t, StatusRunning, updated.Status)
	updatedPhases, _ := e.GetPhases(p.ID)
	assert.Equal(t, PhaseCompleted, updatedPhases[0].Status)
	assert.Equal(t, PhaseRunning, updatedPhases[1].Status)
}

// TestEngineAdvance_DefaultMode verifies that ModeDefault respects Phase.Checkpoint
// and pauses the pipeline when the phase has Checkpoint: true.
func TestEngineAdvance_DefaultMode_Pauses(t *testing.T) {
	e := setupEngine(t)
	origDir, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(origDir)

	// ModeDefault + survey phase-1 has Checkpoint: true → should pause after Advance.
	p, _ := e.Create("sess-1", "topic", "survey", ModeDefault, 8, "")
	phases, _ := e.GetPhases(p.ID)
	require.True(t, phases[0].Checkpoint, "survey phase 1 should have Checkpoint=true")

	e.StartPhase(p.ID, phases[0].ID)
	err := e.Advance(p.ID)
	require.NoError(t, err)

	// Phase 1 completed, pipeline paused waiting for approval.
	updatedPhases, _ := e.GetPhases(p.ID)
	assert.Equal(t, PhaseCompleted, updatedPhases[0].Status)
	updated, _ := e.Get(p.ID)
	assert.Equal(t, StatusPaused, updated.Status, "pipeline should pause at checkpoint in default mode")
}

func TestEnginePauseResume(t *testing.T) {
	e := setupEngine(t)
	origDir, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(origDir)

	p, _ := e.Create("sess-1", "topic", "empirical", ModeAuto, 8, "")
	e.Pause(p.ID)
	updated, _ := e.Get(p.ID)
	assert.Equal(t, StatusPaused, updated.Status)

	e.Resume(p.ID)
	updated, _ = e.Get(p.ID)
	assert.Equal(t, StatusRunning, updated.Status)
}

func TestEngineBudget(t *testing.T) {
	e := setupEngine(t)
	origDir, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(origDir)

	p, _ := e.Create("sess-1", "topic", "empirical", ModeAuto, 10, "")

	// 80% → 无错误（预警通过 PubSub）
	err := e.UpdateBudget(p.ID, 8)
	assert.NoError(t, err)

	// 100% → ErrBudgetExceeded + 暂停
	err = e.UpdateBudget(p.ID, 10)
	assert.ErrorIs(t, err, ErrBudgetExceeded)
	updated, _ := e.Get(p.ID)
	assert.Equal(t, StatusPaused, updated.Status)
}
