package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/debug"
	"github.com/openscholar/openscholar/internal/hooks"
	llmcontext "github.com/openscholar/openscholar/internal/llm/context"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/prompt/modules"
	"github.com/openscholar/openscholar/internal/llm/provider"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/memory"
	"github.com/openscholar/openscholar/internal/message"
	"github.com/openscholar/openscholar/internal/permission"
	"github.com/openscholar/openscholar/internal/pubsub"
)

const (
	maxTokensRecoveryLimit  = 3
	maxTokensRecoveryPrompt = "Output token limit hit. Resume directly — no apology, no recap of what you were doing. Pick up mid-thought if that is where the cut happened. Break remaining work into smaller pieces."
)

var errCompactExhausted = errors.New("auto-compact could not reduce context below the blocking threshold")

func (a *agent) processGeneration(ctx context.Context, sessionID, content string, extraParts ...message.ContentPart) AgentEvent {
	// Session logging setup (enabled by sessionLog or debug config)
	if config.SessionLogEnabled() {
		sessionDir := debug.SessionLogDirFromCtx(ctx)
		if sessionDir == "" {
			// Main agent first call: create session directory
			if dir, err := debug.CreateSessionDir(config.LogDir()); err == nil && dir != "" {
				sessionDir = dir
				ctx = debug.WithSessionLogDir(ctx, sessionDir)
			}
		}
		if sessionDir != "" {
			var dl *debug.SessionLogger
			if a.agentName != config.AgentCoder {
				// Sub-agent: separate log file per sub-agent
				dl, _ = debug.NewSubAgentLogger(sessionDir, sessionID, string(a.agentName), sessionID)
			} else {
				// Main agent: append to agent.log
				dl, _ = debug.NewMainAgentLogger(sessionDir, sessionID)
			}
			if dl != nil {
				defer dl.LogSessionEnd()
				defer dl.Close()
				ctx = context.WithValue(ctx, debugLoggerCtxKey, dl)
				dl.LogSessionStart(debug.SessionStartData{Model: string(a.modelForRequest(ctx).ID)})
			}
		}
	}

	// Phase 5: inject turnID and checkpoint store into context for Edit/Write tools
	turnID := uuid.New().String()
	ctx = context.WithValue(ctx, tools.TurnIDContextKey, turnID)
	if a.checkpointStore != nil {
		ctx = context.WithValue(ctx, tools.CheckpointStoreContextKey, &checkpointBridge{store: a.checkpointStore})
	}

	// Intent-based tool pre-activation: skip ToolSearch for common scenarios
	if a.registry != nil {
		a.registry.BeginTurn()
		ctx = tools.WithLocalKBIntent(ctx, tools.TextHasLocalKBIntent(content))
		a.preActivateToolsByIntent(content)
	}

	msgs, err := a.messages.List(ctx, sessionID)
	if err != nil {
		return a.terminalEvent(ReasonSessionError, fmt.Errorf("failed to list messages: %w", err))
	}

	// Set title from first user message for new sessions
	if len(msgs) == 0 {
		go a.setTitleFromFirstMessage(context.Background(), sessionID, content)
	}

	// Handle summary
	sess, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return a.terminalEvent(ReasonSessionError, fmt.Errorf("failed to get session: %w", err))
	}
	if sess.SummaryMessageID != "" {
		msgs = historyAfterSummaryBoundary(msgs, sess.SummaryMessageID)
	}

	// Note: auto-compact on session load is removed. Compaction only happens
	// mid-conversation when approaching the API token limit (120K tokens).
	// Users can also manually trigger /compact.

	// Create user message with optional extra parts (e.g. PDF attachments)
	userParts := []message.ContentPart{message.TextContent{Text: content}}
	userParts = append(userParts, extraParts...)
	userMsg, err := a.messages.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.User,
		Parts: userParts,
	})
	if err != nil {
		return a.terminalEvent(ReasonSessionError, fmt.Errorf("failed to create user message: %w", err))
	}

	// [Hook injection] UserPromptSubmit — after user message created
	if a.hookService != nil {
		hookInput := hooks.Input{
			SessionID:  sessionID,
			UserPrompt: content,
			Timestamp:  time.Now(),
		}
		if err := a.hookService.RunBlocking(ctx, hooks.UserPromptSubmit, hookInput); err != nil {
			return a.terminalEvent(ReasonSessionError, fmt.Errorf("user prompt hook failed: %w", err))
		}
	}

	// Log user message content
	if dl := debugLoggerFromCtx(ctx); dl != nil {
		dl.LogUserMessage(debug.UserMessageData{Content: content})
	}

	a.triggerMemoryCapture(sessionID, content)

	msgHistory := append(msgs, userMsg)

	// Main agent loop
	streamRetries := 0
	iteration := 0
	totalToolCalls := 0               // Phase 5 Fix 3: cumulative tool call count for PostSamplingContext
	generationStartedAt := time.Now() // Phase 5 Fix 3: track start time
	const maxStreamRetries = 5
	maxTokensRecoveryCount := 0

	// Loop Governance: build query config snapshot and initialize loop state
	model := a.modelForRequest(ctx)
	qcfg := BuildQueryConfig(sessionID, string(model.ID), models.RuntimeContextWindow(model), model.DefaultMaxTokens, config.Get())
	loopState := NewLoopState()
	stopController := newGenerationStopController(qcfg)

	for {
		iteration++
		loopState.Iteration = iteration
		if dl := debugLoggerFromCtx(ctx); dl != nil {
			dl.SetIteration(iteration)
		}
		select {
		case <-ctx.Done():
			return a.terminalEvent(ReasonUserCancelled, ctx.Err())
		default:
		}

		// Inject background task completion notifications from AsyncExecutor.
		if a.asyncExecutor != nil {
			for _, n := range a.asyncExecutor.DrainNotifications() {
				notifyMsg := message.Message{
					Role: message.User,
					Parts: []message.ContentPart{
						message.ToolResult{
							ToolCallID: "bg-notify-" + n.TaskID,
							Name:       "bg_complete",
							Content:    fmt.Sprintf("Background task %s (%s) %s in %s.\nResult:\n%s", n.TaskID, n.ToolName, n.Status, n.Duration, n.Result),
							IsError:    n.Status == "failed",
						},
					},
				}
				msgHistory = append(msgHistory, notifyMsg)
			}
		}
		taskNotifications, err := a.drainTaskNotifications(ctx, sessionID)
		if err != nil {
			return a.terminalEvent(ReasonSessionError, fmt.Errorf("failed to drain task notifications: %w", err))
		}
		if len(taskNotifications) > 0 {
			msgHistory = append(msgHistory, taskNotifications...)
		}

		// Build the final request view and run request-time preflight inside
		// streamAndHandleEvents so injected context and active tools are counted.
		roundCtx := context.WithValue(ctx, toolExecutionBudgetKey, toolExecutionBudget{
			RemainingToolCalls:   remainingBudget(qcfg.Loop.MaxToolCalls, loopState.TotalToolCalls),
			RemainingSearchCalls: remainingBudget(defaultMaxSearchToolCalls, loopState.SearchToolCalls),
		})
		assistantMsg, toolResults, err := a.streamAndHandleEvents(roundCtx, sessionID, msgHistory)
		if err != nil {
			if errors.Is(err, errCompactExhausted) {
				a.emitBudgetExhaustedHook(ctx, sessionID, ReasonCompactExhausted)
				return AgentEvent{
					Type:           AgentEventTypeResponse,
					SessionID:      sessionID,
					Message:        assistantMsg,
					Done:           true,
					TerminalReason: ReasonCompactExhausted,
					Warning:        TerminalReasonMessage(ReasonCompactExhausted),
				}
			}
			if errors.Is(err, context.Canceled) {
				assistantMsg.AddFinish(message.FinishReasonCanceled)
				a.messages.Update(context.Background(), assistantMsg)
				return a.terminalEvent(ReasonUserCancelled, ErrRequestCancelled)
			}
			// Retry transient stream errors (JSON parse, network)
			if isTransientStreamError(err) && streamRetries < maxStreamRetries {
				streamRetries++
				// Delete the failed assistant message to keep history clean
				a.messages.Delete(context.Background(), assistantMsg.ID)
				continue
			}
			return a.terminalEvent(ReasonStreamError, fmt.Errorf("failed to process events: %w", err))
		}
		streamRetries = 0 // reset on success
		loopState.StreamRetries = streamRetries

		// Loop Governance: update LoopState from last API usage (set by trackUsage)
		u := a.lastUsageData
		loopState.UpdateFromUsage(u.InputTokens, u.OutputTokens, u.CacheRead, u.CacheCreate, u.Cost, u.CostKnown)

		// Checkpoint A: per-turn budget check (cost/token hard stop)
		if a.budgetExceededHook != nil {
			if d := a.budgetExceededHook.Check(ctx, &loopState); d.Action == ActionTerminate {
				if dl := debugLoggerFromCtx(ctx); dl != nil {
					dl.LogBudgetDecision(debug.BudgetDecisionData{
						Action:          "terminate",
						Reason:          d.Reason,
						CostAccumulated: loopState.CostAccumulated,
					})
				}
				reason := stopCodeToTerminalReason(d.Code)
				slog.Info("budget exceeded, terminating loop", "reason", d.Reason, "code", d.Code)
				a.emitBudgetExhaustedHook(ctx, sessionID, reason)
				return AgentEvent{
					Type:           AgentEventTypeResponse,
					SessionID:      sessionID,
					Message:        assistantMsg,
					Done:           true,
					TerminalReason: reason,
					Warning:        warningForStopDecision(d),
				}
			} else {
				if dl := debugLoggerFromCtx(ctx); dl != nil {
					dl.LogBudgetDecision(debug.BudgetDecisionData{
						Action:          "continue",
						Reason:          "budget check passed",
						CostAccumulated: loopState.CostAccumulated,
					})
				}
			}
		}

		// Phase 5 Fix 5: include current assistant response in snapshot and history
		// Build up-to-date history that includes the current assistant message
		currentHistory := append(msgHistory, assistantMsg)

		// Phase 5: save snapshot after each successful LLM call
		if a.forkedRunner != nil {
			snap := CacheSafeSnapshot{
				SessionID:     sessionID,
				Model:         a.modelForRequest(ctx),
				ActiveTools:   a.activeTools(),
				MessagePrefix: currentHistory,
				CapturedAt:    time.Now(),
			}
			a.forkedRunner.SaveSnapshot(sessionID, snap)
		}

		if assistantMsg.FinishReason() == message.FinishReasonMaxTokens {
			if maxTokensRecoveryCount < maxTokensRecoveryLimit {
				maxTokensRecoveryCount++
				slog.Info("output token limit reached, continuing turn",
					"attempt", maxTokensRecoveryCount,
					"session_id", sessionID,
				)
				msgHistory = append(msgHistory, assistantMsg, message.Message{
					Role: message.User,
					Parts: []message.ContentPart{
						message.TextContent{Text: maxTokensRecoveryPrompt},
					},
				})
				continue
			}
			reason := ReasonMaxTokensRecoveryExhausted
			slog.Warn("output token limit recovery exhausted",
				"attempts", maxTokensRecoveryCount,
				"session_id", sessionID,
			)
			return AgentEvent{
				Type:           AgentEventTypeResponse,
				SessionID:      sessionID,
				Message:        assistantMsg,
				Done:           true,
				TerminalReason: reason,
				Warning:        TerminalReasonMessage(reason),
			}
		}
		maxTokensRecoveryCount = 0

		if assistantMsg.FinishReason() == message.FinishReasonToolUse && toolResults != nil {
			// Phase 5 Fix 3: count tool calls from this round + collect signatures
			roundToolCalls := 0
			var roundSignatures []string
			toolOutcomes := buildToolOutcomes(assistantMsg, *toolResults)
			for _, part := range assistantMsg.Parts {
				if tc, ok := part.(message.ToolCall); ok {
					totalToolCalls++
					roundToolCalls++
					sig := semanticToolSignature(tc, toolOutcomes)
					roundSignatures = append(roundSignatures, sig)
				}
			}
			for i := range toolOutcomes {
				if toolOutcomes[i].Signature == "" {
					for _, part := range assistantMsg.Parts {
						if tc, ok := part.(message.ToolCall); ok && tc.ID == toolOutcomes[i].ToolCallID {
							toolOutcomes[i].Signature = semanticToolSignature(tc, toolOutcomes)
						}
					}
				}
			}
			loopState.TotalToolCalls += roundToolCalls
			loopState.LastRoundToolSignatures = roundSignatures
			loopState.LastRoundToolOutcomes = toolOutcomes
			loopState.LastRoundProgress = classifyRoundProgress(&loopState, toolOutcomes)

			msgHistory = append(msgHistory, assistantMsg, *toolResults)

			// Phase 5: new turnID for each tool-use iteration
			turnID = uuid.New().String()
			ctx = context.WithValue(ctx, tools.TurnIDContextKey, turnID)

			// Loop Governance: check stop policies before continuing
			if stopController != nil {
				if d := stopController.Run(ctx, &loopState); d.Action == ActionTerminate {
					if dl := debugLoggerFromCtx(ctx); dl != nil {
						dl.LogStopDecision(debug.StopDecisionData{
							PolicyName:  d.PolicyName,
							Action:      "terminate",
							Code:        fmt.Sprintf("%d", int(d.Code)),
							Reason:      d.Reason,
							Diagnostics: d.Diagnostics,
						})
					}
					reason := stopCodeToTerminalReason(d.Code)
					slog.Info("stop controller terminated loop",
						"reason", d.Reason, "code", d.Code,
						"iteration", loopState.Iteration, "tool_calls", loopState.TotalToolCalls)
					// Graceful stop: return Done=true with Warning (not Error),
					// so TUI shows it as a status message instead of an inline error.
					finalMsg, finalWarn := a.finalizeWithoutTools(ctx, sessionID, msgHistory, d)
					return AgentEvent{Type: AgentEventTypeResponse, SessionID: sessionID, Message: finalMsg, Done: true, TerminalReason: reason, Warning: finalWarn}
				} else {
					if dl := debugLoggerFromCtx(ctx); dl != nil {
						dl.LogStopDecision(debug.StopDecisionData{
							Action:      "continue",
							Code:        fmt.Sprintf("%d", int(StopCodeNone)),
							Reason:      "stop policies passed",
							Diagnostics: loopDiagnostics(&loopState, qcfg),
						})
					}
				}
			}

			// Proactively compact before hitting the API context limit.
			// Claude Code semantics: use the last real usage snapshot plus an
			// estimated delta for messages added afterwards.
			compactThreshold := qcfg.AutoCompactThreshold
			if compactThreshold <= 0 {
				compactThreshold = 120000 // fallback
			}
			if a.summarizerProvider != nil && a.compactFailures < qcfg.Compact.MaxConsecutiveFailures {
				tokenCount := int(llmcontext.TokenCountWithEstimation(msgHistory).TokenCount)
				if tokenCount > compactThreshold {
					a.Publish(pubsub.CreatedEvent, AgentEvent{Type: AgentEventTypeCompacting, SessionID: sessionID})
					msgHistory = a.compactAndRebuildHistory(ctx, sessionID, msgHistory, qcfg)
					a.Publish(pubsub.CreatedEvent, AgentEvent{Type: AgentEventTypeCompactDone, SessionID: sessionID})
					tokenCount = int(llmcontext.TokenCountWithEstimation(msgHistory).TokenCount)
					if qcfg.BlockingCompactThreshold > 0 && tokenCount > qcfg.BlockingCompactThreshold {
						reason := stopCodeToTerminalReason(StopCodeCompactFuse)
						a.emitBudgetExhaustedHook(ctx, sessionID, reason)
						return AgentEvent{
							Type:           AgentEventTypeResponse,
							SessionID:      sessionID,
							Message:        assistantMsg,
							Done:           true,
							TerminalReason: reason,
							Warning:        TerminalReasonMessage(reason),
						}
					}
				}
			}
			continue
		}

		// Checkpoint C: continuation gate — BudgetController decides whether to continue
		if a.budgetController != nil {
			cd := a.budgetController.ShouldContinue(&loopState)
			if dl := debugLoggerFromCtx(ctx); dl != nil {
				action := "deny"
				switch cd.Action {
				case ContinuationAllow:
					action = "allow"
				case ContinuationHardStop:
					action = "hard_stop"
				}
				dl.LogBudgetDecision(debug.BudgetDecisionData{
					Action:          action,
					Reason:          cd.Reason,
					CostAccumulated: loopState.CostAccumulated,
					HasNudge:        cd.NudgeText != "",
				})
			}
			switch cd.Action {
			case ContinuationAllow:
				loopState.ContinuationCount++
				if cd.NudgeText != "" {
					nudgeMsg := message.Message{
						Role:  message.User,
						Parts: []message.ContentPart{message.TextContent{Text: cd.NudgeText}},
					}
					msgHistory = append(msgHistory, assistantMsg, nudgeMsg)
				} else {
					msgHistory = append(msgHistory, assistantMsg)
				}
				slog.Info("budget controller: continuing",
					"reason", cd.Reason, "continuation_count", loopState.ContinuationCount)
				continue
			case ContinuationHardStop:
				reason := stopCodeToTerminalReason(StopCodeCostLimit)
				slog.Info("budget controller: hard stop", "reason", cd.Reason)
				a.emitBudgetExhaustedHook(ctx, sessionID, reason)
				return AgentEvent{
					Type:           AgentEventTypeResponse,
					SessionID:      sessionID,
					Message:        assistantMsg,
					Done:           true,
					TerminalReason: reason,
					Warning:        "Agent stopped: " + cd.Reason,
				}
			case ContinuationDeny:
				// fall through to normal completion
			}
		}

		// Phase 5: fire post-sampling hooks on final response (not on tool-use)
		if a.postSamplingRegistry != nil {
			var latestSnapshot *CacheSafeSnapshot
			if a.forkedRunner != nil {
				if snap, ok := a.forkedRunner.LatestSnapshot(sessionID); ok {
					latestSnapshot = &snap
				}
			}
			psCtx := PostSamplingContext{
				SessionID:     sessionID,
				Source:        SamplingSourceMain,
				AssistantMsg:  assistantMsg,
				History:       deepCopyMessages(currentHistory),
				ToolCallCount: totalToolCalls, // Fix 3: use cumulative count
				Snapshot:      latestSnapshot,
				StartedAt:     generationStartedAt, // Fix 3: set start time
				FinishedAt:    time.Now(),
			}
			a.postSamplingRegistry.ExecuteAsync(ctx, psCtx)
		}

		return AgentEvent{
			Type:           AgentEventTypeResponse,
			SessionID:      sessionID,
			Message:        assistantMsg,
			Done:           true,
			TerminalReason: ReasonCompleted,
		}
	}
}

func buildToolOutcomes(assistantMsg message.Message, toolMsg message.Message) []ToolOutcome {
	byID := make(map[string]message.ToolCall)
	for _, tc := range assistantMsg.ToolCalls() {
		byID[tc.ID] = tc
	}
	results := toolMsg.ToolResults()
	outcomes := make([]ToolOutcome, 0, len(results))
	for _, tr := range results {
		out := ToolOutcome{
			ToolCallID:   tr.ToolCallID,
			IsError:      tr.IsError,
			ResultBytes:  len(strings.TrimSpace(tr.Content)),
			ContentEmpty: strings.TrimSpace(tr.Content) == "",
		}
		if tc, ok := byID[tr.ToolCallID]; ok {
			out.ToolName = tc.Name
			out.QueryKey = tc.Input
		}
		if tr.Metadata != "" {
			var md map[string]any
			if err := json.Unmarshal([]byte(tr.Metadata), &md); err == nil {
				out.Provider, _ = md["provider"].(string)
				out.Source, _ = md["source"].(string)
				out.Action, _ = md["action"].(string)
				out.ProgressKind, _ = md["progress_kind"].(string)
				out.ErrorKind, _ = md["error_kind"].(string)
				if qk, _ := md["query_key"].(string); qk != "" {
					out.QueryKey = qk
				}
				out.TargetKey, _ = md["target_key"].(string)
				out.CanonicalURL, _ = md["canonical_url"].(string)
				out.ContentClass, _ = md["content_class"].(string)
				out.PromptClass, _ = md["prompt_class"].(string)
				out.OutcomeHash, _ = md["outcome_hash"].(string)
				out.LowValueReason, _ = md["low_value_reason"].(string)
				out.EvidenceKeys = collectStringArray(md, "evidence_keys")
				out.Recoverable, _ = md["recoverable"].(bool)
				out.DurableProgress = boolMetadata(md, "durable_progress")
				out.ArtifactPaths = collectArtifactPaths(md)
			}
		}
		if !out.IsError && isDurableWriterTool(out.ToolName) && (len(out.ArtifactPaths) > 0 || !out.ContentEmpty) {
			out.DurableProgress = true
		}
		if isNonDurableProgressKind(out.ProgressKind) {
			out.DurableProgress = false
		}
		outcomes = append(outcomes, out)
	}
	return outcomes
}

func collectStringArray(md map[string]any, key string) []string {
	raw, ok := md[key]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func isNonDurableProgressKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "rate_limited", "not_found", "too_large", "search_page", "none":
		return true
	default:
		return false
	}
}

func boolMetadata(md map[string]any, key string) bool {
	v, ok := md[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func collectArtifactPaths(md map[string]any) []string {
	out := make([]string, 0, 8)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		for _, existing := range out {
			if existing == v {
				return
			}
		}
		out = append(out, v)
	}
	for _, key := range []string{"path", "svg_path", "png_path", "d2_path"} {
		if v, _ := md[key].(string); v != "" {
			add(v)
		}
	}
	if raw, ok := md["artifact_paths"]; ok {
		if arr, ok := raw.([]any); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					add(s)
				}
			}
		}
	}
	return out
}

func isDurableWriterTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "write", "edit", "diagramgen", "imagegen", "docexport":
		return true
	default:
		return false
	}
}

func semanticToolSignature(tc message.ToolCall, outcomes []ToolOutcome) string {
	for _, out := range outcomes {
		if out.ToolCallID != tc.ID || out.ToolName != tc.Name {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(tc.Name)) {
		case "webfetch":
			target := strings.TrimSpace(out.TargetKey)
			if target == "" {
				target = strings.TrimSpace(out.CanonicalURL)
			}
			if target != "" {
				return "WebFetch:" + strings.ToLower(target)
			}
		case "websearch":
			if q, d := parseSearchQueryAndDomains(tc.Input); q != "" {
				return "WebSearch:" + q + ":" + d
			}
		case "scholarsearch":
			providerName := strings.ToLower(strings.TrimSpace(out.Provider))
			if providerName == "" {
				providerName = strings.ToLower(strings.TrimSpace(out.Source))
			}
			action := strings.ToLower(strings.TrimSpace(out.Action))
			if action == "" {
				action = "search"
			}
			if out.ErrorKind == "rate_limited" || out.ErrorKind == "provider_cooldown" {
				return "ScholarSearch:" + providerName + ":" + action + ":" + strings.ToLower(strings.TrimSpace(out.ErrorKind))
			}
			qk := strings.ToLower(strings.TrimSpace(out.QueryKey))
			if qk != "" {
				return "ScholarSearch:" + providerName + ":" + action + ":" + qk
			}
		}
		break
	}
	return tc.Name + ":" + toolInputHash(tc.Input)
}

func parseSearchQueryAndDomains(raw string) (string, string) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", ""
	}
	query := strings.ToLower(strings.Join(collectStringArrayFromAny(payload["query"]), " "))
	domains := append(collectStringArrayFromAny(payload["domains"]), collectStringArrayFromAny(payload["allowed_domains"])...)
	blocked := collectStringArrayFromAny(payload["blocked_domains"])
	domains = normalizeSignatureParts(domains)
	blocked = normalizeSignatureParts(blocked)
	query = strings.Join(strings.Fields(query), " ")
	return query, strings.Join(domains, ",") + "|blocked:" + strings.Join(blocked, ",")
}

func collectStringArrayFromAny(raw any) []string {
	switch v := raw.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func normalizeSignatureParts(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func newGenerationStopController(qcfg QueryConfig) *StopController {
	sc := NewStopController(NewMaxIterationHook(defaultMaxIterationHook))
	sc.AddPolicy(NewToolCallLimitHook(qcfg.Loop.MaxToolCalls))
	sc.AddPolicy(NewSearchConvergenceBudgetHook())
	sc.AddPolicy(NewRepeatedInspectionTargetHook())
	sc.AddPolicy(NewRepeatedToolPatternHook())
	sc.AddPolicy(NewProviderCooldownLoopHook(2))
	sc.AddPolicy(NewNoProgressHook(6))
	return sc
}

func remainingBudget(maxBudget, used int) int {
	if maxBudget <= 0 {
		return -1
	}
	return maxBudget - used
}

func loopDiagnostics(state *LoopState, qcfg QueryConfig) map[string]any {
	if state == nil {
		return nil
	}
	return map[string]any{
		"iteration":                          state.Iteration,
		"tool_calls":                         state.TotalToolCalls,
		"max_tool_calls":                     qcfg.Loop.MaxToolCalls,
		"search_tool_calls":                  state.SearchToolCalls,
		"max_search_tool_calls":              defaultMaxSearchToolCalls,
		"soft_search_tool_calls":             defaultSoftSearchToolCalls,
		"last_round_tool_calls":              state.LastRoundProgress.ToolCalls,
		"last_round_durable_progress":        state.LastRoundProgress.DurableProgress,
		"last_round_new_evidence":            state.LastRoundProgress.NewEvidenceCount,
		"last_round_low_value_inspection":    state.LastRoundProgress.LowValueInspection,
		"provider_cooldown_consecutive":      state.ProviderCooldownConsecutive,
		"provider_cooldown_last_scope":       state.ProviderCooldownLastScope,
		"last_round_signatures":              state.LastRoundToolSignatures,
		"seen_evidence_count":                len(state.SeenEvidence),
		"tracked_targets":                    len(state.TargetCounts),
		"tracked_outcomes":                   len(state.OutcomeCounts),
		"tracked_failure_signatures":         len(state.FailureCounts),
		"remaining_tool_calls":               remainingBudget(qcfg.Loop.MaxToolCalls, state.TotalToolCalls),
		"remaining_search_tool_calls":        remainingBudget(defaultMaxSearchToolCalls, state.SearchToolCalls),
		"finalization_attempted_on_continue": false,
	}
}

func (a *agent) finalizeWithoutTools(ctx context.Context, sessionID string, history []message.Message, decision StopDecision) (message.Message, string) {
	nudge := message.Message{
		Role: message.User,
		Parts: []message.ContentPart{message.TextContent{
			Text: "External search/fetch budget has reached a stop policy. Do not call tools. Provide the best final answer from existing evidence, coverage limits, failures, uncertainty, and next steps.",
		}},
	}
	resp, err := a.providerForRequest(ctx).SendMessages(ctx, append(history, nudge), nil)
	if err != nil || resp == nil || strings.TrimSpace(resp.Content) == "" {
		warningText := "Unable to complete tool-free finalization after stop policy. Please retry or narrow the request."
		msg, _ := a.messages.Create(context.Background(), sessionID, message.CreateMessageParams{
			Role:  message.Assistant,
			Parts: []message.ContentPart{message.TextContent{Text: warningText}, message.Finish{Reason: message.FinishReasonEndTurn}},
		})
		return msg, warningForStopDecision(decision)
	}
	msg, err := a.messages.Create(context.Background(), sessionID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: strings.TrimSpace(resp.Content)}, message.Finish{Reason: message.FinishReasonEndTurn}},
	})
	if err != nil {
		return message.Message{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: strings.TrimSpace(resp.Content)}}}, warningForStopDecision(decision)
	}
	return msg, warningForStopDecision(decision)
}

func (a *agent) streamAndHandleEvents(ctx context.Context, sessionID string, msgHistory []message.Message) (message.Message, *message.Message, error) {
	ctx = context.WithValue(ctx, tools.SessionIDContextKey, sessionID)
	if a.permissionService != nil && sessionID != "" {
		ctx = permission.WithMode(ctx, a.permissionService.SessionMode(sessionID))
	}
	if a.readStateManager != nil {
		ctx = context.WithValue(ctx, tools.ReadStateContextKey, a.readStateManager.Get(sessionID))
	}
	ctx = context.WithValue(ctx, tools.ReadLimitsContextKey, tools.ReadLimits{
		DefaultLines: 2000,
		MaxLineChars: 2000,
		MaxSizeBytes: 4 * 1024 * 1024,
	})

	// Phase 5 Fix 2: inject MagicDoc file read notifier
	if a.fileReadNotifier != nil {
		ctx = context.WithValue(ctx, tools.FileReadNotifierContextKey, &magicDocBridge{onRead: a.fileReadNotifier})
	}
	if a.fileToolUsageNotifier != nil {
		ctx = context.WithValue(ctx, tools.FileToolUsageNotifierContextKey, &fileToolUsageBridge{onUse: a.fileToolUsageNotifier})
	}

	reqView := a.buildRequestView(ctx, sessionID, msgHistory)
	ctx = provider.WithRequestSystemPrompt(ctx, provider.SystemPrompt{
		Message: reqView.SystemMessage,
		Blocks:  promptBlocksToProvider(reqView.SystemBlocks),
	})
	msgHistory = reqView.Messages
	activeTools := reqView.ActiveTools
	model := a.modelForRequest(ctx)
	qcfg := BuildQueryConfig(sessionID, string(model.ID), models.RuntimeContextWindow(model), model.DefaultMaxTokens, config.Get())
	preflight := a.preflightRequestView(ctx, reqView, qcfg, false)
	if preflight.Exhausted {
		return message.Message{}, nil, errCompactExhausted
	}
	preflightMsgs := preflight.Messages

	// Debug: log LLM request and store start time in context
	if dl := debugLoggerFromCtx(ctx); dl != nil {
		ctx = context.WithValue(ctx, llmStartTimeCtxKey, time.Now())
		dl.LogLLMRequest(debug.LLMRequestData{
			Model:        string(model.ID),
			MessageCount: len(preflightMsgs),
			ToolCount:    len(activeTools),
		})
	}

	assistantMsg, toolResults, err := a.streamOnce(ctx, sessionID, preflightMsgs, activeTools)
	if err == nil {
		return assistantMsg, toolResults, nil
	}
	if !provider.IsPromptTooLong(err) {
		return assistantMsg, toolResults, err
	}
	if assistantMsg.ID != "" {
		_ = a.messages.Delete(context.Background(), assistantMsg.ID)
	}
	reqView.Messages = msgHistory
	retryPreflight := a.preflightRequestView(ctx, reqView, qcfg, true)
	if retryPreflight.Exhausted {
		return message.Message{}, nil, errCompactExhausted
	}
	retryMsgs := retryPreflight.Messages
	retryAssistantMsg, retryToolResults, retryErr := a.streamOnce(ctx, sessionID, retryMsgs, activeTools)
	if retryErr == nil {
		return retryAssistantMsg, retryToolResults, nil
	}
	return retryAssistantMsg, retryToolResults, fmt.Errorf("%w (retry after forced compaction failed: %v)", err, retryErr)
}

func (a *agent) streamOnce(ctx context.Context, sessionID string, msgHistory []message.Message, activeTools []tools.BaseTool) (message.Message, *message.Message, error) {
	eventChan := a.providerForRequest(ctx).StreamResponse(ctx, msgHistory, activeTools)

	assistantMsg, err := a.messages.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{},
		Model: a.modelForRequest(ctx).ID,
	})
	if err != nil {
		return assistantMsg, nil, fmt.Errorf("failed to create assistant message: %w", err)
	}

	ctx = context.WithValue(ctx, tools.MessageIDContextKey, assistantMsg.ID)

	const persistInterval = 100 * time.Millisecond
	persistTimer := time.NewTicker(persistInterval)
	defer persistTimer.Stop()
	persistDirty := false

eventLoop:
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				// Stream ended: flush any remaining changes
				if persistDirty {
					a.messages.Update(ctx, assistantMsg)
				}
				break eventLoop
			}

			keyEvent, err := a.processEvent(ctx, &assistantMsg, event)
			if err != nil {
				assistantMsg.CleanIncompleteToolCalls()
				assistantMsg.SanitizeTextContent(provider.SanitizeContentDelta)
				assistantMsg.AddFinish(message.FinishReasonCanceled)
				a.messages.Update(ctx, assistantMsg)
				return assistantMsg, nil, err
			}
			if keyEvent {
				persistDirty = false // key events already persisted in processEvent
			} else {
				persistDirty = true
			}

			if ctx.Err() != nil {
				assistantMsg.CleanIncompleteToolCalls()
				assistantMsg.SanitizeTextContent(provider.SanitizeContentDelta)
				assistantMsg.AddFinish(message.FinishReasonCanceled)
				a.messages.Update(context.Background(), assistantMsg)
				return assistantMsg, nil, ctx.Err()
			}

		case <-persistTimer.C:
			if persistDirty {
				if err := a.messages.Update(ctx, assistantMsg); err != nil {
					// Non-fatal: log and continue, next tick will retry
					continue
				}
				persistDirty = false
			}

		case <-ctx.Done():
			if persistDirty {
				a.messages.Update(context.Background(), assistantMsg)
			}
			assistantMsg.CleanIncompleteToolCalls()
			assistantMsg.SanitizeTextContent(provider.SanitizeContentDelta)
			assistantMsg.AddFinish(message.FinishReasonCanceled)
			a.messages.Update(context.Background(), assistantMsg)
			return assistantMsg, nil, ctx.Err()
		}
	}

	// Log assistant thinking and content
	if dl := debugLoggerFromCtx(ctx); dl != nil {
		for _, part := range assistantMsg.Parts {
			if rc, ok := part.(message.ReasoningContent); ok && rc.Thinking != "" {
				dl.LogAssistantThinking(debug.AssistantThinkingData{Thinking: rc.Thinking})
			}
		}
		if tc := assistantMsg.Content(); tc.Text != "" {
			dl.LogAssistantMessage(debug.AssistantMessageData{
				Content:      tc.Text,
				FinishReason: string(assistantMsg.FinishReason()),
			})
		}
	}

	return a.executeToolCalls(ctx, assistantMsg)
}

type preflightRequestResult struct {
	Messages  []message.Message
	Snapshot  llmcontext.BudgetSnapshot
	Compacted bool
	Exhausted bool
}

func (a *agent) preflightRequestView(ctx context.Context, view RequestView, qcfg QueryConfig, force bool) preflightRequestResult {
	promptTokens, estimate := a.preflightPromptTokens(ctx, view)
	snapshot := llmcontext.BuildBudgetSnapshot(llmcontext.BudgetSnapshotInput{
		ContextWindow:   int64(qcfg.EffectiveContextWindow + qcfg.Compact.ReserveOutputTokens),
		MaxOutputTokens: int64(qcfg.Compact.ReserveOutputTokens),
		ReserveTokens:   int64(qcfg.Compact.ReserveOutputTokens),
		PromptTokens:    promptTokens,
		Estimate:        estimate,
		WarningAt:       int64(max(qcfg.EffectiveContextWindow-qcfg.Compact.WarningBufferTokens, 0)),
		AutoCompactAt:   int64(qcfg.AutoCompactThreshold),
		HardStopAt:      int64(qcfg.EffectiveContextWindow),
	})

	overAutoCompact := snapshot.AutoCompactAt > 0 && snapshot.EffectiveTokens > snapshot.AutoCompactAt
	overHardStop := snapshot.HardStopAt > 0 && snapshot.EffectiveTokens >= snapshot.HardStopAt
	needsCompact := overAutoCompact || overHardStop
	if !force && !needsCompact {
		return preflightRequestResult{Messages: view.Messages, Snapshot: snapshot}
	}
	keepRounds := 5
	if force {
		keepRounds = 2
	} else if overHardStop {
		keepRounds = 3
	}
	compactedMessages := compactRequestViewMessages(view, keepRounds)
	if len(compactedMessages) == 0 {
		return preflightRequestResult{
			Messages:  view.Messages,
			Snapshot:  snapshot,
			Compacted: false,
			Exhausted: qcfg.BlockingCompactThreshold > 0 && snapshot.EffectiveTokens > int64(qcfg.BlockingCompactThreshold),
		}
	}
	if qcfg.SessionID != "" && a.Broker != nil {
		a.Publish(pubsub.CreatedEvent, AgentEvent{Type: AgentEventTypeCompacting, SessionID: qcfg.SessionID})
	}
	compactedView := view
	compactedView.Messages = compactedMessages
	compactedView.PrefixMessageCount = min(view.PrefixMessageCount, len(compactedMessages))
	promptTokens, estimate = a.preflightPromptTokens(ctx, compactedView)
	snapshot = llmcontext.BuildBudgetSnapshot(llmcontext.BudgetSnapshotInput{
		ContextWindow:   int64(qcfg.EffectiveContextWindow + qcfg.Compact.ReserveOutputTokens),
		MaxOutputTokens: int64(qcfg.Compact.ReserveOutputTokens),
		ReserveTokens:   int64(qcfg.Compact.ReserveOutputTokens),
		PromptTokens:    promptTokens,
		Estimate:        estimate,
		WarningAt:       int64(max(qcfg.EffectiveContextWindow-qcfg.Compact.WarningBufferTokens, 0)),
		AutoCompactAt:   int64(qcfg.AutoCompactThreshold),
		HardStopAt:      int64(qcfg.EffectiveContextWindow),
	})
	if qcfg.SessionID != "" && a.Broker != nil {
		a.Publish(pubsub.CreatedEvent, AgentEvent{Type: AgentEventTypeCompactDone, SessionID: qcfg.SessionID})
	}
	return preflightRequestResult{
		Messages:  compactedMessages,
		Snapshot:  snapshot,
		Compacted: true,
		Exhausted: qcfg.BlockingCompactThreshold > 0 && snapshot.EffectiveTokens > int64(qcfg.BlockingCompactThreshold),
	}
}

func compactRequestViewMessages(view RequestView, keepRounds int) []message.Message {
	prefixCount := min(max(view.PrefixMessageCount, 0), len(view.Messages))
	prefix := deepCopyMessages(view.Messages[:prefixCount])
	compacted := SessionCompact(view.Messages[prefixCount:], keepRounds, "")
	result := make([]message.Message, 0, len(prefix)+len(compacted.Messages))
	result = append(result, prefix...)
	result = append(result, compacted.Messages...)
	return result
}

func (a *agent) preflightPromptTokens(ctx context.Context, view RequestView) (int64, llmcontext.TokenCountEstimate) {
	estimate := llmcontext.TokenCountWithEstimation(view.Messages)
	promptTokens := estimate.TokenCount
	if tc, err := a.providerForRequest(ctx).CountTokens(ctx, view.Messages, view.ActiveTools); err == nil {
		promptTokens = tc.InputTokens
	} else {
		promptTokens = estimateFullRequestMessages(view.Messages) + estimateRequestOverhead(view)
	}
	return promptTokens, estimate
}

func estimateFullRequestMessages(msgs []message.Message) int64 {
	var tokens int64
	for _, msg := range msgs {
		tokens += llmcontext.EstimateMessageTokens(msg)
	}
	return tokens
}

func estimateRequestOverhead(view RequestView) int64 {
	var tokens int64
	for _, block := range view.SystemBlocks {
		tokens += int64((len(block.Text) + 3) / 4)
	}
	for _, tool := range view.ActiveTools {
		info := tool.Info()
		tokens += int64((len(info.Name) + len(info.Description) + len(fmt.Sprintf("%v", info.Parameters)) + 3) / 4)
	}
	return tokens
}

func (a *agent) processEvent(ctx context.Context, assistantMsg *message.Message, event provider.ProviderEvent) (keyEvent bool, err error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	switch event.Type {
	case provider.EventThinkingDelta:
		assistantMsg.AppendReasoningContent(event.Thinking)
		return false, nil // 不立即持久化
	case provider.EventContentDelta:
		assistantMsg.AppendContent(event.Content)
		return false, nil // 不立即持久化
	case provider.EventToolUseStart:
		assistantMsg.AddToolCall(*event.ToolCall)
		if err := a.messages.Update(ctx, *assistantMsg); err != nil {
			return true, err
		}
		return true, nil // 关键事件，立即持久化
	case provider.EventToolUseDelta:
		if event.ToolCall != nil {
			assistantMsg.AppendToolInput(event.ToolCall.ID, event.ToolCall.Input)
		}
		return false, nil // 不立即持久化
	case provider.EventToolUseStop:
		// Mark input as fully received from the stream. Do NOT set State=completed here;
		// the actual execution state is managed by executeToolCalls.
		assistantMsg.MarkToolCallInputDone(event.ToolCall.ID)
		if err := a.messages.Update(ctx, *assistantMsg); err != nil {
			return true, err
		}
		return true, nil // 关键事件，立即持久化
	case provider.EventError:
		if errors.Is(event.Error, context.Canceled) {
			return true, context.Canceled
		}
		return true, event.Error
	case provider.EventComplete:
		assistantMsg.SetToolCalls(event.Response.ToolCalls)
		// 计算有效 finish reason：模型请求 tool_use 但规范化后没有有效 tool call 时转 error
		effectiveFinish := event.Response.FinishReason
		if effectiveFinish == message.FinishReasonToolUse && len(assistantMsg.ToolCalls()) == 0 {
			effectiveFinish = message.FinishReasonError
			assistantMsg.AppendContent("\n\n[System: All tool calls had invalid parameters and were discarded.]")
		}
		assistantMsg.AddFinish(effectiveFinish)
		assistantMsg.Usage = llmcontext.UsageFromProvider(event.Response.Usage)
		// 兜底净化：清除流式过程中可能漏过的 Provider 标签
		assistantMsg.SanitizeTextContent(provider.SanitizeContentDelta)
		if err := a.messages.Update(ctx, *assistantMsg); err != nil {
			return true, fmt.Errorf("failed to update message: %w", err)
		}
		return true, a.trackUsage(ctx, getSessionID(ctx), event.Response.Usage, effectiveFinish)
	}
	return false, nil
}

// magicDocBridge adapts magicdoc.Service to tools.FileReadNotifier interface.
type magicDocBridge struct {
	onRead func(sessionID, filePath, content string)
}

func (b *magicDocBridge) OnFileRead(sessionID, filePath, content string) {
	if b.onRead != nil {
		b.onRead(sessionID, filePath, content)
	}
}

type fileToolUsageBridge struct {
	onUse func(sessionID string, evt tools.FileToolUsageEvent)
}

func (b *fileToolUsageBridge) OnFileToolUsage(sessionID string, evt tools.FileToolUsageEvent) {
	if b.onUse != nil {
		b.onUse(sessionID, evt)
	}
}

// checkpointBridge adapts CheckpointStore to tools.WriteCheckpointer interface
// to avoid import cycles (tools cannot import agent).
type checkpointBridge struct {
	store CheckpointStore
}

func (b *checkpointBridge) CaptureBeforeWrite(ctx context.Context, sessionID, turnID, filePath string) error {
	_, err := b.store.CaptureBeforeWrite(ctx, sessionID, turnID, filePath)
	return err
}

func getSessionID(ctx context.Context) string {
	if sid, ok := ctx.Value(tools.SessionIDContextKey).(string); ok {
		return sid
	}
	return ""
}

func memoryItemsToEntries(items []memory.MemoryItem) []modules.MemoryEntry {
	entries := make([]modules.MemoryEntry, len(items))
	for i, m := range items {
		entries[i] = modules.MemoryEntry{
			Content:   m.Content,
			CreatedAt: m.CreatedAt,
		}
	}
	return entries
}

// toolInputHash returns a short hash of the tool input for signature fingerprinting.
// JSON objects are re-marshaled first so insignificant key ordering does not
// create different fallback signatures.
func toolInputHash(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return "empty"
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		if b, err := json.Marshal(v); err == nil {
			s = string(b)
		}
	}
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8])
}
