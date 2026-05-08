package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"time"

	"github.com/openscholar/openscholar/internal/bib"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/db"
	"github.com/openscholar/openscholar/internal/debug"
	"github.com/openscholar/openscholar/internal/evolution"
	"github.com/openscholar/openscholar/internal/hooks"
	"github.com/openscholar/openscholar/internal/kb"
	"github.com/openscholar/openscholar/internal/llm/agent"
	llmcontext "github.com/openscholar/openscholar/internal/llm/context"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/prompt"
	"github.com/openscholar/openscholar/internal/llm/prompt/modules"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/llm/tools/codeagent"
	"github.com/openscholar/openscholar/internal/llm/web"
	"github.com/openscholar/openscholar/internal/magicdoc"
	"github.com/openscholar/openscholar/internal/mcp"
	"github.com/openscholar/openscholar/internal/memory"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/plan"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/research"
	researchcoord "github.com/openscholar/openscholar/internal/research/coordinator"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/skillbank"
	"github.com/openscholar/openscholar/internal/task"
)

type App struct {
	Sessions             session.Service
	Resume               session.ResumeService
	Messages             message.Service
	Permissions          permission.Service
	Plans                plan.Service
	CoderAgent           agent.Service
	BibEntries           bib.Service
	ClarificationBroker  *pubsub.Broker[tools.ClarificationEvent]
	PlanApprovalBroker   *pubsub.Broker[tools.PlanApprovalEvent]
	KB                   kb.Service
	KBIndexer            kb.Indexer
	SkillBank            skillbank.Service
	Memory               memory.Service
	MCPClient            *mcp.Client
	EvolutionStore       *evolution.Store
	EvolutionService     evolution.Service
	ResearchEngine       *research.Engine
	ResearchEvents       *pubsub.Broker[research.ResearchEvent] // Wave 4: research pipeline events
	ResearchCoordinator  *researchcoord.Coordinator
	ResearchWorkerPool   *researchcoord.WorkerPool
	CheckpointBroker     *pubsub.Broker[tools.CheckpointEvent]
	HookService          hooks.Service               // Phase 4: lifecycle hooks (nil-safe)
	TaskRegistry         *task.Registry              // Phase 4: unified task registry (nil-safe)
	CronScheduler        CronSchedulerService        // Phase 4: cron scheduler (nil on non-unix or if disabled)
	ForkedRunner         agent.ForkedRunner          // Phase 5: snapshot-based background LLM runner
	PostSamplingRegistry agent.PostSamplingRegistry  // Phase 5: post-sampling hook registry
	CheckpointStore      agent.CheckpointStore       // Phase 5: file checkpointing
	SessionMemory        session.MemoryManager       // Phase 5: rolling session memory
	MemoryEvents         *pubsub.Broker[MemoryEvent] // Wave 3: memory update notifications
	MagicDocService      magicdoc.Service            // Phase 5: magic doc auto-update
	DependencyWarnings   []string                    // Populated at startup, shown once in TUI
	taskAgentRunner      tools.AgentRunner
	kbServices           *tools.KBServices
	subagentScheduler    *tools.SubagentScheduler
	webRuntime           *web.Runtime
}

func New(ctx context.Context, conn *sql.DB) (*App, error) {
	q := db.New(conn)
	sessions := session.NewService(q)
	messages := message.NewService(q)

	app := &App{
		Sessions:            sessions,
		Resume:              session.NewResumeService(q, sessions, messages),
		Messages:            messages,
		Permissions:         permission.NewPermissionServiceWithDB(q, config.WorkingDirectory()),
		Plans:               plan.NewService(config.StatePath("plans")),
		BibEntries:          bib.NewService(q),
		ClarificationBroker: pubsub.NewBroker[tools.ClarificationEvent](),
		PlanApprovalBroker:  pubsub.NewBroker[tools.PlanApprovalEvent](),
		CheckpointBroker:    pubsub.NewBroker[tools.CheckpointEvent](),
	}
	app.Permissions.SetPlanFileResolver(app.Plans)

	// Rotate old debug logs on startup
	debug.RotateLogs(config.LogDir(), 7*24*time.Hour)

	// Initialize knowledge base service (pass raw DB for FTS5 queries)
	app.KB = kb.NewService(q, conn)

	// KB Indexer (non-fatal if python not available)
	kbIndexer, err := kb.NewPythonIndexer()
	if err != nil {
		log.Printf("[warn] KB indexer unavailable: %v", err)
		app.DependencyWarnings = append(app.DependencyWarnings,
			fmt.Sprintf("KB indexer unavailable: %v. Run `openscholar doctor` for details.", err))
	}
	app.KBIndexer = kbIndexer

	kbs := &tools.KBServices{
		KB:      app.KB,
		Indexer: app.KBIndexer,
	}
	app.kbServices = kbs

	// Initialize skill bank (general-purpose skill registry)
	// LLM caller is nil at startup; set later when provider is ready.
	app.SkillBank = skillbank.NewService(conn, config.ExtensionsPath("skills"), nil)

	// Wire skill catalog loader for L1 prompt injection (progressive disclosure).
	// Uses context.Background() instead of the init ctx to avoid lifecycle issues
	// when the prompt is rebuilt after provider changes (SetModel, ReloadProvider).
	prompt.SetSkillMetaLoader(func() []modules.SkillCatalogEntry {
		metas, err := app.SkillBank.ListMetadata(context.Background())
		if err != nil {
			return nil
		}
		entries := make([]modules.SkillCatalogEntry, 0, len(metas))
		for _, m := range metas {
			if !m.IsModelInvocable() {
				continue
			}
			entries = append(entries, modules.SkillCatalogEntry{
				ID:                     m.ID,
				Name:                   m.Name,
				Description:            m.Description,
				WhenToUse:              m.WhenToUse,
				Category:               m.Category,
				Exposure:               string(m.Exposure),
				Paths:                  append([]string(nil), m.Paths...),
				Agent:                  m.Agent,
				Context:                m.Context,
				Effort:                 m.Effort,
				Source:                 m.Source,
				UsageCount:             m.UsageCount,
				AllowedTools:           append([]string(nil), m.AllowedTools...),
				Model:                  m.Model,
				UserInvocable:          m.UserInvocable,
				DisableModelInvocation: m.DisableModelInvocation,
			})
		}
		return entries
	})

	// Initialize memory service (LLM caller will be nil until provider is ready)
	// Memory skills are loaded from SkillBank with category="memory"
	app.Memory = memory.NewService(q, app.SkillBank, nil)

	// Initialize research engine
	researchStore := research.NewStore(conn)
	researchBroker := pubsub.NewBroker[research.ResearchEvent]()
	app.ResearchEvents = researchBroker
	app.ResearchEngine = research.NewEngine(researchStore, researchBroker)

	// Initialize evolution store and service for skill reflection
	app.EvolutionStore = evolution.NewStore(conn)
	// LLM caller is nil at startup; set later when provider is ready (same as SkillBank).
	app.EvolutionService = evolution.NewService(app.EvolutionStore, app.SkillBank, nil)

	// Initialize MCP client (connect to configured MCP servers)
	cfg := config.Get()
	if cfg != nil && len(cfg.MCPServers) > 0 {
		app.MCPClient = mcp.New()
		app.MCPClient.ConnectAll(ctx, cfg.MCPServers)
	}

	// Phase 4: Initialize hooks service from config (nil-safe, fail-open)
	if cfg != nil && len(cfg.Hooks) > 0 {
		app.HookService = hooks.New(cfg.Hooks)
	}

	// Phase 4: Initialize task registry with background GC
	app.TaskRegistry = task.NewRegistry()
	app.TaskRegistry.StartGC(ctx, 5*time.Minute)
	tools.SetTaskRuntime(app.TaskRegistry, app.HookService)
	app.initResearchCoordinatorRuntime(ctx)

	// Phase 4: Initialize cron scheduler (unix-only, nil-safe)
	app.CronScheduler = initCronSchedulerIfAvailable(ctx, app.TaskRegistry)

	// AgentRunner callback for the Task tool — breaks the tools→agent circular import.
	agentRunner := func(ctx context.Context, agentName config.AgentName, sessionID string, prompt string, agentTools []tools.BaseTool) (string, error) {
		childAgent, err := agent.NewAgent(agentName, app.Sessions, app.Messages, agentTools)
		if err != nil {
			return "", fmt.Errorf("failed to create sub-agent: %w", err)
		}
		// Phase 4: inject hooks into sub-agents for consistent lifecycle coverage
		if app.HookService != nil {
			childAgent.SetHookService(app.HookService)
		}
		if app.TaskRegistry != nil {
			childAgent.SetTaskRegistry(app.TaskRegistry)
		}
		if runtime, ok := tools.TaskRuntimeOverrideFromContext(ctx); ok {
			ctx = agent.WithRequestRuntime(ctx, agent.RequestRuntime{
				Model:        runtime.Model,
				AllowedTools: runtime.AllowedTools,
			})
		}
		eventCh, err := childAgent.Run(ctx, sessionID, prompt)
		if err != nil {
			return "", fmt.Errorf("sub-agent failed to start: %w", err)
		}
		result := <-eventCh
		if result.Error != nil {
			return "", result.Error
		}
		content := result.Message.Content()
		if content.String() != "" {
			return content.String(), nil
		}
		return "", nil
	}
	app.taskAgentRunner = agentRunner

	// Research controller adapter for ResearchControl tool
	var researchCtrl tools.ResearchController
	if app.ResearchEngine != nil {
		researchCtrl = &researchControllerAdapter{engine: app.ResearchEngine}
	}

	// MCP caller adapter for CodeAgent (MCP providers take priority over Bash)
	var mcpCaller codeagent.MCPCaller
	if app.MCPClient != nil {
		mcpCaller = &mcpCallerAdapter{client: app.MCPClient}
	}

	registry := tools.NewCoderRegistry(tools.ToolDeps{
		Perms:            app.Permissions,
		Sessions:         app.Sessions,
		Messages:         app.Messages,
		RunAgent:         agentRunner,
		AskBroker:        app.ClarificationBroker,
		Plans:            app.Plans,
		PlanBroker:       app.PlanApprovalBroker,
		KBs:              kbs,
		CallLLM:          nil, // set later after provider is ready
		MemService:       app.Memory,
		Skills:           app.SkillBank,
		EvoService:       app.EvolutionService,
		ResearchCtrl:     researchCtrl,
		CheckpointBroker: app.CheckpointBroker,
		MCPCaller:        mcpCaller,
	})
	app.CoderAgent, err = agent.NewAgentWithRegistry(
		config.AgentCoder,
		app.Sessions,
		app.Messages,
		registry,
		app.Memory,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create coder agent: %w", err)
	}

	// Phase 4: inject hook service and task registry into agent (nil-safe)
	if app.HookService != nil {
		app.CoderAgent.SetHookService(app.HookService)
	}
	if app.TaskRegistry != nil {
		app.CoderAgent.SetTaskRegistry(app.TaskRegistry)
	}
	app.CoderAgent.SetPermissionService(app.Permissions)
	app.CoderAgent.SetPlanService(app.Plans)

	// Phase 5: initialize forked runner, post-sampling registry, and checkpoint store
	forkedRunner, err := agent.NewForkedRunnerForAgent(config.AgentCoder, slog.Default())
	if err != nil {
		log.Printf("[warn] forked runner unavailable: %v", err)
	} else {
		app.ForkedRunner = forkedRunner
		app.CoderAgent.SetForkedRunner(forkedRunner)
	}

	app.PostSamplingRegistry = agent.NewPostSamplingRegistry(slog.Default())
	app.CoderAgent.SetPostSamplingRegistry(app.PostSamplingRegistry)

	checkpointDir := config.StatePath("file-history")
	app.CheckpointStore = agent.NewCheckpointStore(checkpointDir)
	app.CoderAgent.SetCheckpointStore(app.CheckpointStore)

	// Phase 5 Fix 1: initialize and register Sprint 17 services
	dataDir := config.StatePath()
	app.SessionMemory = session.NewMemoryManager(dataDir, session.DefaultMemoryTrigger())
	app.MagicDocService = magicdoc.NewService(60 * time.Second)
	app.MemoryEvents = pubsub.NewBroker[MemoryEvent]()

	// Register PostSamplingHooks for Session Memory and MagicDocs
	app.PostSamplingRegistry.Register(&sessionMemoryHook{mgr: app.SessionMemory, runner: app.ForkedRunner, broker: app.MemoryEvents})
	app.PostSamplingRegistry.Register(&magicDocHook{svc: app.MagicDocService, runner: app.ForkedRunner})

	// Fix 2: wire session memory loader for prompt injection
	app.CoderAgent.SetSessionMemoryLoader(app.SessionMemory.LoadForPrompt)

	// Fix 2: wire MagicDoc file read notifier to View tool
	app.CoderAgent.SetFileReadNotifier(func(sessionID, filePath, content string) {
		app.MagicDocService.OnFileRead(magicdoc.FileReadEvent{
			SessionID: sessionID,
			FilePath:  filePath,
			Content:   content,
			ReadAt:    time.Now(),
		})
	})

	_, smallFastCallLLM, callLLMErr := app.refreshLLMCallers()

	// Register KB tools that require an LLM caller (deferred because of circular dependency).
	if kbs.KB != nil && callLLMErr == nil {
		kbCallLLM := func(ctx context.Context, prompt string) (string, error) {
			if app.kbServices == nil || app.kbServices.CallLLM == nil {
				return "", errors.New("KB LLM caller unavailable")
			}
			return app.kbServices.CallLLM(ctx, prompt)
		}
		registry.RegisterDeferred(tools.NewKBQueryTool(kbs.KB, kbCallLLM, app.Memory))
		registry.RegisterDeferred(tools.NewKBSearchTool(kbs.KB, kbCallLLM, app.Memory))
	}

	// Register Web tools (WebSearch + WebFetch) — deferred, activated via ToolSearch.
	{
		webCfg := mergeWebConfig(config.Get().Web)
		router := buildWebSearchProvider(webCfg)
		webRT := web.NewRuntime(router, smallFastCallLLM, webCfg)
		app.webRuntime = webRT
		kbs.WebRuntime = webRT
		registry.RegisterDeferred(tools.NewWebSearchTool(app.Permissions, webRT))
		registry.RegisterDeferred(tools.NewWebFetchTool(app.Permissions, webRT))
	}

	return app, nil
}

func (app *App) SetModel(modelID models.ModelID) error {
	ref, err := models.ResolveModelRef(string(modelID))
	if err != nil {
		return err
	}
	return app.SetModelProvider(ref.Provider, ref.ModelID)
}

func (app *App) SetModelProvider(providerName models.ModelProvider, modelID string) error {
	ref, err := models.ResolveModelRefForProvider(providerName, modelID)
	if err == nil {
		if ref.Metadata == nil {
			return fmt.Errorf("model %s resolved without metadata", modelID)
		}
		return app.setModelProvider(providerName, modelID, *ref.Metadata, false)
	}
	cfg := config.Get()
	if cfg != nil {
		if providerCfg, ok := cfg.Providers[providerName]; ok {
			if modelCfg, ok := providerCfg.Models[modelID]; ok {
				return app.setModelProvider(providerName, modelID, config.ModelFromConfig(providerName, modelID, modelCfg), false)
			}
		}
	}
	return err
}

func (app *App) SetModelProviderForce(providerName models.ModelProvider, modelID string) error {
	ref, err := models.ResolveModelRefForProviderFallback(providerName, modelID)
	if err != nil {
		return err
	}
	if ref.Metadata == nil {
		return fmt.Errorf("model %s resolved without metadata", modelID)
	}
	return app.setModelProvider(providerName, modelID, *ref.Metadata, true)
}

func (app *App) setModelProvider(providerName models.ModelProvider, modelID string, model models.Model, force bool) error {
	if err := app.CoderAgent.SetModel(model); err != nil {
		return err
	}
	var err error
	if force {
		err = config.SaveAgentModelProviderForce(config.WorkingDirectory(), config.AgentCoder, providerName, modelID)
	} else {
		err = config.SaveAgentModelProvider(config.WorkingDirectory(), config.AgentCoder, providerName, modelID)
	}
	if err != nil {
		return err
	}
	_, _, err = app.refreshLLMCallers()
	return err
}

func (app *App) ReloadProvider() error {
	if err := app.CoderAgent.ReloadProvider(); err != nil {
		return err
	}
	_, _, err := app.refreshLLMCallers()
	return err
}

func (app *App) refreshLLMCallers() (func(ctx context.Context, prompt string) (string, error), func(ctx context.Context, prompt string) (string, error), error) {
	coderCallLLM, err := agent.CreateCallLLM(config.AgentCoder)
	if err != nil {
		return nil, nil, err
	}
	smallFastCallLLM, err := agent.CreateSmallFastCallLLM()
	if err != nil {
		return nil, nil, err
	}
	if app.Memory != nil {
		app.Memory.SetLLMCaller(coderCallLLM)
	}
	if app.EvolutionService != nil {
		app.EvolutionService.SetLLMCaller(coderCallLLM)
	}
	if app.kbServices != nil {
		app.kbServices.CallLLM = smallFastCallLLM
		app.kbServices.MemService = app.Memory
	}
	if app.webRuntime != nil {
		webCfg := mergeWebConfig(config.Get().Web)
		app.webRuntime.SetLLMCaller(smallFastCallLLM)
		app.webRuntime.SetSearchProvider(buildWebSearchProvider(webCfg))
	}
	return coderCallLLM, smallFastCallLLM, nil
}

func (app *App) RunNonInteractive(ctx context.Context, prompt string) error {
	return app.runNonInteractive(ctx, "", prompt)
}

func (app *App) RunNonInteractiveInSession(ctx context.Context, sessionID string, prompt string) error {
	return app.runNonInteractive(ctx, sessionID, prompt)
}

func (app *App) runNonInteractive(ctx context.Context, sessionID string, prompt string) error {
	const maxTitleLen = 100
	if sessionID == "" {
		titlePrefix := "Non-interactive: "
		titleSuffix := prompt
		if len(titleSuffix) > maxTitleLen {
			titleSuffix = titleSuffix[:maxTitleLen] + "..."
		}
		sess, err := app.Sessions.Create(ctx, titlePrefix+titleSuffix)
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
		sessionID = sess.ID
	}

	// Auto-approve all permissions for non-interactive mode
	app.Permissions.AutoApproveSession(sessionID)

	done, err := app.CoderAgent.Run(ctx, sessionID, prompt)
	if err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	result := <-done
	if result.Error != nil {
		if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, agent.ErrRequestCancelled) {
			return nil
		}
		return fmt.Errorf("agent failed: %w", result.Error)
	}

	content := result.Message.Content().String()
	if content != "" {
		fmt.Println(content)
	}

	return nil
}

func (app *App) Shutdown() {
	// Phase 4: stop cron scheduler (releases PID lock)
	if app.CronScheduler != nil {
		app.CronScheduler.Stop()
	}
	// Phase 4: shutdown task registry (stops GC goroutine + broker)
	if app.TaskRegistry != nil {
		app.TaskRegistry.Shutdown()
	}
	// Cleanup resources
	if app.ClarificationBroker != nil {
		app.ClarificationBroker.Shutdown()
	}
	if app.PlanApprovalBroker != nil {
		app.PlanApprovalBroker.Shutdown()
	}
	if app.CheckpointBroker != nil {
		app.CheckpointBroker.Shutdown()
	}
	if app.MCPClient != nil {
		app.MCPClient.Close()
	}
}

// CronSchedulerService is the interface satisfied by *cron.Scheduler.
// Defined here so app.go stays cross-platform (cron.Scheduler is unix-only).
type CronSchedulerService interface {
	Stop()
	IsOwner() bool
}

// researchControllerAdapter adapts *research.Engine to the tools.ResearchController interface.
type researchControllerAdapter struct {
	engine *research.Engine
}

func (a *researchControllerAdapter) GetBySession(sessionID string) (tools.ResearchPipelineView, error) {
	p, err := a.engine.GetBySession(sessionID)
	if err != nil {
		return tools.ResearchPipelineView{}, err
	}
	return tools.ResearchPipelineView{
		ID:          p.ID,
		Topic:       p.Topic,
		Template:    p.Template,
		Status:      string(p.Status),
		WorkDir:     p.WorkDir,
		Mode:        string(p.Mode),
		BudgetLimit: p.Budget.Limit,
		BudgetSpent: p.Budget.Spent,
	}, nil
}

func (a *researchControllerAdapter) GetPhasesView(pipelineID string) ([]tools.ResearchPhaseView, error) {
	phases, err := a.engine.GetPhases(pipelineID)
	if err != nil {
		return nil, err
	}
	views := make([]tools.ResearchPhaseView, len(phases))
	for i, ph := range phases {
		views[i] = tools.ResearchPhaseView{
			ID:         ph.ID,
			Name:       ph.Name,
			Status:     string(ph.Status),
			Order:      ph.Order,
			Checkpoint: ph.Checkpoint,
		}
	}
	return views, nil
}

func (a *researchControllerAdapter) AdvancePipeline(pipelineID string) error {
	return a.engine.Advance(pipelineID)
}

func (a *researchControllerAdapter) AdvancePipelineWithOptions(pipelineID string, opts tools.ResearchAdvanceOptions) error {
	return a.engine.AdvanceWithOptions(pipelineID, research.AdvanceOptions{
		SkipCheckpoint: opts.SkipCheckpoint,
	})
}

func (a *researchControllerAdapter) PausePipeline(pipelineID string) error {
	return a.engine.Pause(pipelineID)
}

func (a *researchControllerAdapter) SetPipelineMode(pipelineID, mode string) error {
	return a.engine.SetMode(pipelineID, research.AutomationMode(mode))
}

// mcpCallerAdapter adapts *mcp.Client to the codeagent.MCPCaller interface,
// breaking the import cycle between codeagent ↔ mcp packages.
type mcpCallerAdapter struct {
	client *mcp.Client
}

func (a *mcpCallerAdapter) ServerNames() []string {
	return a.client.ServerNames()
}

func (a *mcpCallerAdapter) CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (codeagent.MCPResult, error) {
	result, err := a.client.CallTool(ctx, serverName, toolName, args)
	if err != nil {
		return codeagent.MCPResult{}, err
	}

	var contents []codeagent.MCPContent
	for _, c := range result.Content {
		// Extract text from MCP TextContent
		if tc, ok := c.(interface{ GetText() string }); ok {
			contents = append(contents, codeagent.MCPContent{Text: tc.GetText()})
		} else if s, ok := c.(fmt.Stringer); ok {
			contents = append(contents, codeagent.MCPContent{Text: s.String()})
		}
	}

	return codeagent.MCPResult{
		Content: contents,
		IsError: result.IsError,
	}, nil
}

// ─── Wave 3: Memory Event ───────────────────────────────��───────────────────

// MemoryEvent is published when session memory is updated.
type MemoryEvent struct {
	SessionID string
	FilePath  string // relative path of the notes file
	Action    string // "updated", "created"
}

// ─── Phase 5 Fix 1: PostSamplingHook adapters ──────────────────────────────

// sessionMemoryHook adapts session.MemoryManager as a PostSamplingHook.
type sessionMemoryHook struct {
	mgr    session.MemoryManager
	runner agent.ForkedRunner
	broker *pubsub.Broker[MemoryEvent] // Wave 3: publish memory events
}

func (h *sessionMemoryHook) Name() string { return "session-memory" }

func (h *sessionMemoryHook) Run(ctx context.Context, ps agent.PostSamplingContext) error {
	tokenEstimate := llmcontext.TokenCountWithEstimation(ps.History)
	currentTokens := int(tokenEstimate.TokenCount)

	if !h.mgr.ShouldTrigger(ps.SessionID, currentTokens, ps.ToolCallCount) {
		return nil
	}

	// Build recent turns text
	recent := ps.History
	if len(recent) > 10 {
		recent = recent[len(recent)-10:]
	}
	var recentText string
	for _, msg := range recent {
		content := msg.Content()
		if content.String() != "" {
			recentText += fmt.Sprintf("[%s]: %s\n", msg.Role, content.String())
		}
	}

	// Use ForkedRunner as the LLM query backend
	query := func(ctx context.Context, prompt string) (string, error) {
		if h.runner == nil || ps.Snapshot == nil {
			return "", fmt.Errorf("no forked runner or snapshot")
		}
		result, err := h.runner.Run(ctx, agent.ForkedRunOptions{
			Label:           "session-memory",
			Prompt:          prompt,
			ParentSessionID: ps.SessionID,
			Snapshot:        ps.Snapshot,
			MaxTokens:       2048,
		})
		if err != nil {
			return "", err
		}
		return result.FinalMessage.Content().String(), nil
	}

	err := h.mgr.UpdateNotesWithState(ctx, ps.SessionID, recentText, currentTokens, ps.ToolCallCount, query)
	if err == nil && h.broker != nil {
		h.broker.Publish(pubsub.UpdatedEvent, MemoryEvent{
			SessionID: ps.SessionID,
			FilePath:  "session-memory/" + ps.SessionID + "/notes.md",
			Action:    "updated",
		})
	}
	return err
}

// magicDocHook adapts magicdoc.Service as a PostSamplingHook.
type magicDocHook struct {
	svc    magicdoc.Service
	runner agent.ForkedRunner
}

func (h *magicDocHook) Name() string { return "magicdoc" }

func (h *magicDocHook) Run(ctx context.Context, ps agent.PostSamplingContext) error {
	pending := h.svc.PendingUpdates(ps.SessionID)
	if len(pending) == 0 {
		return nil
	}

	for _, doc := range pending {
		query := func(ctx context.Context, prompt string) (string, error) {
			if h.runner == nil || ps.Snapshot == nil {
				return "", fmt.Errorf("no forked runner or snapshot")
			}
			allowedTools := map[string]struct{}{"Edit": {}, "View": {}}
			result, err := h.runner.Run(ctx, agent.ForkedRunOptions{
				Label:            "magicdoc-" + doc.Title,
				Prompt:           prompt,
				ParentSessionID:  ps.SessionID,
				Snapshot:         ps.Snapshot,
				AllowedToolNames: allowedTools,
				MaxTokens:        4096,
			})
			if err != nil {
				return "", err
			}
			return result.FinalMessage.Content().String(), nil
		}
		_ = h.svc.RunUpdate(ctx, doc, query) // fail-open per doc
	}
	return nil
}

func buildWebSearchProvider(webCfg web.Config) web.SearchProvider {
	nativeBackend := agent.CreateWebSearchBackend(config.AgentCoder, webCfg.OpenAISearchModel)
	var anthropicBackend, openaiBackend web.SearchProvider
	switch agent.WebSearchBackendType(config.AgentCoder) {
	case "anthropic":
		anthropicBackend = nativeBackend
	case "openai":
		openaiBackend = nativeBackend
	}
	statePath := config.StatePath("web-search-state.json")
	return web.BuildSearchRouter(webCfg, anthropicBackend, openaiBackend, statePath)
}

// mergeWebConfig converts config.WebConfig into web.Config, applying defaults for unset fields.
func mergeWebConfig(wc config.WebConfig) web.Config {
	cfg := web.DefaultConfig()
	if wc.SearchMaxUses > 0 {
		cfg.SearchMaxUses = wc.SearchMaxUses
	}
	if wc.FetchCacheTTLMinutes > 0 {
		cfg.FetchCacheTTLMinutes = wc.FetchCacheTTLMinutes
	}
	if wc.FetchMaxResponseMB > 0 {
		cfg.FetchMaxResponseMB = wc.FetchMaxResponseMB
	}
	if wc.FetchMaxMarkdownLen > 0 {
		cfg.FetchMaxMarkdownLen = wc.FetchMaxMarkdownLen
	}
	if wc.OpenAISearchModel != "" {
		cfg.OpenAISearchModel = wc.OpenAISearchModel
	}
	if wc.URLPolicy.DNSFailMode != "" {
		cfg.URLPolicy.DNSFailMode = wc.URLPolicy.DNSFailMode
	}
	if wc.URLPolicy.BlockMetadataServices != nil {
		cfg.URLPolicy.BlockMetadataServices = *wc.URLPolicy.BlockMetadataServices
	}
	if wc.URLPolicy.BlockUserinfo != nil {
		cfg.URLPolicy.BlockUserinfo = *wc.URLPolicy.BlockUserinfo
	}
	if wc.URLPolicy.BlockURLSecrets != nil {
		cfg.URLPolicy.BlockURLSecrets = *wc.URLPolicy.BlockURLSecrets
	}
	if wc.URLPolicy.SkipCacheWhenHasSecret != nil {
		cfg.URLPolicy.SkipCacheWhenHasSecret = *wc.URLPolicy.SkipCacheWhenHasSecret
	}
	if wc.WebsitePolicy.Mode != "" {
		cfg.WebsitePolicy.Mode = wc.WebsitePolicy.Mode
	}
	if wc.WebsitePolicy.DefaultAction != "" {
		cfg.WebsitePolicy.DefaultAction = wc.WebsitePolicy.DefaultAction
	}
	if wc.Proxy.Mode != "" {
		cfg.Proxy.Mode = wc.Proxy.Mode
	}
	if wc.Proxy.URL != "" {
		cfg.Proxy.URL = wc.Proxy.URL
	}
	if wc.Proxy.AllowLocalProxy != nil {
		cfg.Proxy.AllowLocalProxy = *wc.Proxy.AllowLocalProxy
	}
	if len(wc.Proxy.FakeIPCIDRs) > 0 {
		cfg.Proxy.FakeIPCIDRs = append([]string(nil), wc.Proxy.FakeIPCIDRs...)
	}
	for _, r := range wc.WebsitePolicy.Rules {
		cfg.WebsitePolicy.Rules = append(cfg.WebsitePolicy.Rules, web.WebsitePolicyRule{
			Pattern: r.Pattern,
			Action:  r.Action,
		})
	}
	for _, bc := range wc.SearchBackends {
		cfg.SearchBackends = append(cfg.SearchBackends, web.SearchBackendConfig{
			Name:         bc.Name,
			Enabled:      bc.Enabled,
			Priority:     bc.Priority,
			APIKey:       bc.APIKey,
			BaseURL:      bc.BaseURL,
			MonthlyLimit: bc.MonthlyLimit,
			DailyLimit:   bc.DailyLimit,
			MaxUses:      bc.MaxUses,
		})
	}
	return cfg
}
