package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/debug"
	llmcontext "github.com/Nahasma/openscholar-public/internal/llm/context"
	"github.com/Nahasma/openscholar-public/internal/llm/provider"
	"github.com/Nahasma/openscholar-public/internal/message"
)

func debugLoggerFromCtx(ctx context.Context) *debug.SessionLogger {
	if dl, ok := ctx.Value(debugLoggerCtxKey).(*debug.SessionLogger); ok {
		return dl
	}
	return nil
}

// lastUsage holds the most recent API usage for the current generation loop.
// Updated by trackUsage, consumed by processGeneration to update LoopState.
type lastUsage struct {
	InputTokens  int64
	OutputTokens int64
	CacheRead    int64
	CacheCreate  int64
	Cost         float64
	CostKnown    bool
}

func (a *agent) trackUsage(ctx context.Context, sessID string, usage provider.TokenUsage, finishReason message.FinishReason) error {
	if sessID == "" {
		return nil
	}
	sess, err := a.sessions.Get(ctx, sessID)
	if err != nil {
		return nil
	}

	model := a.modelForRequest(ctx)
	costKnown := model.CostKnown
	cost := 0.0
	if costKnown {
		cost = model.CostPer1MInCached/1e6*float64(usage.CacheCreationTokens) +
			model.CostPer1MOutCached/1e6*float64(usage.CacheReadTokens) +
			model.CostPer1MIn/1e6*float64(usage.InputTokens) +
			model.CostPer1MOut/1e6*float64(usage.OutputTokens)
	}

	currentUsage := llmcontext.UsageFromProvider(usage)
	a.lastInputTokens.Store(llmcontext.CurrentUsageTokens(currentUsage))
	a.contextState.Store(ContextSnapshot{
		CurrentUsage:              currentUsage,
		LastResponseContextTokens: llmcontext.FullResponseTokenCount(currentUsage),
		LastResponseOutputTokens:  currentUsage.OutputTokens,
	})

	// C2: record per-model cost (legacy tracker)
	if a.costTracker != nil {
		a.costTracker.RecordWithCostKnown(
			string(model.ID),
			usage.InputTokens,
			usage.OutputTokens,
			usage.CacheReadTokens,
			usage.CacheCreationTokens,
			cost,
			costKnown,
		)
	}

	// Loop Governance: store last usage for LoopState update in processGeneration
	a.lastUsageData = lastUsage{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		CacheRead:    usage.CacheReadTokens,
		CacheCreate:  usage.CacheCreationTokens,
		Cost:         cost,
		CostKnown:    costKnown,
	}

	sess.Cost += cost
	// Unified token accounting: PromptEffective = input + cacheRead + cacheCreation
	sess.PromptTokens = llmcontext.CurrentUsageTokens(currentUsage)
	sess.CompletionTokens = usage.OutputTokens

	// Debug: log LLM response with timing
	if dl := debugLoggerFromCtx(ctx); dl != nil {
		var latencyMs int64
		if start, ok := ctx.Value(llmStartTimeCtxKey).(time.Time); ok {
			latencyMs = time.Since(start).Milliseconds()
		}
		dl.LogLLMResponse(debug.LLMResponseData{
			Model:               string(model.ID),
			InputTokens:         usage.InputTokens,
			OutputTokens:        usage.OutputTokens,
			CacheCreationTokens: usage.CacheCreationTokens,
			CacheReadTokens:     usage.CacheReadTokens,
			LatencyMs:           latencyMs,
			Cost:                cost,
			CostKnown:           costKnown,
			FinishReason:        string(finishReason),
		})
	}

	if usage.CacheCreationTokens > 0 || usage.CacheReadTokens > 0 {
		slog.Debug("token usage",
			"session", sessID,
			"input", usage.InputTokens,
			"output", usage.OutputTokens,
			"cache_write", usage.CacheCreationTokens,
			"cache_read", usage.CacheReadTokens,
			"cost", formatCostForLog(cost, costKnown),
		)
	} else {
		slog.Debug("token usage",
			"session", sessID,
			"input", usage.InputTokens,
			"output", usage.OutputTokens,
			"cost", formatCostForLog(cost, costKnown),
		)
	}

	_, err = a.sessions.Save(ctx, sess)
	return err
}

func formatCostForLog(cost float64, known bool) string {
	if !known {
		return "unknown"
	}
	return fmt.Sprintf("$%.4f", cost)
}

func (a *agent) setTitleFromFirstMessage(ctx context.Context, sessionID string, content string) {
	if content == "" {
		return
	}
	sess, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return
	}
	// Use first line of user message as title
	title := strings.TrimSpace(content)
	if idx := strings.IndexAny(title, "\n\r"); idx >= 0 {
		title = title[:idx]
	}
	const maxTitleLen = 80
	if len(title) > maxTitleLen {
		title = title[:maxTitleLen] + "..."
	}
	if title == "" {
		return
	}
	sess.Title = title
	a.sessions.Save(ctx, sess)
}

func (a *agent) errEvent(err error) AgentEvent {
	return AgentEvent{Type: AgentEventTypeError, Error: err}
}

// terminalEvent creates a terminal AgentEvent with a structured reason.
// Use this instead of errEvent for all loop exit points.
func (a *agent) terminalEvent(reason TerminalReason, err error) AgentEvent {
	return AgentEvent{
		Type:           AgentEventTypeError,
		Error:          err,
		Done:           true,
		TerminalReason: reason,
	}
}
