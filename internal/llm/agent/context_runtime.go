package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openscholar/openscholar/internal/config"
	llmcontext "github.com/openscholar/openscholar/internal/llm/context"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/prompt"
	"github.com/openscholar/openscholar/internal/llm/prompt/modules"
	"github.com/openscholar/openscholar/internal/llm/provider"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/memory"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/permission"
)

type requestRuntimeCtxKeyType struct{}

var requestRuntimeCtxKey = requestRuntimeCtxKeyType{}

// RequestRuntime carries request-scoped runtime overrides for a single Run call.
type RequestRuntime struct {
	AllowedTools           []string
	Model                  string
	DisableModelInvocation bool
}

// WithRequestRuntime attaches runtime overrides to a request context.
func WithRequestRuntime(ctx context.Context, runtime RequestRuntime) context.Context {
	normalized := runtime
	normalized.Model = strings.TrimSpace(normalized.Model)
	normalized.AllowedTools = normalizeToolNames(normalized.AllowedTools)
	if normalized.Model == "" && len(normalized.AllowedTools) == 0 && !normalized.DisableModelInvocation {
		return ctx
	}
	return context.WithValue(ctx, requestRuntimeCtxKey, normalized)
}

// RequestRuntimeFromContext returns the request runtime overrides stored in ctx.
func RequestRuntimeFromContext(ctx context.Context) (RequestRuntime, bool) {
	runtime, ok := ctx.Value(requestRuntimeCtxKey).(RequestRuntime)
	if !ok {
		return RequestRuntime{}, false
	}
	return runtime, true
}

type RequestView struct {
	Messages           []message.Message
	ActiveTools        []tools.BaseTool
	SystemMessage      string
	SystemBlocks       []prompt.PromptBlock
	PrefixMessageCount int
}

type ContextReport struct {
	SessionID            string
	ContextWindow        int64
	CurrentUsage         message.Usage
	CurrentUsageTokens   int64
	CurrentUsagePct      int
	RemainingPct         int
	ThresholdTokenCount  int64
	ThresholdEstimated   bool
	CountedInputTokens   int64
	CountedPrecisely     bool
	APIMessageCount      int
	ActiveToolCount      int
	Analysis             ContextAnalysis
	Suggestions          []ContextSuggestion
	CumulativePrompt     int64
	CumulativeCompletion int64
	CumulativeCacheRead  int64
	CumulativeCacheWrite int64
	CumulativeCostUSD    float64
}

type ToolBreakdown struct {
	Name         string
	CallCount    int
	InputTokens  int
	ResultTokens int
}

func (a *agent) buildRequestView(ctx context.Context, sessionID string, msgHistory []message.Message) RequestView {
	activeTools := a.activeTools()
	currentMode := permission.CurrentMode(ctx)
	if a.permissionService != nil && sessionID != "" {
		currentMode = a.permissionService.SessionMode(sessionID)
	}
	if tools.IsResearchMode(ctx) {
		if a.registry != nil {
			activeTools = appendRequestTools(ctx, activeTools, a.registry,
				"Task",
				"ResearchPipeline",
				"ResearchTask",
				"ResearchMessage",
			)
		}
		activeTools = tools.FilterResearchToolsForContext(ctx, activeTools)
	}
	if runtime, ok := RequestRuntimeFromContext(ctx); ok && len(runtime.AllowedTools) > 0 {
		if a.registry != nil {
			activeTools = appendRequestTools(ctx, activeTools, a.registry, runtime.AllowedTools...)
		}
		allowedSet, _ := resolveAllowedToolSet(activeTools, runtime.AllowedTools)
		filtered := make([]tools.BaseTool, 0, len(activeTools))
		for _, tool := range activeTools {
			if _, ok := allowedSet[tool.Info().Name]; ok {
				filtered = append(filtered, tool)
			}
		}
		activeTools = filtered
	}
	if currentMode == permission.ModePlan {
		if a.registry != nil {
			activeTools = appendRequestTools(ctx, activeTools, a.registry, "AskUser", "ExitPlanMode", "Write", "Edit", "ToolSearch")
		}
		filtered := make([]tools.BaseTool, 0, len(activeTools))
		for _, tool := range activeTools {
			if tools.PlanModeToolAllowed(tool.Info().Name) {
				filtered = append(filtered, tool)
			}
		}
		activeTools = filtered
	}

	view := deepCopyMessages(msgHistory)
	view = normalizeResumeHistoryForProvider(view)
	prefixMessageCount := 0

	if currentMode == permission.ModePlan {
		if reminder := a.planModeReminder(ctx, sessionID); reminder != "" {
			view = append([]message.Message{{
				Role:  message.User,
				Parts: []message.ContentPart{message.TextContent{Text: reminder}},
			}}, view...)
			prefixMessageCount++
		}
	}

	if rc, ok := ctx.Value(tools.ResearchContextContextKey).(string); ok && rc != "" {
		view = append([]message.Message{{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: rc}},
		}}, view...)
		prefixMessageCount++
	}

	if a.memoryService != nil {
		sessItems, globalItems, err := a.memoryService.RetrieveForPrompt(ctx, sessionID, memory.DefaultPromptOptions)
		if err == nil && (len(sessItems) > 0 || len(globalItems) > 0) {
			sessEntries := memoryItemsToEntries(sessItems)
			globalEntries := memoryItemsToEntries(globalItems)
			if memText := modules.FormatMemoryContext(sessEntries, globalEntries); memText != "" {
				view = append([]message.Message{{
					Role:  message.User,
					Parts: []message.ContentPart{message.TextContent{Text: memText}},
				}}, view...)
				prefixMessageCount++
			}
		}
	}

	if a.sessionMemoryLoader != nil {
		if notes, err := a.sessionMemoryLoader(ctx, sessionID); err == nil && notes != "" {
			view = append([]message.Message{{
				Role: message.User,
				Parts: []message.ContentPart{message.TextContent{
					Text: "<session-memory>\n" + notes + "\n</session-memory>",
				}},
			}}, view...)
			prefixMessageCount++
		}
	}

	cfg := config.Get()
	if cfg != nil && cfg.Harness.MicroCompactEnabled {
		compacted, _ := MicroCompact(view, cfg.Harness.MicroCompactKeepRecent)
		view = compacted
	}

	runtimePrompt := prompt.BuildAgentPromptRuntime(a.agentName, time.Now())
	return RequestView{
		Messages:           view,
		ActiveTools:        activeTools,
		SystemMessage:      runtimePrompt.SystemMessage,
		SystemBlocks:       runtimePrompt.Blocks,
		PrefixMessageCount: prefixMessageCount,
	}
}

func normalizeResumeHistoryForProvider(msgs []message.Message) []message.Message {
	copied := deepCopyMessages(msgs)
	out := make([]message.Message, 0, len(copied))
	for i := 0; i < len(copied); i++ {
		msg := copied[i]
		if msg.Role == message.Tool {
			continue
		}
		if msg.Role != message.Assistant {
			out = append(out, msg)
			continue
		}

		msg.CleanIncompleteToolCalls()
		calls := msg.ToolCalls()
		if len(calls) == 0 {
			if len(msg.Parts) > 0 {
				out = append(out, msg)
			}
			continue
		}

		callIDs := make(map[string]struct{}, len(calls))
		for _, call := range calls {
			if call.ID != "" {
				callIDs[call.ID] = struct{}{}
			}
		}

		j := i + 1
		adjacentTools := make([]message.Message, 0)
		resultIDs := make(map[string]struct{}, len(callIDs))
		for j < len(copied) && copied[j].Role == message.Tool {
			toolMsg := copied[j]
			parts := make([]message.ContentPart, 0, len(toolMsg.Parts))
			for _, part := range toolMsg.Parts {
				tr, ok := part.(message.ToolResult)
				if !ok {
					continue
				}
				if _, wanted := callIDs[tr.ToolCallID]; !wanted {
					continue
				}
				resultIDs[tr.ToolCallID] = struct{}{}
				parts = append(parts, tr)
			}
			if len(parts) > 0 {
				toolMsg.Parts = parts
				adjacentTools = append(adjacentTools, toolMsg)
			}
			j++
		}

		parts := make([]message.ContentPart, 0, len(msg.Parts))
		for _, part := range msg.Parts {
			tc, ok := part.(message.ToolCall)
			if !ok {
				parts = append(parts, part)
				continue
			}
			if _, hasResult := resultIDs[tc.ID]; hasResult {
				parts = append(parts, tc)
			}
		}
		if len(parts) > 0 {
			msg.Parts = parts
			out = append(out, msg)
			out = append(out, adjacentTools...)
		}
		i = j - 1
	}
	return out
}

func (a *agent) planModeReminder(ctx context.Context, sessionID string) string {
	if a.planService == nil || sessionID == "" {
		return `<plan-mode>
Plan mode is active. Explore with read-only tools, keep the plan in the session plan file, and call ExitPlanMode for approval before implementation.
</plan-mode>`
	}
	pf, err := a.planService.Ensure(ctx, sessionID)
	if err != nil {
		return fmt.Sprintf(`<plan-mode>
Plan mode is active, but the plan file could not be prepared: %v.
Use read-only tools and ask the user for guidance if you cannot continue safely.
</plan-mode>`, err)
	}
	existence := "does not exist yet"
	if pf.Exists {
		existence = "already exists"
	}
	return fmt.Sprintf(`<plan-mode>
Plan mode is active.

Current plan file: %s (%s)

Rules:
- Explore with read-only tools only.
- Write/Edit may target only this exact plan file.
- Do not modify source files, config, dependencies, external services, or workspace artifacts.
- Use AskUser only for clarification, not approval.
- Keep the plan file as Markdown body only (no protocol wrapper tags).
- When the plan is complete, call ExitPlanMode to request dedicated approval; it submits a canonical <proposed_plan>...</proposed_plan>.
</plan-mode>`, pf.Path, existence)
}

func appendRequestTools(ctx context.Context, active []tools.BaseTool, registry *tools.DeferredRegistry, names ...string) []tools.BaseTool {
	if registry == nil || len(names) == 0 {
		return active
	}

	seen := make(map[string]struct{}, len(active))
	for _, tool := range active {
		seen[tool.Info().Name] = struct{}{}
	}

	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		if !registry.ToolEligible(ctx, name, "") {
			continue
		}
		tool, ok := registry.FindTool(name)
		if !ok {
			continue
		}
		active = append(active, tool)
		seen[name] = struct{}{}
	}
	return active
}

func (a *agent) AnalyzeContext(ctx context.Context, sessionID string) (ContextReport, error) {
	if sessionID == "" {
		return ContextReport{}, fmt.Errorf("no active session")
	}

	msgs, err := a.messages.List(ctx, sessionID)
	if err != nil {
		return ContextReport{}, err
	}
	sess, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return ContextReport{}, err
	}
	msgs = historyAfterSummaryBoundary(msgs, sess.SummaryMessageID)

	view := a.buildRequestView(ctx, sessionID, msgs)
	snapshot := a.ContextSnapshot()
	contextWindow := models.RuntimeContextWindow(a.modelForRequest(ctx))
	currentPct := int(llmcontext.CurrentUsagePercentage(snapshot.CurrentUsage, contextWindow))
	if currentPct < 0 {
		currentPct = 0
	}
	if currentPct > 100 {
		currentPct = 100
	}

	countedTokens := int64(0)
	countedPrecisely := false
	if tc, err := a.providerForRequest(ctx).CountTokens(ctx, view.Messages, view.ActiveTools); err == nil {
		countedTokens = tc.InputTokens
		countedPrecisely = true
	} else if !provider.IsCountTokensUnsupported(err) {
		return ContextReport{}, err
	}

	analyzer := NewContextAnalyzer()
	analysis := analyzer.Analyze(sessionID, view.SystemBlocks, view.Messages, int(countedTokens))
	tokenEstimate := llmcontext.TokenCountWithEstimation(msgs)

	costState := a.CostState()
	var totalInput, totalOutput, totalCacheRead, totalCacheWrite int64
	for _, mu := range costState.ByModel {
		totalInput += mu.InputTokens
		totalOutput += mu.OutputTokens
		totalCacheRead += mu.CacheReadTokens
		totalCacheWrite += mu.CacheWriteTokens
	}

	return ContextReport{
		SessionID:            sessionID,
		ContextWindow:        contextWindow,
		CurrentUsage:         snapshot.CurrentUsage,
		CurrentUsageTokens:   llmcontext.CurrentUsageTokens(snapshot.CurrentUsage),
		CurrentUsagePct:      currentPct,
		RemainingPct:         100 - currentPct,
		ThresholdTokenCount:  tokenEstimate.TokenCount,
		ThresholdEstimated:   !tokenEstimate.HasRealUsage || tokenEstimate.EstimatedDelta > 0,
		CountedInputTokens:   countedTokens,
		CountedPrecisely:     countedPrecisely,
		APIMessageCount:      len(view.Messages),
		ActiveToolCount:      len(view.ActiveTools),
		Analysis:             analysis,
		Suggestions:          NewSuggestionEngine().Suggest(analysis, int(contextWindow), true),
		CumulativePrompt:     totalInput,
		CumulativeCompletion: totalOutput,
		CumulativeCacheRead:  totalCacheRead,
		CumulativeCacheWrite: totalCacheWrite,
		CumulativeCostUSD:    costState.Total,
	}, nil
}

func SortedToolBreakdown(byTool map[string]ToolTokenStats) []ToolBreakdown {
	result := make([]ToolBreakdown, 0, len(byTool))
	for name, stats := range byTool {
		result = append(result, ToolBreakdown{
			Name:         name,
			CallCount:    stats.CallCount,
			InputTokens:  stats.InputTokens,
			ResultTokens: stats.ResultTokens,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i].InputTokens + result[i].ResultTokens
		right := result[j].InputTokens + result[j].ResultTokens
		if left == right {
			return result[i].Name < result[j].Name
		}
		return left > right
	})
	return result
}
