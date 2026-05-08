package agent

import "github.com/openscholar/openscholar/internal/message"

type AgentEventType string

const (
	AgentEventTypeError       AgentEventType = "error"
	AgentEventTypeResponse    AgentEventType = "response"
	AgentEventTypeCompacting  AgentEventType = "compacting"
	AgentEventTypeCompactDone AgentEventType = "compact_done"

	// Tool lifecycle events — emitted during executeToolCalls.
	AgentEventTypeToolQueued   AgentEventType = "tool_queued"
	AgentEventTypeToolStarted  AgentEventType = "tool_started"
	AgentEventTypeToolFinished AgentEventType = "tool_finished"
	AgentEventTypeToolFailed   AgentEventType = "tool_failed"
	AgentEventTypeToolCanceled AgentEventType = "tool_canceled"
)

// TerminalReason describes why the agent loop terminated.
// Only meaningful when AgentEvent.Done is true.
type TerminalReason string

const (
	ReasonCompleted                  TerminalReason = "completed"         // Normal completion, no more tool calls
	ReasonAbortedTools               TerminalReason = "aborted_tools"     // Unrecoverable tool execution failure
	ReasonPromptTooLong              TerminalReason = "prompt_too_long"   // Context exceeds provider limit
	ReasonMaxTurnsReached            TerminalReason = "max_turns_reached" // Reached maximum iteration count
	ReasonNoProgress                 TerminalReason = "no_progress"
	ReasonRepeatedToolPattern        TerminalReason = "repeated_tool_pattern"
	ReasonTokenBudgetExceeded        TerminalReason = "token_budget_exceeded"
	ReasonCostBudgetExceeded         TerminalReason = "cost_budget_exceeded"
	ReasonLoopHookStopped            TerminalReason = "loop_hook_stopped"
	ReasonPermissionDenied           TerminalReason = "permission_denied"             // User denied required permission
	ReasonStreamError                TerminalReason = "stream_error"                  // Streaming error after retries exhausted
	ReasonUserCancelled              TerminalReason = "user_cancelled"                // Context cancelled by user
	ReasonCompactExhausted           TerminalReason = "compact_exhausted"             // Auto-compact failed repeatedly
	ReasonSessionError               TerminalReason = "session_error"                 // Session read/write failure
	ReasonMaxTokensRecoveryExhausted TerminalReason = "max_tokens_recovery_exhausted" // Output truncated repeatedly
	ReasonProviderCooldown           TerminalReason = "provider_cooldown"
)

// TerminalReasonMessage returns a user-facing status message for the given reason.
func TerminalReasonMessage(r TerminalReason) string {
	switch r {
	case ReasonCompleted:
		return ""
	case ReasonAbortedTools:
		return "Agent stopped: tool execution failed"
	case ReasonPromptTooLong:
		return "Context too long, try /compact"
	case ReasonMaxTurnsReached:
		return "Reached maximum turns limit"
	case ReasonNoProgress:
		return "Agent stopped: no progress detected"
	case ReasonRepeatedToolPattern:
		return "Agent stopped: repeated tool pattern detected"
	case ReasonTokenBudgetExceeded:
		return "Agent stopped: token budget exceeded"
	case ReasonCostBudgetExceeded:
		return "Agent stopped: cost budget exceeded"
	case ReasonLoopHookStopped:
		return "Agent stopped by loop policy"
	case ReasonPermissionDenied:
		return "Permission denied, agent stopped"
	case ReasonStreamError:
		return "Stream error after retries"
	case ReasonUserCancelled:
		return "Cancelled"
	case ReasonCompactExhausted:
		return "Auto-compact failed, try manual /compact"
	case ReasonSessionError:
		return "Session error"
	case ReasonMaxTokensRecoveryExhausted:
		return "Output token limit reached repeatedly"
	case ReasonProviderCooldown:
		return "Semantic Scholar is rate limited; use available results or retry later."
	default:
		return string(r)
	}
}

type AgentEvent struct {
	Type           AgentEventType
	SessionID      string // Session this event belongs to (for TUI filtering)
	Message        message.Message
	Error          error
	Done           bool
	TerminalReason TerminalReason      // Set when Done=true to indicate why the loop ended
	Warning        string              // Non-fatal warning (e.g., unknown model)
	ToolLifecycle  *ToolLifecycleEvent // Non-nil for tool_* event types
}

// ToolLifecycleEvent carries metadata for tool state transitions.
type ToolLifecycleEvent struct {
	ToolCallID    string
	ToolName      string
	BatchID       string
	Input         string
	State         message.ToolCallState
	Order         int   // 0-based position in batch
	Total         int   // total tools in batch
	DurationMs    int64 // available on terminal states
	ResultSummary string
}
