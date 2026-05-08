package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/hooks"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/research"
	researchcoord "github.com/openscholar/openscholar/internal/research/coordinator"
	"github.com/openscholar/openscholar/internal/task"
)

const defaultResearchPhaseReadConcurrency = 4
const researchLeaderCompletionGracePeriod = 500 * time.Millisecond

var (
	errResearchLeaderFailed   = errors.New("research leader task failed")
	errResearchLeaderCanceled = errors.New("research leader task canceled")
	errResearchLeaderStalled  = errors.New("research leader task completed before phase advanced")
)

type researchPhaseExecutor struct {
	engine *research.Engine
	events *pubsub.Broker[research.ResearchEvent]
}

func (e *researchPhaseExecutor) RunPhase(
	ctx context.Context,
	pipeline *researchcoord.PipelineRef,
	phase *researchcoord.PhaseRef,
) error {
	if e.engine == nil {
		return errors.New("research engine is required")
	}
	if pipeline == nil || phase == nil {
		return errors.New("pipeline and phase are required")
	}

	if e.events == nil {
		if done, err := e.phaseResult(pipeline.ID, phase.ID); done {
			return err
		}
		return nil
	}
	sub := e.events.Subscribe(ctx)
	if done, err := e.phaseResult(pipeline.ID, phase.ID); done {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt, ok := <-sub:
			if !ok {
				return ctx.Err()
			}
			if evt.Payload.PipelineID != pipeline.ID {
				continue
			}
			if evt.Payload.Type == research.EventBudgetExceeded {
				return research.ErrBudgetExceeded
			}
			if evt.Payload.PhaseID != phase.ID {
				continue
			}
			switch evt.Payload.Type {
			case research.EventPhaseCompleted, research.EventCheckpoint:
				return nil
			case research.EventPhaseFailed:
				return fmt.Errorf("research phase %s failed", phase.Name)
			}
		}
	}
}

func (e *researchPhaseExecutor) phaseResult(pipelineID, phaseID string) (bool, error) {
	phases, err := e.engine.GetPhases(pipelineID)
	if err != nil {
		return false, err
	}
	for _, phase := range phases {
		if phase.ID != phaseID {
			continue
		}
		switch phase.Status {
		case research.PhaseCompleted:
			return true, nil
		case research.PhaseFailed:
			return true, fmt.Errorf("research phase %s failed", phase.Name)
		default:
			return false, nil
		}
	}
	return true, fmt.Errorf("research phase %s not found", phaseID)
}

func (app *App) initResearchCoordinatorRuntime(ctx context.Context) {
	if app.ResearchEngine == nil || app.ResearchEvents == nil {
		return
	}
	if app.subagentScheduler == nil {
		app.subagentScheduler = tools.SharedSubagentScheduler()
	}
	if app.ResearchWorkerPool == nil {
		app.ResearchWorkerPool = researchcoord.NewWorkerPool(defaultResearchPhaseReadConcurrency)
	}
	if app.ResearchCoordinator == nil {
		app.ResearchCoordinator = researchcoord.NewCoordinatorWithNotifier(
			&researchPhaseExecutor{
				engine: app.ResearchEngine,
				events: app.ResearchEvents,
			},
			func(_ context.Context, payload string) error {
				slog.Debug("research coordinator notification", "payload", payload)
				return nil
			},
		)
	}
	go app.watchResearchPhaseStarts(ctx)
}

func (app *App) watchResearchPhaseStarts(ctx context.Context) {
	if app.ResearchEvents == nil {
		return
	}
	sub := app.ResearchEvents.Subscribe(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-sub:
			if !ok {
				return
			}
			if evt.Payload.Type != research.EventPhaseStarted {
				continue
			}
			app.handleResearchPhaseStarted(ctx, evt.Payload)
		}
	}
}

func (app *App) handleResearchPhaseStarted(ctx context.Context, event research.ResearchEvent) {
	if app.ResearchCoordinator == nil || app.ResearchEngine == nil {
		return
	}
	pipeline, phase, nextPhase, err := app.loadResearchPhaseContext(event.PipelineID, event.PhaseID)
	if err != nil {
		slog.Warn("research coordinator skipped phase",
			"pipeline_id", event.PipelineID,
			"phase_id", event.PhaseID,
			"error", err,
		)
		return
	}

	taskID := researchPhaseTaskID(phase.ID)
	app.registerResearchPhaseTask(ctx, taskID, pipeline, phase)
	leaderTaskID, err := app.launchResearchPhaseLeaderTask(ctx, pipeline, phase)
	if err != nil {
		app.completeResearchPhaseTask(ctx, taskID, pipeline, phase, err)
		slog.Warn("research coordinator failed to start leader task",
			"pipeline_id", pipeline.ID,
			"phase_id", phase.ID,
			"error", err,
		)
		return
	}

	go func() {
		if err := app.runResearchPhase(ctx, taskID, pipeline, phase, nextPhase, leaderTaskID); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("research phase coordinator run failed",
				"pipeline_id", pipeline.ID,
				"phase_id", phase.ID,
				"error", err,
			)
		}
	}()
}

func (app *App) runResearchPhase(
	ctx context.Context,
	taskID string,
	pipeline *research.Pipeline,
	phase *research.Phase,
	nextPhase *researchcoord.PhaseRef,
	leaderTaskID string,
) error {
	if app.ResearchCoordinator == nil {
		return errors.New("research coordinator is not configured")
	}

	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	var releases []func()
	if app.ResearchWorkerPool != nil {
		releases = app.ResearchWorkerPool.AcquireWrite([]string{pipeline.WorkDir})
		defer app.ResearchWorkerPool.ReleaseAll(releases)
	}

	phaseDoneCh := make(chan error, 1)
	go func() {
		phaseDoneCh <- app.ResearchCoordinator.RunPhase(
			runCtx,
			taskID,
			&researchcoord.PipelineRef{
				ID:        pipeline.ID,
				SessionID: pipeline.SessionID,
				Topic:     pipeline.Topic,
				WorkDir:   pipeline.WorkDir,
			},
			&researchcoord.PhaseRef{
				ID:         phase.ID,
				Name:       phase.Name,
				Order:      phase.Order,
				MaxWorkers: phase.MaxWorkers,
			},
			nextPhase,
		)
	}()

	leaderDoneCh := make(chan error, 1)
	if strings.TrimSpace(leaderTaskID) != "" {
		go func() {
			leaderDoneCh <- app.waitResearchLeaderTask(runCtx, leaderTaskID)
		}()
	}

	var err error
	if strings.TrimSpace(leaderTaskID) == "" {
		err = <-phaseDoneCh
	} else {
		var leaderCompletionTimer *time.Timer
		var leaderCompletionCh <-chan time.Time
		for {
			select {
			case err = <-phaseDoneCh:
				if leaderCompletionTimer != nil {
					leaderCompletionTimer.Stop()
				}
				app.completeResearchPhaseTask(ctx, taskID, pipeline, phase, err)
				return err
			case leaderErr := <-leaderDoneCh:
				if leaderErr != nil {
					if leaderCompletionTimer != nil {
						leaderCompletionTimer.Stop()
					}
					runCancel()
					<-phaseDoneCh
					err = leaderErr
					app.completeResearchPhaseTask(ctx, taskID, pipeline, phase, err)
					return err
				} else {
					leaderDoneCh = nil
					leaderCompletionTimer = time.NewTimer(researchLeaderCompletionGracePeriod)
					leaderCompletionCh = leaderCompletionTimer.C
				}
			case <-leaderCompletionCh:
				runCancel()
				<-phaseDoneCh
				err = errResearchLeaderStalled
				app.completeResearchPhaseTask(ctx, taskID, pipeline, phase, err)
				return err
			case <-ctx.Done():
				if leaderCompletionTimer != nil {
					leaderCompletionTimer.Stop()
				}
				err = ctx.Err()
				app.completeResearchPhaseTask(ctx, taskID, pipeline, phase, err)
				return err
			}
		}
	}

	app.completeResearchPhaseTask(ctx, taskID, pipeline, phase, err)
	return err
}

func (app *App) loadResearchPhaseContext(
	pipelineID string,
	phaseID string,
) (*research.Pipeline, *research.Phase, *researchcoord.PhaseRef, error) {
	pipeline, err := app.ResearchEngine.Get(pipelineID)
	if err != nil {
		return nil, nil, nil, err
	}
	phases, err := app.ResearchEngine.GetPhases(pipelineID)
	if err != nil {
		return nil, nil, nil, err
	}

	var current *research.Phase
	var next *researchcoord.PhaseRef
	for i, phase := range phases {
		if phase.ID != phaseID {
			continue
		}
		current = phase
		if i+1 < len(phases) {
			next = &researchcoord.PhaseRef{
				ID:         phases[i+1].ID,
				Name:       phases[i+1].Name,
				Order:      phases[i+1].Order,
				MaxWorkers: phases[i+1].MaxWorkers,
			}
		}
		break
	}
	if current == nil {
		return nil, nil, nil, fmt.Errorf("research phase %s not found", phaseID)
	}

	return pipeline, current, next, nil
}

func (app *App) registerResearchPhaseTask(
	ctx context.Context,
	taskID string,
	pipeline *research.Pipeline,
	phase *research.Phase,
) {
	if app.TaskRegistry != nil {
		app.TaskRegistry.Register(&task.BaseState{M: task.Meta{
			ID:             taskID,
			Label:          fmt.Sprintf("Research Phase %d: %s", phase.Order, phase.Name),
			SessionID:      pipeline.SessionID,
			Kind:           task.KindResearchPhase,
			Status:         task.StatusRunning,
			StartedAt:      time.Now(),
			IsBackgrounded: true,
			Notified:       false,
		}})
	}
	app.emitResearchTaskHook(ctx, hooks.TaskCreated, pipeline.SessionID)
}

func (app *App) completeResearchPhaseTask(
	ctx context.Context,
	taskID string,
	pipeline *research.Pipeline,
	phase *research.Phase,
	runErr error,
) {
	now := time.Now()
	status := app.resolveResearchPhaseTaskStatus(ctx, pipeline, phase, runErr)

	if app.TaskRegistry != nil {
		app.TaskRegistry.Mutate(taskID, func(state task.State) {
			meta := state.TaskMeta()
			meta.Status = status
			meta.EndedAt = &now
			meta.Notified = false
		})
	}
	if pipeline != nil {
		app.emitResearchTaskHook(ctx, hooks.TaskCompleted, pipeline.SessionID)
	}
}

func (app *App) resolveResearchPhaseTaskStatus(
	ctx context.Context,
	pipeline *research.Pipeline,
	phase *research.Phase,
	runErr error,
) task.Status {
	switch {
	case runErr == nil:
		return task.StatusCompleted
	case errors.Is(runErr, context.Canceled):
		return task.StatusCanceled
	case errors.Is(runErr, research.ErrBudgetExceeded):
		return task.StatusCanceled
	}

	app.failResearchPhase(ctx, pipeline, phase, runErr)

	switch app.currentResearchPhaseStatus(pipeline, phase) {
	case research.PhaseCompleted:
		return task.StatusCompleted
	case research.PhaseFailed:
		return task.StatusFailed
	default:
		return task.StatusFailed
	}
}

func (app *App) failResearchPhase(ctx context.Context, pipeline *research.Pipeline, phase *research.Phase, runErr error) {
	if app.ResearchEngine == nil || pipeline == nil || phase == nil || runErr == nil {
		return
	}
	failure := research.PhaseFailureData{
		Reason:      classifyResearchPhaseFailure(runErr),
		Error:       strings.TrimSpace(runErr.Error()),
		Recoverable: false,
	}
	_, _ = app.ResearchEngine.FailPhase(pipeline.ID, phase.ID, failure)
}

func (app *App) currentResearchPhaseStatus(pipeline *research.Pipeline, phase *research.Phase) research.PhaseStatus {
	if app.ResearchEngine == nil || pipeline == nil || phase == nil {
		return ""
	}
	phases, err := app.ResearchEngine.GetPhases(pipeline.ID)
	if err != nil {
		return ""
	}
	for _, item := range phases {
		if item.ID == phase.ID {
			return item.Status
		}
	}
	return ""
}

func classifyResearchPhaseFailure(err error) research.PhaseFailureReason {
	switch {
	case errors.Is(err, errResearchLeaderFailed):
		return research.FailureReasonLeaderFailed
	case errors.Is(err, errResearchLeaderCanceled):
		return research.FailureReasonLeaderCanceled
	case errors.Is(err, errResearchLeaderStalled):
		return research.FailureReasonLeaderStalled
	default:
		return research.FailureReasonExecution
	}
}

func (app *App) emitResearchTaskHook(ctx context.Context, event hooks.Event, sessionID string) {
	if app.HookService == nil {
		return
	}
	_ = app.HookService.Run(context.WithoutCancel(ctx), event, hooks.Input{
		SessionID: sessionID,
		Timestamp: time.Now(),
	})
}

func researchPhaseTaskID(phaseID string) string {
	return "research-phase-" + phaseID
}

func researchPhaseLeaderTaskID(phaseID string) string {
	return "research-leader-" + phaseID
}

func researchFanoutGroup(pipelineID string) string {
	pipelineID = strings.TrimSpace(pipelineID)
	if pipelineID == "" {
		return "research"
	}
	return "research:" + pipelineID
}

func (app *App) launchResearchPhaseLeaderTask(ctx context.Context, pipeline *research.Pipeline, phase *research.Phase) (string, error) {
	if pipeline == nil || phase == nil {
		return "", errors.New("research pipeline context is required")
	}
	if strings.TrimSpace(pipeline.SessionID) == "" {
		return "", errors.New("research pipeline has no parent session")
	}
	if app.Sessions == nil {
		return "", errors.New("session service is not configured")
	}
	if app.Permissions == nil {
		return "", errors.New("permission service is not configured")
	}
	if app.taskAgentRunner == nil {
		return "", errors.New("agent runner is not configured")
	}

	launchCtx := context.WithValue(ctx, tools.ResearchModeContextKey, true)
	launchCtx = context.WithValue(launchCtx, tools.ResearchToolProfileContextKey, tools.ResearchToolProfileLeader)
	launchCtx = context.WithValue(launchCtx, tools.WorkspaceDirContextKey, config.WorkingDirectory())
	launchCtx = context.WithValue(launchCtx, tools.ResearchWorkDirContextKey, pipeline.WorkDir)
	launchCtx = context.WithValue(launchCtx, tools.ResearchContextContextKey, app.buildResearchRuntimeContext(pipeline))
	launchCtx = context.WithValue(launchCtx, tools.ResearchRootSessionContextKey, pipeline.SessionID)

	agentTools := app.researchLeaderTools()
	profile, agentTools, err := app.resolveResearchLeaderSubtaskProfile(agentTools)
	if err != nil {
		return "", err
	}

	launcher := tools.NewBackgroundSubtaskLauncher(app.Permissions, app.Sessions, app.taskAgentRunner)
	leaderTaskID := researchPhaseLeaderTaskID(phase.ID)
	_, err = launcher.Launch(launchCtx, tools.BackgroundSubtaskSpec{
		TaskID:          leaderTaskID,
		ParentSessionID: pipeline.SessionID,
		Description:     fmt.Sprintf("Research Leader %d: %s", phase.Order, phase.Name),
		Prompt:          buildResearchLeaderPrompt(pipeline, phase),
		AgentType:       "leader",
		AgentName:       config.AgentLeader,
		AgentTools:      agentTools,
		SessionMode:     app.researchSubtaskSessionMode(pipeline.SessionID, profile.SessionMode),
		AllowedTools:    researchToolNames(agentTools),
		VerifyPolicy:    profile.VerifyPolicy,
		ParentTaskID:    researchPhaseTaskID(phase.ID),
		FanoutGroup:     researchFanoutGroup(pipeline.ID),
		ResultMaxChars:  researchLeaderResultMaxChars(profile),
		MaxTurns:        profile.MaxTurns,
		Scheduler:       app.researchSubagentScheduler(),
		Profile:         profile,
	})
	if err != nil {
		if strings.Contains(err.Error(), "task_id already exists") {
			return leaderTaskID, nil
		}
		return "", err
	}
	return leaderTaskID, nil
}

func (app *App) researchSubagentScheduler() *tools.SubagentScheduler {
	if app != nil && app.subagentScheduler != nil {
		return app.subagentScheduler
	}
	return tools.SharedSubagentScheduler()
}

func (app *App) resolveResearchLeaderSubtaskProfile(agentTools []tools.BaseTool) (tools.SubagentProfile, []tools.BaseTool, error) {
	resolver := tools.NewSubagentProfileResolver(config.Get())
	profile, err := resolver.Resolve("leader", nil)
	if err != nil {
		return tools.SubagentProfile{}, nil, err
	}
	agentTools = filterResearchLeaderToolsForProfile(agentTools, profile)
	profile.AgentType = "leader"
	profile.AgentName = config.AgentLeader
	profile.AllowedTools = researchToolNames(agentTools)
	profile.ToolsExplicit = true
	profile.CanSpawnTask = true
	profile.CanWriteFiles = false
	profile.RequireWriteSet = false
	profile.VerifyPolicy = "required"
	return profile, agentTools, nil
}

func filterResearchLeaderToolsForProfile(agentTools []tools.BaseTool, profile tools.SubagentProfile) []tools.BaseTool {
	if len(agentTools) == 0 {
		return nil
	}
	allowed := make(map[string]struct{})
	if profile.ToolsExplicit && !researchAllowsAllToolNames(profile.AllowedTools) {
		for _, name := range profile.AllowedTools {
			name = strings.ToLower(strings.TrimSpace(name))
			if name != "" {
				allowed[name] = struct{}{}
			}
		}
	}
	denied := make(map[string]struct{}, len(profile.DeniedTools))
	for _, name := range profile.DeniedTools {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			denied[name] = struct{}{}
		}
	}
	filtered := make([]tools.BaseTool, 0, len(agentTools))
	for _, tool := range agentTools {
		name := strings.ToLower(tool.Info().Name)
		if _, blocked := denied[name]; blocked {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[name]; !ok {
				continue
			}
		}
		filtered = append(filtered, tool)
	}
	return filtered
}

func researchAllowsAllToolNames(names []string) bool {
	for _, name := range names {
		if strings.TrimSpace(name) == "*" {
			return true
		}
	}
	return false
}

func researchToolNames(agentTools []tools.BaseTool) []string {
	names := make([]string, 0, len(agentTools))
	for _, tool := range agentTools {
		if tool == nil {
			continue
		}
		if name := strings.TrimSpace(tool.Info().Name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (app *App) researchSubtaskSessionMode(parentSessionID string, requested permission.Mode) permission.Mode {
	parentMode := permission.ModeDefault
	if app != nil && app.Permissions != nil && strings.TrimSpace(parentSessionID) != "" {
		parentMode = app.Permissions.SessionMode(parentSessionID)
	}
	if parentMode == permission.ModePlan || requested == permission.ModePlan {
		return permission.ModePlan
	}
	return permission.ModeDefault
}

func researchLeaderResultMaxChars(profile tools.SubagentProfile) int {
	cfg := config.Get()
	if cfg == nil || !cfg.SubagentOrchestration.Enabled {
		return 0
	}
	if profile.ResultMaxChars > 0 {
		return profile.ResultMaxChars
	}
	return cfg.SubagentOrchestration.DefaultResultMaxChars
}

func (app *App) researchLeaderTools() []tools.BaseTool {
	base := tools.ResearchLeaderTools(app.Permissions, app.Sessions, app.Messages, app.taskAgentRunner)
	if app.ResearchEngine == nil {
		return base
	}
	researchCtrl := &researchControllerAdapter{engine: app.ResearchEngine}
	return append(base,
		tools.NewResearchControlTool(researchCtrl, app.CheckpointBroker, app.Permissions, app.taskAgentRunner, app.Sessions, app.Messages),
		tools.NewResearchPipelineTool(researchCtrl, app.CheckpointBroker, app.Permissions, app.taskAgentRunner, app.Sessions, app.Messages),
		tools.NewResearchTaskTool(researchCtrl),
		tools.NewResearchMessageTool(researchCtrl),
	)
}

func (app *App) waitResearchLeaderTask(ctx context.Context, taskID string) error {
	if app.TaskRegistry == nil {
		return errors.New("task registry is not configured")
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			state, ok := app.TaskRegistry.Get(taskID)
			if !ok || state == nil {
				continue
			}
			meta := state.TaskMeta()
			switch meta.Status {
			case task.StatusCompleted:
				return nil
			case task.StatusFailed:
				if sub, ok := state.(*task.SubtaskState); ok && strings.TrimSpace(sub.LastError) != "" {
					return fmt.Errorf("%w: %s", errResearchLeaderFailed, sub.LastError)
				}
				return errResearchLeaderFailed
			case task.StatusCanceled:
				return errResearchLeaderCanceled
			}
		}
	}
}

func (app *App) buildResearchRuntimeContext(pipeline *research.Pipeline) string {
	if pipeline == nil {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Research topic: %s\nTemplate: %s\nWorkdir: %s\nPipelineID: %s\n",
		pipeline.Topic,
		pipeline.Template,
		pipeline.WorkDir,
		pipeline.ID,
	)
	if strings.HasPrefix(pipeline.Template, "aris") {
		fmt.Fprintf(&sb, "ARIS run config: %s/.aris/run_config.json\n", pipeline.WorkDir)
		fmt.Fprintf(&sb, "ARIS output manifest: %s/MANIFEST.md\n", pipeline.WorkDir)
	}
	return sb.String()
}

func buildResearchLeaderPrompt(pipeline *research.Pipeline, phase *research.Phase) string {
	if pipeline != nil && strings.HasPrefix(pipeline.Template, "aris") {
		return buildARISResearchLeaderPrompt(pipeline, phase)
	}
	return fmt.Sprintf(
		"You are the research leader for pipeline %s.\nTopic: %s\nTemplate: %s\nCurrent phase: %d - %s.\n"+
			"Coordinate this phase through Task workers. Do not write files directly; workers must write deliverables inside the research workspace.\n"+
			"If you create TaskV2 workers, continue from injected task notifications or use Task read/list to collect terminal results; do not finish merely saying you are waiting.\n"+
			"Use View/Glob/Grep to inspect outputs, and call ResearchPipeline with action=advance plus a concise summary once deliverables and verification are ready.\n"+
			"ResearchTask/ResearchMessage remain available only for compatibility; the default collaboration path is TaskV2 -> worker deliverable -> verify -> ResearchPipeline.\n",
		pipeline.ID,
		pipeline.Topic,
		pipeline.Template,
		phase.Order,
		phase.Name,
	)
}

func buildARISResearchLeaderPrompt(pipeline *research.Pipeline, phase *research.Phase) string {
	phaseName := ""
	phaseOrder := 0
	if phase != nil {
		phaseName = phase.Name
		phaseOrder = phase.Order
	}
	workDir := pipeline.WorkDir
	deliverables := "none declared"
	if phases, ok := research.Templates[pipeline.Template]; ok && phaseOrder >= 1 && phaseOrder <= len(phases) {
		deliverables = strings.Join(phases[phaseOrder-1].Deliverables, "\n- ")
	}
	return fmt.Sprintf(
		"You are the ARIS research leader for pipeline %s.\nTopic: %s\nTemplate: %s\nCurrent phase: %d - %s.\n"+
			"Coordinate this phase through Task workers. Do not write files directly; workers must write deliverables inside the research workspace.\n"+
			"Workspace contract:\n"+
			"- Workdir: %s\n"+
			"- Run config: %s/.aris/run_config.json\n"+
			"- Output manifest: %s/MANIFEST.md\n"+
			"- Required phase outputs are declared by the template and must be real non-empty files before advance.\n"+
			"- Current phase required outputs:\n- %s\n"+
			"- Every substantial output should be recorded in the manifest and any phase state file required by the phase.\n"+
			"Review contract:\n"+
			"- Reviewer independence is mandatory. Do not ask reviewers to trust leader summaries, handoff prose, or explanations of changes.\n"+
			"- Reviews and gates must be based on objective file paths, rubric, and JSON schema.\n"+
			"- Auto mode may skip human checkpoints, but it must not skip mandatory ARIS audits or claim gates.\n"+
			"ResearchTask/ResearchMessage remain compatibility-only; the default collaboration path is TaskV2 worker deliverables plus independent verification.\n"+
			"If you create TaskV2 workers, continue from injected task notifications or use Task read/list to collect terminal results; do not finish merely saying you are waiting.\n"+
			"Use View/Glob/Grep to inspect outputs. Call ResearchPipeline(action=\"advance\", summary=\"...\") only after worker deliverables, manifest updates, and required gate artifacts are ready.\n",
		pipeline.ID,
		pipeline.Topic,
		pipeline.Template,
		phaseOrder,
		phaseName,
		workDir,
		workDir,
		workDir,
		deliverables,
	)
}
