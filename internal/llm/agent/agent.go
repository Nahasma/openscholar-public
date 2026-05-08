package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"log/slog"

	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/fileop"
	"github.com/openscholar/openscholar/internal/hooks"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/provider"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/memory"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/plan"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/task"
)

var (
	ErrRequestCancelled        = errors.New("request cancelled by user")
	ErrSessionBusy             = errors.New("session is currently processing another request")
	ErrModelInvocationDisabled = errors.New("model invocation disabled by runtime override")
)

type debugLoggerKeyType struct{}
type llmStartTimeKeyType struct{}

var debugLoggerCtxKey = debugLoggerKeyType{}
var llmStartTimeCtxKey = llmStartTimeKeyType{}

type Service interface {
	pubsub.Subscriber[AgentEvent]
	pubsub.Publisher[AgentEvent]
	Model() models.Model
	SetModel(model models.Model) error
	ReloadProvider() error
	Run(ctx context.Context, sessionID string, content string, extraParts ...message.ContentPart) (<-chan AgentEvent, error)
	Cancel(sessionID string)
	IsSessionBusy(sessionID string) bool
	IsBusy() bool
	SetHookService(svc hooks.Service)
	SetTaskRegistry(reg *task.Registry)
	SetPostSamplingRegistry(reg PostSamplingRegistry)
	SetCheckpointStore(cs CheckpointStore)
	SetForkedRunner(fr ForkedRunner)
	SetSessionMemoryLoader(fn func(ctx context.Context, sessionID string) (string, error))
	SetFileReadNotifier(fn func(sessionID, filePath, content string))
	SetFileToolUsageNotifier(fn func(sessionID string, evt tools.FileToolUsageEvent))
	SetPermissionService(svc permission.Service)
	SetPlanService(svc plan.Service)
	CostState() SessionCostState
	LastInputTokens() int64
	CompactSession(ctx context.Context, sessionID string, focus string) error
	ContextSnapshot() ContextSnapshot
	AnalyzeContext(ctx context.Context, sessionID string) (ContextReport, error)
	ResetTelemetry()
}

type ContextSnapshot struct {
	CurrentUsage              message.Usage
	LastResponseContextTokens int64
	LastResponseOutputTokens  int64
}

type agent struct {
	*pubsub.Broker[AgentEvent]
	agentName config.AgentName
	sessions  session.Service
	messages  message.Service

	tools              []tools.BaseTool        // used by non-Coder agents (static tool list)
	registry           *tools.DeferredRegistry // used by Coder agent (deferred loading)
	agentProvider      provider.Provider
	summarizerProvider provider.Provider

	memoryService  memory.Service
	hookService    hooks.Service  // nil-safe: callers check before use
	taskRegistry   *task.Registry // nil-safe: callers check before use
	asyncExecutor  *AsyncExecutor
	activeRequests sync.Map

	// A2: precise token tracking
	lastInputTokens atomic.Int64 // most recent API-returned input token count

	// C2: per-model cost tracking
	costTracker *costState

	// Loop Governance: stop controller with pluggable policies
	stopController *StopController

	// Loop Governance: budget controller for end_turn continuation gate
	budgetController *BudgetController

	// Loop Governance: budget exceeded hook for per-turn cost/token check
	budgetExceededHook *BudgetExceededHook

	// Loop Governance: most recent API usage (set by trackUsage, read by processGeneration)
	lastUsageData lastUsage
	contextState  atomic.Value // stores ContextSnapshot

	// PC3: auto-compact circuit breaker
	compactFailures int // consecutive compact failure count

	// Phase 5: post-sampling hooks, file checkpointing, forked runner
	postSamplingRegistry PostSamplingRegistry
	checkpointStore      CheckpointStore
	forkedRunner         ForkedRunner

	// Phase 5: session memory loader (injected from app layer, nil-safe)
	sessionMemoryLoader func(ctx context.Context, sessionID string) (string, error)
	// Phase 5: MagicDoc file read notifier (injected from app layer, nil-safe)
	fileReadNotifier func(sessionID, filePath, content string)
	// SkillBank: file tool usage notifier for paths-based activation.
	fileToolUsageNotifier func(sessionID string, evt tools.FileToolUsageEvent)
	readStateManager      *fileop.SessionReadStateManager
	permissionService     permission.Service
	planService           plan.Service
}

func NewAgent(
	agentName config.AgentName,
	sessions session.Service,
	messages message.Service,
	agentTools []tools.BaseTool,
) (Service, error) {
	agentProvider, err := createAgentProvider(agentName)
	if err != nil {
		return nil, err
	}

	var summarizerProvider provider.Provider
	if agentName == config.AgentCoder {
		summarizerProvider, err = createAgentProvider(config.AgentSummarizer)
		if err != nil {
			// Non-fatal: summarization is optional
			summarizerProvider = nil
		}
	}

	sc := NewStopController(NewMaxIterationHook(defaultMaxIterationHook))
	sc.AddPolicy(NewRepeatedToolPatternHook())
	sc.AddPolicy(NewProviderCooldownLoopHook(2))
	sc.AddPolicy(NewNoProgressHook(6))

	return &agent{
		Broker:             pubsub.NewBroker[AgentEvent](),
		agentName:          agentName,
		agentProvider:      agentProvider,
		summarizerProvider: summarizerProvider,
		messages:           messages,
		sessions:           sessions,
		tools:              agentTools,
		activeRequests:     sync.Map{},
		costTracker:        newCostState(),
		stopController:     sc,
		readStateManager:   fileop.NewSessionReadStateManager(),
	}, nil
}

// NewAgentForTest creates an agent with a pre-configured provider, bypassing config/HTTP setup.
// Use this in tests to inject a MockProvider.
func NewAgentForTest(
	p provider.Provider,
	sessions session.Service,
	messages message.Service,
	agentTools []tools.BaseTool,
) Service {
	sc := NewStopController(NewMaxIterationHook(defaultMaxIterationHook))
	sc.AddPolicy(NewRepeatedToolPatternHook())
	sc.AddPolicy(NewProviderCooldownLoopHook(2))
	sc.AddPolicy(NewNoProgressHook(6))

	return &agent{
		Broker:           pubsub.NewBroker[AgentEvent](),
		agentName:        config.AgentCoder, // default for tests
		agentProvider:    p,
		messages:         messages,
		sessions:         sessions,
		tools:            agentTools,
		activeRequests:   sync.Map{},
		costTracker:      newCostState(),
		stopController:   sc,
		readStateManager: fileop.NewSessionReadStateManager(),
	}
}

// NewAgentWithRegistry creates an agent that uses a DeferredRegistry for on-demand tool loading.
// Core tools are always sent to LLM; extended tools are activated via ToolSearch.
func NewAgentWithRegistry(
	agentName config.AgentName,
	sessions session.Service,
	messages message.Service,
	registry *tools.DeferredRegistry,
	memService memory.Service,
) (Service, error) {
	agentProvider, err := createAgentProvider(agentName)
	if err != nil {
		return nil, err
	}

	var summarizerProvider provider.Provider
	if agentName == config.AgentCoder {
		summarizerProvider, _ = createAgentProvider(config.AgentSummarizer)
	}

	sc := NewStopController(NewMaxIterationHook(defaultMaxIterationHook))
	sc.AddPolicy(NewRepeatedToolPatternHook())
	sc.AddPolicy(NewProviderCooldownLoopHook(2))
	sc.AddPolicy(NewNoProgressHook(6))

	return &agent{
		Broker:             pubsub.NewBroker[AgentEvent](),
		agentName:          agentName,
		agentProvider:      agentProvider,
		summarizerProvider: summarizerProvider,
		messages:           messages,
		sessions:           sessions,
		registry:           registry,
		memoryService:      memService,
		activeRequests:     sync.Map{},
		costTracker:        newCostState(),
		stopController:     sc,
		readStateManager:   fileop.NewSessionReadStateManager(),
	}, nil
}

func (a *agent) Model() models.Model {
	return a.agentProvider.Model()
}

// CostState returns a snapshot of per-model cost tracking for this agent.
// Implements CostStateReader.
func (a *agent) CostState() SessionCostState {
	if a.costTracker == nil {
		return SessionCostState{ByModel: make(map[string]*ModelUsage), CostKnown: true}
	}
	return a.costTracker.Snapshot()
}

// LastInputTokens returns the most recent API-returned input token count
// (input + cacheRead + cacheCreation). Used by TUI for context usage display.
func (a *agent) LastInputTokens() int64 {
	return a.lastInputTokens.Load()
}

func (a *agent) ContextSnapshot() ContextSnapshot {
	if v := a.contextState.Load(); v != nil {
		if snap, ok := v.(ContextSnapshot); ok {
			return snap
		}
	}
	return ContextSnapshot{}
}

func (a *agent) ResetTelemetry() {
	a.lastInputTokens.Store(0)
	a.lastUsageData = lastUsage{}
	a.contextState.Store(ContextSnapshot{})
	a.costTracker = newCostState()
}

func (a *agent) CompactSession(ctx context.Context, sessionID string, focus string) error {
	if sessionID == "" {
		return errors.New("no active session")
	}
	if a.IsSessionBusy(sessionID) {
		return ErrSessionBusy
	}

	msgs, err := a.messages.List(ctx, sessionID)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}

	sess, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	msgs = historyAfterSummaryBoundary(msgs, sess.SummaryMessageID)

	a.Publish(pubsub.CreatedEvent, AgentEvent{
		Type:      AgentEventTypeCompacting,
		SessionID: sessionID,
	})
	if err := a.compact(ctx, sessionID, msgs, focus); err != nil {
		a.Publish(pubsub.CreatedEvent, AgentEvent{
			Type:      AgentEventTypeError,
			SessionID: sessionID,
			Error:     err,
			Done:      true,
		})
		return err
	}
	a.Publish(pubsub.CreatedEvent, AgentEvent{
		Type:      AgentEventTypeCompactDone,
		SessionID: sessionID,
	})
	return nil
}

// SetHookService injects the lifecycle hook service. Safe to call with nil.
func (a *agent) SetHookService(svc hooks.Service) {
	a.hookService = svc
}

// SetTaskRegistry injects the unified task registry. Safe to call with nil.
func (a *agent) SetTaskRegistry(reg *task.Registry) {
	a.taskRegistry = reg
}

// SetPostSamplingRegistry injects the post-sampling hook registry. Safe to call with nil.
func (a *agent) SetPostSamplingRegistry(reg PostSamplingRegistry) {
	a.postSamplingRegistry = reg
}

// SetCheckpointStore injects the file checkpoint store. Safe to call with nil.
func (a *agent) SetCheckpointStore(cs CheckpointStore) {
	a.checkpointStore = cs
}

// SetForkedRunner injects the forked runner. Safe to call with nil.
func (a *agent) SetForkedRunner(fr ForkedRunner) {
	a.forkedRunner = fr
}

// SetSessionMemoryLoader injects a callback to load session notes for prompt injection.
func (a *agent) SetSessionMemoryLoader(fn func(ctx context.Context, sessionID string) (string, error)) {
	a.sessionMemoryLoader = fn
}

// SetFileReadNotifier injects a callback to notify MagicDoc of file reads.
func (a *agent) SetFileReadNotifier(fn func(sessionID, filePath, content string)) {
	a.fileReadNotifier = fn
}

// SetFileToolUsageNotifier injects a callback to notify SkillBank paths activator.
func (a *agent) SetFileToolUsageNotifier(fn func(sessionID string, evt tools.FileToolUsageEvent)) {
	a.fileToolUsageNotifier = fn
}

func (a *agent) SetPermissionService(svc permission.Service) {
	a.permissionService = svc
}

func (a *agent) SetPlanService(svc plan.Service) {
	a.planService = svc
}

// CreateForkedRunnerFactory creates a providerFactory for ForkedRunner that reuses
// the same config as the main agent but creates fresh provider instances.
func CreateForkedRunnerFactory(agentName config.AgentName) func() (provider.Provider, error) {
	return func() (provider.Provider, error) {
		return createAgentProvider(agentName)
	}
}

// NewForkedRunnerForAgent creates a ForkedRunner configured for the given agent.
func NewForkedRunnerForAgent(agentName config.AgentName, logger *slog.Logger) (ForkedRunner, error) {
	factory := CreateForkedRunnerFactory(agentName)
	return NewForkedRunner(factory, logger), nil
}

func (a *agent) Cancel(sessionID string) {
	if cancelFunc, exists := a.activeRequests.LoadAndDelete(sessionID); exists {
		if cancel, ok := cancelFunc.(context.CancelFunc); ok {
			cancel()
		}
	}
}

func (a *agent) IsBusy() bool {
	busy := false
	a.activeRequests.Range(func(key, value interface{}) bool {
		busy = true
		return false
	})
	return busy
}

func (a *agent) IsSessionBusy(sessionID string) bool {
	_, busy := a.activeRequests.Load(sessionID)
	return busy
}

func (a *agent) Run(ctx context.Context, sessionID string, content string, extraParts ...message.ContentPart) (<-chan AgentEvent, error) {
	events := make(chan AgentEvent, 1)
	genCtx, cancel := context.WithCancel(ctx)
	runtimeCtx, err := a.prepareRequestRuntime(genCtx)
	if err != nil {
		cancel()
		return nil, err
	}
	if _, loaded := a.activeRequests.LoadOrStore(sessionID, cancel); loaded {
		cancel()
		return nil, ErrSessionBusy
	}

	go func() {
		result := a.processGeneration(runtimeCtx, sessionID, content, extraParts...)
		result.SessionID = sessionID // Ensure all events carry session ID for TUI filtering
		a.activeRequests.Delete(sessionID)
		cancel()
		a.Publish(pubsub.CreatedEvent, result)
		events <- result
		close(events)
	}()

	return events, nil
}
