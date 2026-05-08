package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	taskpkg "github.com/Nahasma/openscholar-public/internal/task"
)

// asyncTask represents a single background tool execution.
type asyncTask struct {
	ID       string
	ToolName string
	Status   string // "running" | "completed" | "failed"
	Result   string
	Duration time.Duration
	cancel   context.CancelFunc
}

// AsyncNotification is sent to the agent loop when a background task finishes.
type AsyncNotification struct {
	TaskID   string
	ToolName string
	Status   string // "completed" | "failed"
	Result   string
	Duration time.Duration
}

// AsyncExecutor manages a pool of background tool executions within a session.
// All goroutines are cancelled when the parent context is cancelled.
type AsyncExecutor struct {
	parentCtx  context.Context
	mu         sync.Mutex // protects tasks map
	tasks      map[string]*asyncTask
	counter    atomic.Uint64
	notifyCh   chan AsyncNotification
	onComplete   func(id string, n AsyncNotification) // optional callback (e.g. Registry status writeback)
	taskRegistry *taskpkg.Registry                        // optional: if set, tasks are registered/updated here
	sessionID    string                                // session owning this executor
}

// NewAsyncExecutor creates an AsyncExecutor bound to parentCtx.
// When parentCtx is cancelled all running tasks are automatically stopped.
func NewAsyncExecutor(parentCtx context.Context) *AsyncExecutor {
	return &AsyncExecutor{
		parentCtx: parentCtx,
		tasks:     make(map[string]*asyncTask),
		notifyCh:  make(chan AsyncNotification, 32),
	}
}

// Submit starts execFn in a new goroutine and returns a taskID.
// execFn receives a child context derived from the parent; cancellation
// of the parent propagates automatically.
func (e *AsyncExecutor) Submit(toolName string, execFn func(ctx context.Context) (string, error)) string {
	n := e.counter.Add(1)
	taskID := fmt.Sprintf("bg-%03d", n)

	taskCtx, cancel := context.WithCancel(e.parentCtx)
	task := &asyncTask{
		ID:       taskID,
		ToolName: toolName,
		Status:   "running",
		cancel:   cancel,
	}

	e.mu.Lock()
	e.tasks[taskID] = task
	e.mu.Unlock()

	// Register in task registry if available
	if e.taskRegistry != nil {
		now := time.Now()
		e.taskRegistry.Register(&taskpkg.BaseState{M: taskpkg.Meta{
			ID:        taskID,
			Label:     toolName,
			SessionID: e.sessionID,
			Kind:      taskpkg.KindToolAsync,
			Status:    taskpkg.StatusRunning,
			StartedAt: now,
		}})
	}

	go func() {
		start := time.Now()
		result, err := execFn(taskCtx)
		duration := time.Since(start)

		e.mu.Lock()
		task.Duration = duration
		if err != nil {
			task.Status = "failed"
			task.Result = err.Error()
		} else {
			task.Status = "completed"
			task.Result = result
		}
		e.mu.Unlock()

		// Update task registry with final status
		if e.taskRegistry != nil {
			e.taskRegistry.Update(taskID, func(m *taskpkg.Meta) {
				now := time.Now()
				m.EndedAt = &now
				if err != nil {
					m.Status = taskpkg.StatusFailed
				} else {
					m.Status = taskpkg.StatusCompleted
				}
			})
		}

		notification := AsyncNotification{
			TaskID:   task.ID,
			ToolName: task.ToolName,
			Status:   task.Status,
			Result:   task.Result,
			Duration: duration,
		}

		// Invoke onComplete callback if set (e.g. Registry status writeback)
		if e.onComplete != nil {
			e.onComplete(taskID, notification)
		}

		// Non-blocking send: drop notification if channel is full to avoid goroutine leak.
		select {
		case e.notifyCh <- notification:
		default:
		}
	}()

	return taskID
}

// SetOnComplete sets a callback invoked when any task completes.
func (e *AsyncExecutor) SetOnComplete(fn func(id string, n AsyncNotification)) {
	e.onComplete = fn
}

// SetTaskRegistry configures the executor to register/update tasks in the given registry.
func (e *AsyncExecutor) SetTaskRegistry(reg *taskpkg.Registry, sessionID string) {
	e.taskRegistry = reg
	e.sessionID = sessionID
}

// DrainNotifications returns all pending completion notifications without blocking.
func (e *AsyncExecutor) DrainNotifications() []AsyncNotification {
	var out []AsyncNotification
	for {
		select {
		case n := <-e.notifyCh:
			out = append(out, n)
		default:
			return out
		}
	}
}

// CheckTask returns the task for taskID, or nil if not found.
func (e *AsyncExecutor) CheckTask(taskID string) *asyncTask {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tasks[taskID]
}

// Shutdown cancels all running tasks and drains the notification channel.
func (e *AsyncExecutor) Shutdown() {
	e.mu.Lock()
	for _, task := range e.tasks {
		if task.Status == "running" && task.cancel != nil {
			task.cancel()
		}
	}
	e.mu.Unlock()
	// Drain remaining notifications
	for {
		select {
		case <-e.notifyCh:
		default:
			return
		}
	}
}
