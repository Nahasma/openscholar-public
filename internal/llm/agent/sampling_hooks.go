package agent

// CacheSafeSnapshot is defined in forked_types.go (same package).

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/openscholar/openscholar/internal/message"
)

// SamplingSource distinguishes where a sampling cycle originated, preventing recursive
// hook invocations when a hook itself triggers an agent run.
type SamplingSource string

const (
	// SamplingSourceMain indicates the sampling was initiated by the primary interactive agent.
	SamplingSourceMain SamplingSource = "main"

	// SamplingSourceForked indicates the sampling was initiated by a forked sub-agent.
	// Hooks are skipped for this source to prevent infinite recursion.
	SamplingSourceForked SamplingSource = "forked"
)

// PostSamplingContext carries all relevant information about a completed sampling turn
// to registered PostSamplingHook implementations.
type PostSamplingContext struct {
	// SessionID is the owning session identifier.
	SessionID string

	// Source identifies whether this sampling turn came from the main agent or a fork.
	Source SamplingSource

	// AssistantMsg is the final assistant message produced in this turn.
	AssistantMsg message.Message

	// History is a snapshot of the message history at the time sampling completed.
	History []message.Message

	// ToolCallCount is the number of tool calls executed during this turn.
	ToolCallCount int

	// Snapshot is an optional cache-safe snapshot of the agent state, used by hooks
	// that need to fork a sub-agent. May be nil if snapshotting is not configured.
	// CacheSafeSnapshot is defined in forked_types.go.
	Snapshot *CacheSafeSnapshot

	// StartedAt is the wall-clock time when the agent turn began.
	StartedAt time.Time

	// FinishedAt is the wall-clock time when the agent turn completed.
	FinishedAt time.Time
}

// PostSamplingHook is executed asynchronously after each completed sampling turn.
// Implementations must be safe to call concurrently from multiple goroutines.
type PostSamplingHook interface {
	// Name returns a stable, human-readable identifier used in log messages.
	Name() string

	// Run executes the hook logic. The context carries a 30-second deadline.
	// Returning a non-nil error causes the error to be logged; it does not affect
	// other hooks or the calling agent.
	Run(ctx context.Context, ps PostSamplingContext) error
}

// PostSamplingRegistry manages the set of registered PostSamplingHook instances
// and handles their asynchronous execution.
type PostSamplingRegistry interface {
	// Register adds a hook to the registry. Hooks are executed in registration order.
	Register(h PostSamplingHook)

	// Clear removes all registered hooks.
	Clear()

	// ExecuteAsync launches each registered hook in its own goroutine.
	// Hooks are skipped when ps.Source == SamplingSourceForked to prevent recursion.
	// Each hook receives a child context with a 30-second timeout.
	// Panics inside hooks are recovered and logged; they do not propagate.
	ExecuteAsync(ctx context.Context, ps PostSamplingContext)
}

const hookTimeout = 30 * time.Second

// postSamplingRegistry is the concrete implementation of PostSamplingRegistry.
type postSamplingRegistry struct {
	mu     sync.RWMutex
	hooks  []PostSamplingHook
	logger *slog.Logger
}

// NewPostSamplingRegistry creates a new PostSamplingRegistry that logs via logger.
// If logger is nil, slog.Default() is used.
func NewPostSamplingRegistry(logger *slog.Logger) PostSamplingRegistry {
	if logger == nil {
		logger = slog.Default()
	}
	return &postSamplingRegistry{
		logger: logger,
	}
}

// Register appends h to the registry. Safe for concurrent use.
func (r *postSamplingRegistry) Register(h PostSamplingHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = append(r.hooks, h)
}

// Clear removes all hooks. Safe for concurrent use.
func (r *postSamplingRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = nil
}

// ExecuteAsync launches one goroutine per registered hook.
// If ps.Source is SamplingSourceForked, all hooks are skipped.
// Fix 1: uses context.WithoutCancel so hooks survive after the parent request returns.
func (r *postSamplingRegistry) ExecuteAsync(ctx context.Context, ps PostSamplingContext) {
	if ps.Source == SamplingSourceForked {
		r.logger.Debug("post-sampling hooks skipped: forked source",
			"session_id", ps.SessionID)
		return
	}

	// Detach from parent cancel signal — hooks must outlive the request.
	detachedCtx := context.WithoutCancel(ctx)

	r.mu.RLock()
	snapshot := make([]PostSamplingHook, len(r.hooks))
	copy(snapshot, r.hooks)
	r.mu.RUnlock()

	for _, h := range snapshot {
		h := h // capture loop variable
		go r.runHook(detachedCtx, h, ps)
	}
}

// runHook executes a single hook with timeout and panic recovery.
func (r *postSamplingRegistry) runHook(parent context.Context, h PostSamplingHook, ps PostSamplingContext) {
	hookCtx, cancel := context.WithTimeout(parent, hookTimeout)
	defer cancel()

	name := h.Name()
	r.logger.Debug("post-sampling hook starting",
		"hook", name,
		"session_id", ps.SessionID,
		"source", ps.Source)

	var runErr error

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				runErr = fmt.Errorf("panic: %v", rec)
				r.logger.Error("post-sampling hook panicked",
					"hook", name,
					"session_id", ps.SessionID,
					"panic", rec)
			}
		}()
		runErr = h.Run(hookCtx, ps)
	}()

	if runErr != nil {
		if hookCtx.Err() != nil {
			r.logger.Warn("post-sampling hook timed out",
				"hook", name,
				"session_id", ps.SessionID,
				"timeout", hookTimeout)
		} else {
			r.logger.Error("post-sampling hook failed",
				"hook", name,
				"session_id", ps.SessionID,
				"error", runErr)
		}
		return
	}

	r.logger.Debug("post-sampling hook completed",
		"hook", name,
		"session_id", ps.SessionID)
}
