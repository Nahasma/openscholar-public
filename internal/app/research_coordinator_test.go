package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/research"
	researchcoord "github.com/openscholar/openscholar/internal/research/coordinator"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/task"
	"github.com/openscholar/openscholar/internal/testutil"
)

func waitForRegistryStatus(t *testing.T, reg *task.Registry, taskID string, want task.Status) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, ok := reg.Get(taskID)
		if ok && state.TaskMeta().Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	state, ok := reg.Get(taskID)
	if !ok {
		t.Fatalf("task %s not found", taskID)
	}
	t.Fatalf("task %s status=%s, want %s", taskID, state.TaskMeta().Status, want)
}

func newAutoApproveCheckpointBroker(t *testing.T) *pubsub.Broker[tools.CheckpointEvent] {
	t.Helper()
	broker := pubsub.NewBroker[tools.CheckpointEvent]()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(broker.Shutdown)

	ch := broker.Subscribe(ctx)
	go func() {
		for evt := range ch {
			evt.Payload.ResponseCh <- tools.CheckpointResponse{Approved: true}
		}
	}()

	return broker
}

func loadResearchOrchestrationConfig(t *testing.T, enabled bool, resultMaxChars int) {
	t.Helper()
	config.Reset()
	t.Cleanup(config.Reset)

	wd := t.TempDir()
	enabledValue := "false"
	if enabled {
		enabledValue = "true"
	}
	cfg := fmt.Sprintf(`{
  "defaultProvider": "ollama",
  "providers": {
    "ollama": {"model": "qwen2.5-coder:latest"}
  },
  "subagent_orchestration": {
    "enabled": %s,
    "default_result_max_chars": %d
  }
}`, enabledValue, resultMaxChars)
	if err := os.MkdirAll(filepath.Dir(config.ConfigFilePath(wd)), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(config.ConfigFilePath(wd), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := config.Load(wd); err != nil {
		t.Fatalf("load config: %v", err)
	}
}

func assertResearchPhaseState(t *testing.T, engine *research.Engine, pipelineID, phaseID string, wantPipeline research.PipelineStatus, wantPhase research.PhaseStatus) {
	t.Helper()
	pipeline, err := engine.Get(pipelineID)
	if err != nil {
		t.Fatalf("get pipeline: %v", err)
	}
	if pipeline.Status != wantPipeline {
		t.Fatalf("pipeline status = %s, want %s", pipeline.Status, wantPipeline)
	}
	phases, err := engine.GetPhases(pipelineID)
	if err != nil {
		t.Fatalf("get phases: %v", err)
	}
	for _, phase := range phases {
		if phase.ID == phaseID {
			if phase.Status != wantPhase {
				t.Fatalf("phase status = %s, want %s", phase.Status, wantPhase)
			}
			return
		}
	}
	t.Fatalf("phase %s not found", phaseID)
}

func TestBuildResearchLeaderPrompt_ARISUsesContractPrompt(t *testing.T) {
	pipeline := &research.Pipeline{
		ID:       "pipe-aris",
		Topic:    "aris topic",
		Template: "aris_empirical",
		WorkDir:  "/tmp/aris-work",
	}
	phase := &research.Phase{Order: 5, Name: "Claim & Integrity Gate"}

	prompt := buildResearchLeaderPrompt(pipeline, phase)
	for _, want := range []string{
		"ARIS research leader",
		"Reviewer independence is mandatory",
		"/tmp/aris-work/.aris/run_config.json",
		"/tmp/aris-work/MANIFEST.md",
		"EXPERIMENT_AUDIT.json",
		"must not skip mandatory ARIS audits",
		"TaskV2 worker deliverables plus independent verification",
		"ResearchPipeline(action=\"advance\"",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("ARIS prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildResearchRuntimeContext_ARISReferencesConfigPathsOnly(t *testing.T) {
	app := &App{}
	ctx := app.buildResearchRuntimeContext(&research.Pipeline{
		ID:       "pipe-aris",
		Topic:    "aris topic",
		Template: "aris_empirical",
		WorkDir:  "/tmp/aris-work",
	})

	for _, want := range []string{
		"Template: aris_empirical",
		"ARIS run config: /tmp/aris-work/.aris/run_config.json",
		"ARIS output manifest: /tmp/aris-work/MANIFEST.md",
	} {
		if !strings.Contains(ctx, want) {
			t.Fatalf("runtime context missing %q:\n%s", want, ctx)
		}
	}
	if strings.Contains(ctx, "IDEA_REPORT") || strings.Contains(ctx, "AUTO_REVIEW") {
		t.Fatalf("runtime context should reference ARIS config paths, not artifact contents:\n%s", ctx)
	}
}

func TestResearchCoordinatorRuntimeTracksStartedPhase(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	if _, err := conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('sess-research-runtime', 'runtime', 1, 1)`); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := pubsub.NewBroker[research.ResearchEvent]()
	t.Cleanup(broker.Shutdown)

	engine := research.NewEngine(research.NewStore(conn), broker)
	reg := task.NewRegistry()
	defer reg.Shutdown()
	tools.SetTaskRuntime(reg, nil)
	t.Cleanup(func() { tools.SetTaskRuntime(nil, nil) })

	var workDir string

	app := &App{
		ResearchEngine:   engine,
		ResearchEvents:   broker,
		CheckpointBroker: newAutoApproveCheckpointBroker(t),
		TaskRegistry:     reg,
		Permissions:      permission.NewPermissionService(),
		Sessions:         session.NewService(q),
		Messages:         message.NewService(q),
		taskAgentRunner: func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []tools.BaseTool) (string, error) {
			if agentName == config.AgentVerify {
				return researchVerifyMockOutput(prompt), nil
			}
			if err := os.WriteFile(filepath.Join(workDir, ".handoff", "01-literature.md"), []byte("phase deliverable"), 0o644); err != nil {
				return "", err
			}
			resp, err := runResearchPipelineAdvance(ctx, sessionID, agentTools, "phase finished")
			if err != nil {
				return "", err
			}
			return resp.Content, nil
		},
	}
	app.initResearchCoordinatorRuntime(ctx)

	if app.ResearchCoordinator == nil {
		t.Fatal("expected research coordinator to be initialized")
	}
	if app.ResearchWorkerPool == nil {
		t.Fatal("expected research worker pool to be initialized")
	}

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer os.Chdir(origDir)

	pipeline, err := engine.Create("sess-research-runtime", "runtime topic", "survey", research.ModeDefault, 0, "")
	if err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	workDir = pipeline.WorkDir
	phases, err := engine.GetPhases(pipeline.ID)
	if err != nil {
		t.Fatalf("get phases: %v", err)
	}
	if len(phases) == 0 {
		t.Fatal("expected at least one phase")
	}

	firstPhase := phases[0]
	if err := engine.StartPhase(pipeline.ID, firstPhase.ID); err != nil {
		t.Fatalf("start phase: %v", err)
	}

	waitForRegistryStatus(t, reg, researchPhaseTaskID(firstPhase.ID), task.StatusCompleted)
	assertResearchPhaseState(t, engine, pipeline.ID, firstPhase.ID, research.StatusRunning, research.PhaseCompleted)
}

func TestResearchPhaseExecutorHandlesFastCompletion(t *testing.T) {
	conn, _ := testutil.SetupTestDB(t)
	if _, err := conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('sess-research-fast', 'fast', 1, 1)`); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	broker := pubsub.NewBroker[research.ResearchEvent]()
	t.Cleanup(broker.Shutdown)

	engine := research.NewEngine(research.NewStore(conn), broker)
	executor := &researchPhaseExecutor{
		engine: engine,
		events: broker,
	}

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer os.Chdir(origDir)

	pipeline, err := engine.Create("sess-research-fast", "fast topic", "survey", research.ModeDefault, 0, "")
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

	firstPhase := phases[0]
	if err := engine.StartPhase(pipeline.ID, firstPhase.ID); err != nil {
		t.Fatalf("start phase: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- executor.RunPhase(ctx, &researchcoord.PipelineRef{
			ID:        pipeline.ID,
			SessionID: pipeline.SessionID,
			Topic:     pipeline.Topic,
			WorkDir:   pipeline.WorkDir,
		}, &researchcoord.PhaseRef{
			ID:         firstPhase.ID,
			Name:       firstPhase.Name,
			Order:      firstPhase.Order,
			MaxWorkers: firstPhase.MaxWorkers,
		})
	}()

	if err := engine.Advance(pipeline.ID); err != nil {
		t.Fatalf("advance pipeline: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("executor returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("executor did not observe fast completion")
	}
}

func TestResearchCoordinatorRuntimeStartsLeaderTask(t *testing.T) {
	loadResearchOrchestrationConfig(t, true, 64)

	conn, q := testutil.SetupTestDB(t)
	if _, err := conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('sess-research-leader-start', 'runtime', 1, 1)`); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := pubsub.NewBroker[research.ResearchEvent]()
	t.Cleanup(broker.Shutdown)

	engine := research.NewEngine(research.NewStore(conn), broker)
	reg := task.NewRegistry()
	defer reg.Shutdown()
	tools.SetTaskRuntime(reg, nil)
	t.Cleanup(func() { tools.SetTaskRuntime(nil, nil) })

	var workDir string

	app := &App{
		ResearchEngine:   engine,
		ResearchEvents:   broker,
		CheckpointBroker: newAutoApproveCheckpointBroker(t),
		TaskRegistry:     reg,
		Permissions:      permission.NewPermissionService(),
		Sessions:         session.NewService(q),
		Messages:         message.NewService(q),
		taskAgentRunner: func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []tools.BaseTool) (string, error) {
			if agentName == config.AgentVerify {
				return researchVerifyMockOutput(prompt), nil
			}
			if agentName != config.AgentLeader {
				return "", errors.New("expected leader agent")
			}
			if err := os.WriteFile(filepath.Join(workDir, ".handoff", "01-literature.md"), []byte("phase deliverable"), 0o644); err != nil {
				return "", err
			}
			resp, err := runResearchPipelineAdvance(ctx, sessionID, agentTools, "phase finished")
			if err != nil {
				return "", err
			}
			return resp.Content, nil
		},
	}
	app.initResearchCoordinatorRuntime(ctx)

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer os.Chdir(origDir)

	pipeline, err := engine.Create("sess-research-leader-start", "runtime topic", "survey", research.ModeDefault, 0, "")
	if err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	workDir = pipeline.WorkDir
	phases, err := engine.GetPhases(pipeline.ID)
	if err != nil {
		t.Fatalf("get phases: %v", err)
	}
	if len(phases) == 0 {
		t.Fatal("expected at least one phase")
	}

	firstPhase := phases[0]
	if err := engine.StartPhase(pipeline.ID, firstPhase.ID); err != nil {
		t.Fatalf("start phase: %v", err)
	}

	leaderTaskID := researchPhaseLeaderTaskID(firstPhase.ID)
	waitForRegistryStatus(t, reg, leaderTaskID, task.StatusCompleted)
	waitForRegistryStatus(t, reg, researchPhaseTaskID(firstPhase.ID), task.StatusCompleted)
	state, ok := reg.Get(leaderTaskID)
	if !ok {
		t.Fatalf("leader task %s not found", leaderTaskID)
	}
	sub, ok := state.(*task.SubtaskState)
	if !ok {
		t.Fatalf("leader task %s type mismatch", leaderTaskID)
	}
	if sub.VerifyPolicy != "required" {
		t.Fatalf("leader task verify policy = %q, want required", sub.VerifyPolicy)
	}
	if sub.VerifyStatus != task.VerifyStatusPassed {
		t.Fatalf("leader task verify status = %s, want %s", sub.VerifyStatus, task.VerifyStatusPassed)
	}
	if sub.VerifyVerdict != "PASS" {
		t.Fatalf("leader task verify verdict = %q, want PASS", sub.VerifyVerdict)
	}
	if sub.VerifyResult != "verified" {
		t.Fatalf("leader task verify result = %q, want verified", sub.VerifyResult)
	}
	if sub.FanoutGroup != researchFanoutGroup(pipeline.ID) {
		t.Fatalf("leader task fanout group = %q, want %q", sub.FanoutGroup, researchFanoutGroup(pipeline.ID))
	}
	if got, want := sub.TaskMeta().ParentTaskID, researchPhaseTaskID(firstPhase.ID); got != want {
		t.Fatalf("leader task parent task id = %q, want %q", got, want)
	}
	if !sub.TaskMeta().IsBackgrounded {
		t.Fatal("leader task should be backgrounded")
	}
	if sub.TaskMeta().Notified {
		t.Fatal("leader task should remain notification-ready until TaskNotificationDrain delivers it")
	}
	if sub.ResultMaxChars != 64 {
		t.Fatalf("leader task result cap = %d, want 64", sub.ResultMaxChars)
	}
	assertResearchPhaseState(t, engine, pipeline.ID, firstPhase.ID, research.StatusRunning, research.PhaseCompleted)
}

func TestResearchCoordinatorRuntimeResumesLeaderAfterWorkerNotification(t *testing.T) {
	loadResearchOrchestrationConfig(t, true, 256)

	conn, q := testutil.SetupTestDB(t)
	if _, err := conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('sess-research-leader-resume', 'runtime', 1, 1)`); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := pubsub.NewBroker[research.ResearchEvent]()
	t.Cleanup(broker.Shutdown)

	engine := research.NewEngine(research.NewStore(conn), broker)
	reg := task.NewRegistry()
	defer reg.Shutdown()
	tools.SetTaskRuntime(reg, nil)
	t.Cleanup(func() { tools.SetTaskRuntime(nil, nil) })

	leaderCalls := 0
	var workDir string
	workerTaskID := "research-leader-worker-resume"
	app := &App{
		ResearchEngine:   engine,
		ResearchEvents:   broker,
		CheckpointBroker: newAutoApproveCheckpointBroker(t),
		TaskRegistry:     reg,
		Permissions:      permission.NewPermissionService(),
		Sessions:         session.NewService(q),
		Messages:         message.NewService(q),
		taskAgentRunner: func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []tools.BaseTool) (string, error) {
			switch agentName {
			case config.AgentVerify:
				return researchVerifyMockOutput(prompt), nil
			case config.AgentExplore:
				if err := os.WriteFile(filepath.Join(workDir, ".handoff", "01-literature.md"), []byte("phase deliverable"), 0o644); err != nil {
					return "", err
				}
				return "worker finding", nil
			case config.AgentLeader:
				leaderCalls++
				if leaderCalls == 1 {
					if err := runTaskCreate(ctx, sessionID, agentTools, workerTaskID, "explore", "inspect phase inputs"); err != nil {
						return "", err
					}
					return "waiting for worker", nil
				}
				reg.MarkNotificationDelivered(workerTaskID)
				resp, err := runResearchPipelineAdvance(ctx, sessionID, agentTools, "phase finished after worker notification")
				if err != nil {
					return "", err
				}
				return resp.Content, nil
			default:
				return "", fmt.Errorf("unexpected agent %s", agentName)
			}
		},
	}
	app.initResearchCoordinatorRuntime(ctx)

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer os.Chdir(origDir)

	pipeline, err := engine.Create("sess-research-leader-resume", "runtime topic", "survey", research.ModeDefault, 0, "")
	if err != nil {
		t.Fatalf("create pipeline: %v", err)
	}
	workDir = pipeline.WorkDir
	phases, err := engine.GetPhases(pipeline.ID)
	if err != nil {
		t.Fatalf("get phases: %v", err)
	}
	firstPhase := phases[0]
	if err := engine.StartPhase(pipeline.ID, firstPhase.ID); err != nil {
		t.Fatalf("start phase: %v", err)
	}

	waitForRegistryStatus(t, reg, researchPhaseTaskID(firstPhase.ID), task.StatusCompleted)
	if leaderCalls < 2 {
		t.Fatalf("leader should have resumed after worker notification, calls=%d", leaderCalls)
	}
	waitForRegistryStatus(t, reg, workerTaskID, task.StatusCompleted)
	assertResearchPhaseState(t, engine, pipeline.ID, firstPhase.ID, research.StatusRunning, research.PhaseCompleted)
}

func TestResearchCoordinatorRuntimeMarksPhaseFailedOnLeaderFailure(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	if _, err := conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('sess-research-leader-fail', 'runtime', 1, 1)`); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := pubsub.NewBroker[research.ResearchEvent]()
	t.Cleanup(broker.Shutdown)

	engine := research.NewEngine(research.NewStore(conn), broker)
	reg := task.NewRegistry()
	defer reg.Shutdown()
	tools.SetTaskRuntime(reg, nil)
	t.Cleanup(func() { tools.SetTaskRuntime(nil, nil) })

	app := &App{
		ResearchEngine:   engine,
		ResearchEvents:   broker,
		CheckpointBroker: newAutoApproveCheckpointBroker(t),
		TaskRegistry:     reg,
		Permissions:      permission.NewPermissionService(),
		Sessions:         session.NewService(q),
		Messages:         message.NewService(q),
		taskAgentRunner: func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []tools.BaseTool) (string, error) {
			return "", errors.New("leader failed")
		},
	}
	app.initResearchCoordinatorRuntime(ctx)

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer os.Chdir(origDir)

	pipeline, err := engine.Create("sess-research-leader-fail", "runtime topic", "survey", research.ModeDefault, 0, "")
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

	firstPhase := phases[0]
	if err := engine.StartPhase(pipeline.ID, firstPhase.ID); err != nil {
		t.Fatalf("start phase: %v", err)
	}

	taskID := researchPhaseTaskID(firstPhase.ID)
	waitForRegistryStatus(t, reg, taskID, task.StatusFailed)
	assertResearchPhaseState(t, engine, pipeline.ID, firstPhase.ID, research.StatusFailed, research.PhaseFailed)

	leaderTaskID := researchPhaseLeaderTaskID(firstPhase.ID)
	state, ok := reg.Get(leaderTaskID)
	if !ok {
		t.Fatalf("leader task %s not found", leaderTaskID)
	}
	sub, ok := state.(*task.SubtaskState)
	if !ok {
		t.Fatalf("leader task %s type mismatch", leaderTaskID)
	}
	if !strings.Contains(sub.LastError, "leader failed") {
		t.Fatalf("leader task last error = %q, want contains %q", sub.LastError, "leader failed")
	}
}

func TestResearchCoordinatorRuntimeFailsPhaseWhenLeaderCompletesWithoutAdvancing(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	if _, err := conn.Exec(`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ('sess-research-leader-stall', 'runtime', 1, 1)`); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := pubsub.NewBroker[research.ResearchEvent]()
	t.Cleanup(broker.Shutdown)

	engine := research.NewEngine(research.NewStore(conn), broker)
	reg := task.NewRegistry()
	defer reg.Shutdown()
	tools.SetTaskRuntime(reg, nil)
	t.Cleanup(func() { tools.SetTaskRuntime(nil, nil) })

	app := &App{
		ResearchEngine:   engine,
		ResearchEvents:   broker,
		CheckpointBroker: newAutoApproveCheckpointBroker(t),
		TaskRegistry:     reg,
		Permissions:      permission.NewPermissionService(),
		Sessions:         session.NewService(q),
		Messages:         message.NewService(q),
		taskAgentRunner: func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []tools.BaseTool) (string, error) {
			if agentName == config.AgentVerify {
				return researchVerifyMockOutput(prompt), nil
			}
			return "leader exited without advancing", nil
		},
	}
	app.initResearchCoordinatorRuntime(ctx)

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer os.Chdir(origDir)

	pipeline, err := engine.Create("sess-research-leader-stall", "runtime topic", "survey", research.ModeDefault, 0, "")
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

	firstPhase := phases[0]
	if err := engine.StartPhase(pipeline.ID, firstPhase.ID); err != nil {
		t.Fatalf("start phase: %v", err)
	}

	waitForRegistryStatus(t, reg, researchPhaseLeaderTaskID(firstPhase.ID), task.StatusCompleted)
	waitForRegistryStatus(t, reg, researchPhaseTaskID(firstPhase.ID), task.StatusFailed)
	assertResearchPhaseState(t, engine, pipeline.ID, firstPhase.ID, research.StatusFailed, research.PhaseFailed)
}

func runResearchPipelineAdvance(ctx context.Context, childSessionID string, agentTools []tools.BaseTool, summary string) (tools.ToolResponse, error) {
	for _, tool := range agentTools {
		if tool.Info().Name != "ResearchPipeline" {
			continue
		}
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, childSessionID)
		resp, err := tool.Run(toolCtx, tools.ToolCall{
			ID:    "research-advance",
			Name:  "ResearchPipeline",
			Input: fmt.Sprintf(`{"action":"advance","summary":%q}`, summary),
		})
		if err != nil {
			return resp, err
		}
		if resp.IsError {
			return resp, errors.New(resp.Content)
		}
		return resp, nil
	}
	return tools.ToolResponse{}, errors.New("ResearchPipeline tool not found")
}

func runTaskCreate(ctx context.Context, parentSessionID string, agentTools []tools.BaseTool, taskID string, agentType string, prompt string) error {
	for _, tool := range agentTools {
		if tool.Info().Name != "Task" {
			continue
		}
		toolCtx := context.WithValue(ctx, tools.SessionIDContextKey, parentSessionID)
		resp, err := tool.Run(toolCtx, tools.ToolCall{
			ID:   taskID,
			Name: "Task",
			Input: fmt.Sprintf(
				`{"action":"create","task_id":%q,"description":"worker","prompt":%q,"agent_type":%q}`,
				taskID,
				prompt,
				agentType,
			),
		})
		if err != nil {
			return err
		}
		if resp.IsError {
			return errors.New(resp.Content)
		}
		return nil
	}
	return errors.New("Task tool not found")
}

func researchVerifyMockOutput(prompt string) string {
	if strings.Contains(prompt, "VERDICT: PASS|FAIL|PARTIAL") {
		return "verified\nVERDICT: PASS"
	}
	return `{"score":8,"report":"verified"}`
}
