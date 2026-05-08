package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Nahasma/openscholar-public/internal/config"
	"github.com/Nahasma/openscholar-public/internal/hooks"
	"github.com/Nahasma/openscholar-public/internal/permission"
	"github.com/Nahasma/openscholar-public/internal/session"
	"github.com/Nahasma/openscholar-public/internal/task"
)

var (
	taskRuntimeMu    sync.RWMutex
	taskRegistryRef  *task.Registry
	hookServiceRef   hooks.Service
	taskSchedulerRef *SubagentScheduler
	taskHandles      = make(map[string]*taskRuntime)
)

const defaultSubagentNotificationResumeLimit = 4

// SetTaskRuntime configures optional shared dependencies for Task tool instances.
func SetTaskRuntime(reg *task.Registry, hookSvc hooks.Service) {
	taskRuntimeMu.Lock()
	defer taskRuntimeMu.Unlock()
	taskRegistryRef = reg
	hookServiceRef = hookSvc
	if reg == nil && hookSvc == nil {
		taskSchedulerRef = nil
	} else {
		taskSchedulerRef = newSubagentScheduler(config.Get())
	}
	if reg == nil && hookSvc == nil {
		taskHandles = make(map[string]*taskRuntime)
	}
}

func getTaskRuntime() (*task.Registry, hooks.Service) {
	taskRuntimeMu.RLock()
	defer taskRuntimeMu.RUnlock()
	return taskRegistryRef, hookServiceRef
}

// SharedSubagentScheduler returns the process-local scheduler shared by Task
// tool instances that use the shared runtime.
func SharedSubagentScheduler() *SubagentScheduler {
	taskRuntimeMu.Lock()
	defer taskRuntimeMu.Unlock()
	if taskSchedulerRef == nil {
		taskSchedulerRef = newSubagentScheduler(config.Get())
	}
	return taskSchedulerRef
}

func beginTaskRuntime(taskID string, rt *taskRuntime) bool {
	taskRuntimeMu.Lock()
	defer taskRuntimeMu.Unlock()
	if _, exists := taskHandles[taskID]; exists {
		return false
	}
	taskHandles[taskID] = rt
	return true
}

func finishTaskRuntime(taskID string) {
	taskRuntimeMu.Lock()
	defer taskRuntimeMu.Unlock()
	delete(taskHandles, taskID)
}

func isTaskRuntimeRunning(taskID string) bool {
	taskRuntimeMu.RLock()
	defer taskRuntimeMu.RUnlock()
	_, exists := taskHandles[taskID]
	return exists
}

func cancelTaskRuntime(taskID string) bool {
	taskRuntimeMu.Lock()
	rt, exists := taskHandles[taskID]
	if !exists || rt.cancel == nil {
		taskRuntimeMu.Unlock()
		return false
	}
	cancel := rt.cancel
	rt.cancel = nil
	taskRuntimeMu.Unlock()

	cancel()
	return true
}

// BackgroundSubtaskLauncher starts tracked background sub-agents with a child
// task session and task registry state.
type BackgroundSubtaskLauncher struct {
	permissions permission.Service
	sessions    session.Service
	runAgent    AgentRunner
	registry    *task.Registry
	hookService hooks.Service
}

// BackgroundSubtaskSpec configures a background subtask launch.
type BackgroundSubtaskSpec struct {
	TaskID          string
	ParentSessionID string
	Description     string
	Prompt          string
	AgentType       string
	AgentName       config.AgentName
	AgentTools      []BaseTool
	SessionMode     permission.Mode
	Model           string
	AllowedTools    []string
	VerifyPolicy    string
	Timeout         time.Duration
	ParentTaskID    string
	FanoutGroup     string
	WriteSet        []string
	ResultMaxChars  int
	MaxTurns        int
	Scheduler       *SubagentScheduler
	Profile         SubagentProfile
}

// NewBackgroundSubtaskLauncher creates a launcher using the shared Task runtime.
func NewBackgroundSubtaskLauncher(perms permission.Service, sessions session.Service, runAgent AgentRunner) *BackgroundSubtaskLauncher {
	reg, hookSvc := getTaskRuntime()
	return &BackgroundSubtaskLauncher{
		permissions: perms,
		sessions:    sessions,
		runAgent:    runAgent,
		registry:    reg,
		hookService: hookSvc,
	}
}

// Launch starts a background subtask and registers state in task runtime.
func (l *BackgroundSubtaskLauncher) Launch(ctx context.Context, spec BackgroundSubtaskSpec) (*task.SubtaskState, error) {
	if l.registry == nil {
		return nil, fmt.Errorf("task runtime is unavailable")
	}
	if l.sessions == nil {
		return nil, fmt.Errorf("session service is unavailable")
	}
	if l.permissions == nil {
		return nil, fmt.Errorf("permission service is unavailable")
	}
	if l.runAgent == nil {
		return nil, fmt.Errorf("agent runner is unavailable")
	}

	taskID := strings.TrimSpace(spec.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if _, exists := l.registry.Get(taskID); exists {
		return nil, fmt.Errorf("task_id already exists")
	}

	parentSessionID := strings.TrimSpace(spec.ParentSessionID)
	if parentSessionID == "" {
		return nil, fmt.Errorf("parent session is required")
	}

	prompt := strings.TrimSpace(spec.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	agentType := strings.TrimSpace(spec.AgentType)
	if agentType == "" {
		agentType = "general"
	}
	verifyPolicy, err := normalizeTaskVerifyPolicy(spec.VerifyPolicy, agentType)
	if err != nil {
		return nil, err
	}
	writeSet := normalizeWriteSet(spec.WriteSet)
	if spec.Scheduler != nil && spec.Scheduler.enabled && (spec.Profile.CanWriteFiles || spec.Profile.RequireWriteSet) && len(writeSet) == 0 {
		return nil, fmt.Errorf("write_set is required for writable worker profile %q when subagent orchestration is enabled", agentType)
	}
	label := strings.TrimSpace(spec.Description)
	if label == "" {
		label = "Task " + taskID
	}

	childSession, err := l.sessions.CreateTaskSession(ctx, taskID, parentSessionID, label)
	if err != nil {
		return nil, fmt.Errorf("failed to create task session: %w", err)
	}

	startedAt := time.Now()
	state := task.NewSubtaskState(task.Meta{
		ID:             taskID,
		Label:          label,
		SessionID:      parentSessionID,
		Kind:           task.KindAgent,
		Status:         task.StatusRunning,
		StartedAt:      startedAt,
		IsBackgrounded: true,
		Notified:       false,
		ParentTaskID:   strings.TrimSpace(spec.ParentTaskID),
	}, parentSessionID, childSession.ID, agentType, spec.Description, spec.Model, verifyPolicy)
	state.FanoutGroup = strings.TrimSpace(spec.FanoutGroup)
	state.WriteSet = append([]string(nil), writeSet...)
	state.ResultMaxChars = spec.ResultMaxChars
	state.MaxTurns = spec.MaxTurns

	baseCtx := detachedTaskContext(ctx)
	runCtx, cancel := context.WithCancel(baseCtx)
	if spec.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(baseCtx, spec.Timeout)
	}
	if IsResearchMode(runCtx) {
		runCtx = context.WithValue(runCtx, ResearchToolProfileContextKey, researchToolProfileForAgentType(agentType))
	}
	if strings.TrimSpace(spec.Model) != "" || len(spec.AllowedTools) > 0 {
		runCtx = WithTaskRuntimeOverride(runCtx, TaskRuntimeOverride{
			Model:        spec.Model,
			AllowedTools: spec.AllowedTools,
		})
	}
	hookCtx := context.WithoutCancel(runCtx)
	if !beginTaskRuntime(taskID, &taskRuntime{cancel: cancel}) {
		cancel()
		return nil, fmt.Errorf("task is already running")
	}

	l.permissions.SetSessionMode(childSession.ID, spec.SessionMode)
	l.permissions.RequirePromptSession(childSession.ID)
	l.registry.Register(state)
	_ = l.emitHook(hookCtx, hooks.TaskCreated, parentSessionID)
	snapshot, _ := task.CloneState(state).(*task.SubtaskState)

	_ = l.emitHook(hookCtx, hooks.SubagentStart, parentSessionID)

	go func(taskID string, childSessionID string) {
		lease, acquireErr := acquireSubagentLease(runCtx, spec.Scheduler, parentSessionID, spec.Profile, state.WriteSet)
		if acquireErr != nil {
			now := time.Now()
			l.permissions.RemoveRequirePromptSession(childSessionID)
			l.registry.Mutate(taskID, func(s task.State) {
				sub, ok := s.(*task.SubtaskState)
				if !ok {
					return
				}
				sub.LastError = acquireErr.Error()
				sub.TaskMeta().Status = task.StatusCanceled
				sub.TaskMeta().EndedAt = &now
				sub.TaskMeta().Notified = false
				sub.TaskMeta().NotifyClaimed = false
				if taskNeedsVerification(sub.VerifyPolicy) {
					sub.VerifyStatus = task.VerifyStatusFailed
					sub.VerifyVerdict = ""
					sub.VerifyError = "task execution failed before verification: " + acquireErr.Error()
				}
			})
			finishTaskRuntime(taskID)
			_ = l.emitHook(hookCtx, hooks.SubagentStop, parentSessionID)
			_ = l.emitHook(hookCtx, hooks.TaskCompleted, parentSessionID)
			return
		}
		defer lease.Release()

		result, err := l.runAgentWithNotificationResumes(runCtx, spec, childSessionID)
		l.permissions.RemoveRequirePromptSession(childSessionID)

		fullResult := strings.TrimSpace(result)
		cappedResult, resultTruncated := applyResultCap(fullResult, state.ResultMaxChars)
		verifyStatus := taskVerifyStatusForPolicy(verifyPolicy)
		verifyVerdict := ""
		verifyResult := ""
		verifyError := ""
		finalStatus := task.StatusCompleted

		now := time.Now()
		if err != nil {
			finalStatus = task.StatusFailed
			if runCtx.Err() == context.Canceled {
				finalStatus = task.StatusCanceled
			}
			if taskNeedsVerification(verifyPolicy) {
				verifyStatus = task.VerifyStatusFailed
				verifyError = "task execution failed before verification"
			}
		} else if taskNeedsVerification(verifyPolicy) {
			l.registry.Mutate(taskID, func(s task.State) {
				sub, ok := s.(*task.SubtaskState)
				if !ok {
					return
				}
				sub.Result = cappedResult
				sub.ResultTruncated = resultTruncated
				sub.VerifyStatus = task.VerifyStatusRunning
				sub.VerifyVerdict = ""
				sub.VerifyError = ""
				sub.VerifyResult = ""
				sub.TaskMeta().Notified = false
				sub.TaskMeta().NotifyClaimed = false
			})
			verifyStatus, verifyVerdict, verifyResult, verifyError = runTaskVerification(runCtx, l.permissions, l.sessions, l.runAgent, state, spec.Prompt, fullResult)
			if verifyStatus != task.VerifyStatusPassed {
				finalStatus = task.StatusFailed
				if runCtx.Err() == context.Canceled {
					finalStatus = task.StatusCanceled
				}
			}
		}

		l.registry.Mutate(taskID, func(s task.State) {
			sub, ok := s.(*task.SubtaskState)
			if !ok {
				return
			}
			sub.Result = cappedResult
			sub.ResultTruncated = resultTruncated
			sub.VerifyStatus = verifyStatus
			sub.VerifyVerdict = verifyVerdict
			sub.VerifyResult = verifyResult
			sub.VerifyError = verifyError
			sub.TaskMeta().EndedAt = &now
			sub.TaskMeta().Notified = false
			sub.TaskMeta().NotifyClaimed = false
			if err != nil {
				sub.LastError = err.Error()
				sub.TaskMeta().Status = finalStatus
				return
			}
			if verifyError != "" {
				sub.LastError = verifyError
				sub.TaskMeta().Status = finalStatus
				return
			}
			sub.LastError = ""
			sub.TaskMeta().Status = finalStatus
		})

		finishTaskRuntime(taskID)
		_ = l.emitHook(hookCtx, hooks.SubagentStop, parentSessionID)
		_ = l.emitHook(hookCtx, hooks.TaskCompleted, parentSessionID)
	}(state.TaskMeta().ID, state.ChildSessionID)

	if snapshot == nil {
		return state, nil
	}
	return snapshot, nil
}

func (l *BackgroundSubtaskLauncher) runAgentWithNotificationResumes(ctx context.Context, spec BackgroundSubtaskSpec, childSessionID string) (string, error) {
	result, err := l.runAgent(ctx, spec.AgentName, childSessionID, spec.Prompt, spec.AgentTools)
	if err != nil || !spec.Profile.CanSpawnTask || !subagentOrchestrationEnabled() {
		return result, err
	}
	limit := subagentNotificationResumeLimit(spec.MaxTurns)
	for i := 0; i < limit; i++ {
		ready, waitErr := l.waitForChildTaskNotifications(ctx, childSessionID)
		if waitErr != nil {
			return result, waitErr
		}
		if !ready {
			return result, nil
		}
		next, runErr := l.runAgent(ctx, spec.AgentName, childSessionID, "Continue after completed child task notifications. Use the injected <task-notification> messages to synthesize child results, continue delegated work if needed, or finish the original task.", spec.AgentTools)
		if strings.TrimSpace(next) != "" {
			result = next
		}
		if runErr != nil {
			return result, runErr
		}
	}
	if ready, _ := l.childTaskNotificationState(childSessionID); ready {
		return result, fmt.Errorf("subagent notification resume limit reached for session %s", childSessionID)
	}
	return result, nil
}

func subagentOrchestrationEnabled() bool {
	cfg := config.Get()
	return cfg != nil && cfg.SubagentOrchestration.Enabled
}

func subagentNotificationResumeLimit(maxTurns int) int {
	if maxTurns > 0 && maxTurns < defaultSubagentNotificationResumeLimit {
		return maxTurns
	}
	return defaultSubagentNotificationResumeLimit
}

func (l *BackgroundSubtaskLauncher) waitForChildTaskNotifications(ctx context.Context, parentSessionID string) (bool, error) {
	return waitForChildTaskNotificationsByRegistry(ctx, l.registry, parentSessionID, l.childTaskNotificationState)
}

func waitForChildTaskNotificationsByRegistry(ctx context.Context, reg *task.Registry, parentSessionID string, state func(string) (ready bool, running bool)) (bool, error) {
	if state == nil {
		return false, nil
	}
	ready, running := state(parentSessionID)
	if ready && !running {
		return true, nil
	}
	if !ready && !running {
		return false, nil
	}
	if reg == nil {
		<-ctx.Done()
		return false, ctx.Err()
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	events := reg.Subscribe(subCtx)

	// Re-check after subscribing so a terminal transition between the first
	// snapshot and Subscribe does not leave this waiter parked forever.
	ready, running = state(parentSessionID)
	if ready && !running {
		return true, nil
	}
	if !ready && !running {
		return false, nil
	}

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case _, ok := <-events:
			if !ok {
				if err := ctx.Err(); err != nil {
					return false, err
				}
				return false, nil
			}
			ready, running := state(parentSessionID)
			if ready && !running {
				return true, nil
			}
			if !ready && !running {
				return false, nil
			}
		}
	}
}

func (l *BackgroundSubtaskLauncher) childTaskNotificationState(parentSessionID string) (ready bool, running bool) {
	if l == nil || l.registry == nil {
		return false, false
	}
	for _, st := range l.registry.All() {
		sub, ok := st.(*task.SubtaskState)
		if !ok || sub.ParentSessionID != parentSessionID {
			continue
		}
		switch sub.TaskMeta().Status {
		case task.StatusPending, task.StatusRunning:
			running = true
		case task.StatusCompleted, task.StatusFailed, task.StatusCanceled:
			if !sub.TaskMeta().Notified && !sub.TaskMeta().NotifyClaimed {
				ready = true
			}
		}
	}
	return ready, running
}

func (l *BackgroundSubtaskLauncher) emitHook(ctx context.Context, event hooks.Event, sessionID string) error {
	if l.hookService == nil {
		return nil
	}
	return l.hookService.Run(ctx, event, hooks.Input{
		SessionID: sessionID,
		Timestamp: time.Now(),
	})
}
