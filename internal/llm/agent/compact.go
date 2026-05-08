package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/debug"
	"github.com/Nahasma/openscholar-public/internal/hooks"
	"github.com/Nahasma/openscholar-public/internal/llm/provider"
	"github.com/Nahasma/openscholar-public/internal/message"
)

// estimateHistoryTokens returns a rough token count for the message history.
// Uses the heuristic: total character count / 4.
func estimateHistoryTokens(msgs []message.Message) int {
	totalChars := 0
	for _, msg := range msgs {
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case message.TextContent:
				totalChars += len(p.Text)
			case message.ToolCall:
				totalChars += len(p.Input)
			case message.ToolResult:
				totalChars += len(p.Content)
			case message.ReasoningContent:
				totalChars += len(p.Thinking)
			}
		}
	}
	return totalChars / 4
}

func historyAfterSummaryBoundary(msgs []message.Message, summaryMsgID string) []message.Message {
	if summaryMsgID == "" {
		return msgs
	}
	for i, msg := range msgs {
		if msg.ID == summaryMsgID {
			trimmed := deepCopyMessages(msgs[i:])
			if len(trimmed) > 0 {
				trimmed[0].Role = message.User
			}
			return trimmed
		}
	}
	return msgs
}

// compactAndRebuildHistory implements a waterfall compaction strategy:
//   - L2 (SessionCompact): fast, in-memory round-based trimming — no LLM call, no persistence.
//   - L3 (LLM summary): full summarization via summarizerProvider — persistent.
//
// If L2 brings the token count below the threshold, it returns immediately.
// Otherwise it falls through to L3.
// Returns new msgHistory on success, or original msgs on failure.
func (a *agent) compactAndRebuildHistory(ctx context.Context, sessionID string, msgs []message.Message, _ QueryConfig) []message.Message {
	if err := a.compact(ctx, sessionID, msgs, ""); err != nil {
		return msgs
	}
	newMsgs, err := a.messages.List(ctx, sessionID)
	if err != nil {
		return msgs
	}
	// Reload session to get updated SummaryMessageID
	sess, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return msgs
	}
	return historyAfterSummaryBoundary(newMsgs, sess.SummaryMessageID)
}

// compact summarizes old messages and updates the session's SummaryMessageID.
// PC3: increments compactFailures on error, resets to 0 on success.
// Phase 5: fires PreCompact/PostCompact lifecycle hooks.
func (a *agent) compact(ctx context.Context, sessionID string, msgs []message.Message, focus string) error {
	if a.summarizerProvider == nil {
		return fmt.Errorf("no summarizer provider")
	}

	// Phase 5: PreCompact lifecycle hook (blocking)
	if a.hookService != nil {
		_ = a.hookService.RunBlocking(ctx, hooks.PreCompact, hooks.Input{
			SessionID: sessionID,
			Timestamp: time.Now(),
		})
	}

	// Debug: log compact event
	if dl := debugLoggerFromCtx(ctx); dl != nil {
		dl.LogCompact(debug.CompactData{
			MessagesBefore: len(msgs),
			TokensBefore:   estimateHistoryTokens(msgs),
		})
	}

	// Build summary content from all messages
	var sb strings.Builder
	for _, msg := range msgs {
		sb.WriteString(fmt.Sprintf("[%s]: ", msg.Role))
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case message.TextContent:
				sb.WriteString(p.Text)
			case message.ToolCall:
				sb.WriteString(fmt.Sprintf("[Tool: %s(%s)]", p.Name, truncateForSummary(p.Input, 200)))
			case message.ToolResult:
				sb.WriteString(fmt.Sprintf("[Result: %s]", truncateForSummary(p.Content, 200)))
			}
		}
		sb.WriteString("\n")
	}

	promptText := sb.String()
	if strings.TrimSpace(focus) != "" {
		promptText = fmt.Sprintf("[Preserve Focus]\n%s\n\n%s", focus, promptText)
	}

	resp, err := a.summarizerProvider.SendMessages(ctx, []message.Message{{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: promptText}},
	}}, nil)
	if err != nil {
		// PC3: track failure
		a.compactFailures++
		if a.compactFailures >= 3 {
			slog.Warn("auto-compact fused after 3 consecutive failures", "session", sessionID)
		}
		return fmt.Errorf("summarization failed: %w", err)
	}

	// M1: extract structured <summary> block from summarizer output
	summary := extractSummaryBlock(strings.TrimSpace(resp.Content))
	if summary == "" {
		a.compactFailures++
		if a.compactFailures >= 3 {
			slog.Warn("auto-compact fused after 3 consecutive failures", "session", sessionID)
		}
		return fmt.Errorf("empty summary")
	}

	// Create a summary message
	summaryMsg, err := a.messages.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "[Conversation Summary]\nThis session is being continued from a compacted conversation.\n" + summary}},
		Meta: map[string]any{
			"compact_boundary": true,
			"focus":            strings.TrimSpace(focus),
		},
	})
	if err != nil {
		a.compactFailures++
		if a.compactFailures >= 3 {
			slog.Warn("auto-compact fused after 3 consecutive failures", "session", sessionID)
		}
		return fmt.Errorf("failed to create summary message: %w", err)
	}

	// Update session
	sess, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		a.compactFailures++
		if a.compactFailures >= 3 {
			slog.Warn("auto-compact fused after 3 consecutive failures", "session", sessionID)
		}
		return err
	}
	sess.SummaryMessageID = summaryMsg.ID
	_, err = a.sessions.Save(ctx, sess)
	if err != nil {
		a.compactFailures++
		if a.compactFailures >= 3 {
			slog.Warn("auto-compact fused after 3 consecutive failures", "session", sessionID)
		}
		return err
	}

	// Unified post-compact reset (failure counter + token tracking + cache monitor)
	a.onCompactSuccess()

	// Phase 5: PostCompact lifecycle hook (async, only on success)
	if a.hookService != nil {
		a.hookService.RunAsync(ctx, hooks.PostCompact, hooks.Input{
			SessionID: sessionID,
			Timestamp: time.Now(),
		})
	}

	return nil
}

// onCompactSuccess resets all compact-related state after a successful compaction.
// Called by both L2 (SessionCompact) and L3 (LLM summary) paths.
func (a *agent) onCompactSuccess() {
	a.compactFailures = 0
	a.lastInputTokens.Store(0) // force next check to fall back to estimate
	a.lastUsageData = lastUsage{}
	a.contextState.Store(ContextSnapshot{})

	// Notify CacheMonitor so the next API call doesn't trigger a false cache-break warning
	if cap, ok := a.agentProvider.(provider.CacheAwareProvider); ok {
		if cm := cap.CacheMonitor(); cm != nil {
			cm.NotifyCompaction()
			cm.ResetBaseline()
		}
	}
}

// SessionCompactResult holds the output of a SessionCompact operation.
type SessionCompactResult struct {
	Messages      []message.Message
	DroppedRounds int
	DroppedMsgs   int
}

// SessionCompact performs L2 structured trimming: it keeps the most recent
// keepRounds complete conversation rounds and discards older ones.
// A round starts at a user message and includes all subsequent assistant/tool
// messages until the next user message.
// If summaryMsgID is non-empty, the message with that ID is always preserved
// at the front of the result (it represents a prior L3 summary).
// The returned messages are a deep copy; the input slice is never modified.
func SessionCompact(msgs []message.Message, keepRounds int, summaryMsgID string) SessionCompactResult {
	if keepRounds <= 0 {
		keepRounds = 5
	}

	// Find round boundaries (indices of user messages)
	var roundStarts []int
	for i, msg := range msgs {
		if msg.Role == message.User {
			// Skip the summary message — it's not a real round
			if summaryMsgID != "" && msg.ID == summaryMsgID {
				continue
			}
			roundStarts = append(roundStarts, i)
		}
	}

	// If we have fewer rounds than keepRounds, nothing to trim
	if len(roundStarts) <= keepRounds {
		return SessionCompactResult{
			Messages:      deepCopyMessages(msgs),
			DroppedRounds: 0,
			DroppedMsgs:   0,
		}
	}

	// Keep the last keepRounds rounds
	cutoffRoundIdx := len(roundStarts) - keepRounds
	cutoffMsgIdx := roundStarts[cutoffRoundIdx]

	// Build result: optionally prepend summary message, then kept messages
	var result []message.Message
	droppedMsgs := 0

	// Preserve summary message if it exists and falls before the cutoff
	if summaryMsgID != "" {
		for _, msg := range msgs[:cutoffMsgIdx] {
			if msg.ID == summaryMsgID {
				cp := msg
				cp.Parts = make([]message.ContentPart, len(msg.Parts))
				copy(cp.Parts, msg.Parts)
				result = append(result, cp)
				break
			}
		}
	}

	// Count dropped messages (excluding summary if preserved)
	for i := 0; i < cutoffMsgIdx; i++ {
		if summaryMsgID != "" && msgs[i].ID == summaryMsgID {
			continue
		}
		droppedMsgs++
	}

	// Append kept messages (deep copy)
	kept := deepCopyMessages(msgs[cutoffMsgIdx:])
	result = append(result, kept...)

	return SessionCompactResult{
		Messages:      result,
		DroppedRounds: cutoffRoundIdx,
		DroppedMsgs:   droppedMsgs,
	}
}

// extractSummaryBlock extracts the <summary> block from summarizer output.
// Falls back to the full text if no tags are found.
func extractSummaryBlock(text string) string {
	start := strings.Index(text, "<summary>")
	end := strings.Index(text, "</summary>")
	if start >= 0 && end > start {
		return strings.TrimSpace(text[start+len("<summary>") : end])
	}
	return text // fallback: use full text
}

func truncateForSummary(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// MicroCompact returns a deep-copied slice of messages where tool results
// older than keepRecent conversation rounds are replaced with a short
// placeholder.  Error results are always kept (summarised but not dropped).
//
// The original msgs slice is never modified; the returned slice is a
// fresh copy suitable for sending to a provider without persisting.
//
// Returns the compacted slice and the number of parts that were replaced.
func MicroCompact(msgs []message.Message, keepRecent int) ([]message.Message, int) {
	if keepRecent <= 0 {
		keepRecent = 10
	}

	// Build a map: tool-call-id → tool name, so we can label placeholders.
	toolNames := buildToolNameMap(msgs)

	// Identify the cutoff index: messages beyond keepRecent *tool-result
	// messages from the end are candidates for compaction.
	// We count tool-result messages from the end.
	toolResultCount := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		for _, part := range msgs[i].Parts {
			if _, ok := part.(message.ToolResult); ok {
				toolResultCount++
			}
		}
	}

	// How many tool results should be compressed?
	compressCount := toolResultCount - keepRecent
	if compressCount <= 0 {
		// Nothing to compact — return a deep copy anyway.
		return deepCopyMessages(msgs), 0
	}

	result := make([]message.Message, 0, len(msgs))
	replaced := 0
	compressedSoFar := 0

	for _, msg := range msgs {
		newMsg := message.Message{
			ID:        msg.ID,
			Role:      msg.Role,
			SessionID: msg.SessionID,
			Model:     msg.Model,
			CreatedAt: msg.CreatedAt,
			UpdatedAt: msg.UpdatedAt,
			Parts:     make([]message.ContentPart, 0, len(msg.Parts)),
		}

		for _, part := range msg.Parts {
			switch p := part.(type) {
			case message.ToolResult:
				if compressedSoFar < compressCount && !p.IsError {
					// Compress this result
					toolName := toolNames[p.ToolCallID]
					if toolName == "" {
						toolName = "unknown"
					}
					placeholder := fmt.Sprintf("[compacted: tool=%s id=%s]", toolName, p.ToolCallID)
					newMsg.Parts = append(newMsg.Parts, message.ToolResult{
						ToolCallID: p.ToolCallID,
						Name:       p.Name,
						Content:    placeholder,
						IsError:    false,
					})
					compressedSoFar++
					replaced++
				} else if p.IsError {
					// Always keep error results, but summarize if they are in
					// the compression window.
					if compressedSoFar < compressCount {
						toolName := toolNames[p.ToolCallID]
						if toolName == "" {
							toolName = "unknown"
						}
						summary := fmt.Sprintf("[error result kept: tool=%s id=%s] %s",
							toolName, p.ToolCallID, truncateForCompact(p.Content, 300))
						newMsg.Parts = append(newMsg.Parts, message.ToolResult{
							ToolCallID: p.ToolCallID,
							Name:       p.Name,
							Content:    summary,
							IsError:    true,
						})
						compressedSoFar++
						replaced++
					} else {
						newMsg.Parts = append(newMsg.Parts, p)
					}
				} else {
					newMsg.Parts = append(newMsg.Parts, p)
				}
			default:
				newMsg.Parts = append(newMsg.Parts, part)
			}
		}

		result = append(result, newMsg)
	}

	return result, replaced
}

// buildToolNameMap scans msgs and returns a map from tool-call ID to tool name.
// Tool names come from ToolCall parts on assistant messages.
func buildToolNameMap(msgs []message.Message) map[string]string {
	m := make(map[string]string)
	for _, msg := range msgs {
		for _, part := range msg.Parts {
			if tc, ok := part.(message.ToolCall); ok {
				m[tc.ID] = tc.Name
			}
		}
	}
	return m
}

// deepCopyMessages returns a deep copy of msgs (new slice, new Parts slices).
func deepCopyMessages(msgs []message.Message) []message.Message {
	result := make([]message.Message, len(msgs))
	for i, msg := range msgs {
		newMsg := msg
		newMsg.Parts = make([]message.ContentPart, len(msg.Parts))
		copy(newMsg.Parts, msg.Parts)
		result[i] = newMsg
	}
	return result
}

// truncateForCompact truncates s to at most maxLen bytes with an ellipsis.
func truncateForCompact(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
