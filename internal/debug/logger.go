package debug

import (
	"context"
	crypto_rand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventType enumerates the kinds of debug log entries.
type EventType string

const (
	EventSessionStart      EventType = "session_start"
	EventSessionEnd        EventType = "session_end"
	EventSessionCompact    EventType = "session_compact"
	EventLLMRequest        EventType = "llm_request"
	EventLLMResponse       EventType = "llm_response"
	EventToolStart         EventType = "tool_start"
	EventToolEnd           EventType = "tool_end"
	EventLoopIteration     EventType = "loop_iteration"
	EventError             EventType = "error"
	EventUserMessage       EventType = "user_message"
	EventAssistantMessage  EventType = "assistant_message"
	EventAssistantThinking EventType = "assistant_thinking"
	EventStopDecision      EventType = "stop_decision"
	EventBudgetDecision    EventType = "budget_decision"
)

// MaxTruncateLen is the default max length for truncating tool input/output in logs.
const MaxTruncateLen = 500

// LogEntry is the base envelope for every JSONL line.
type LogEntry struct {
	Timestamp string    `json:"ts"`
	SessionID string    `json:"session_id"`
	Type      EventType `json:"type"`
	AgentName string    `json:"agent,omitempty"`
	SubAgent  bool      `json:"sub_agent,omitempty"`
	Iteration int       `json:"iteration,omitempty"`
	Data      any       `json:"data,omitempty"`
}

// LLMRequestData captures metadata about an API call being sent.
type LLMRequestData struct {
	Model        string `json:"model"`
	MessageCount int    `json:"message_count"`
	ToolCount    int    `json:"tool_count"`
}

// LLMResponseData captures the outcome of an API call.
type LLMResponseData struct {
	Model               string  `json:"model"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int64   `json:"cache_read_tokens,omitempty"`
	LatencyMs           int64   `json:"latency_ms"`
	Cost                float64 `json:"cost"`
	CostKnown           bool    `json:"cost_known"`
	FinishReason        string  `json:"finish_reason"`
	ToolCallCount       int     `json:"tool_call_count,omitempty"`
}

// ToolEventData captures a tool call start or end.
type ToolEventData struct {
	ToolName        string         `json:"tool_name"`
	ToolCallID      string         `json:"tool_call_id"`
	Input           string         `json:"input,omitempty"`
	Output          string         `json:"output,omitempty"`
	DurationMs      int64          `json:"duration_ms,omitempty"`
	IsError         bool           `json:"is_error,omitempty"`
	Error           string         `json:"error,omitempty"`
	MetadataSummary map[string]any `json:"metadata_summary,omitempty"`
}

// CompactData records a compaction event.
type CompactData struct {
	MessagesBefore int `json:"messages_before"`
	TokensBefore   int `json:"tokens_before"`
}

// ErrorData records an error event.
type ErrorData struct {
	Message string `json:"message"`
	Context string `json:"context,omitempty"`
}

// SessionStartData records session start metadata.
type SessionStartData struct {
	Model string `json:"model"`
}

// SessionEndData records session end metadata.
type SessionEndData struct {
	DurationMs int64 `json:"duration_ms"`
	Iterations int   `json:"iterations"`
}

// UserMessageData captures user input content.
type UserMessageData struct {
	Content string `json:"content"`
}

// AssistantMessageData captures assistant response content.
type AssistantMessageData struct {
	Content      string `json:"content"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// AssistantThinkingData captures chain-of-thought reasoning.
type AssistantThinkingData struct {
	Thinking string `json:"thinking"`
}

// StopDecisionData records a StopController policy evaluation.
type StopDecisionData struct {
	PolicyName  string         `json:"policy_name"`
	Action      string         `json:"action"` // "continue" / "terminate"
	Code        string         `json:"code"`
	Reason      string         `json:"reason"`
	Diagnostics map[string]any `json:"diagnostics,omitempty"`
}

// BudgetDecisionData records a budget check decision.
type BudgetDecisionData struct {
	Action          string  `json:"action"` // "allow" / "deny" / "hard_stop"
	Reason          string  `json:"reason"`
	TokenUsageRatio float64 `json:"token_usage_ratio,omitempty"`
	CostAccumulated float64 `json:"cost_accumulated,omitempty"`
	HasNudge        bool    `json:"has_nudge"`
}

// sessionDirMu guards CreateSessionDir to prevent concurrent races.
var sessionDirMu sync.Mutex

// --- Session log directory context key ---

type sessionLogDirKeyType struct{}

var sessionLogDirCtxKey = sessionLogDirKeyType{}

// WithSessionLogDir stores the session log directory path in the context.
func WithSessionLogDir(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, sessionLogDirCtxKey, dir)
}

// SessionLogDirFromCtx retrieves the session log directory from the context.
// Returns empty string if not set.
func SessionLogDirFromCtx(ctx context.Context) string {
	if dir, ok := ctx.Value(sessionLogDirCtxKey).(string); ok {
		return dir
	}
	return ""
}

// CreateSessionDir creates a uniquely-named session directory under logDir.
// Format: 2006-01-02_15-04-05_{8-hex-chars}. The random suffix eliminates
// naming collisions without a stat→mkdir race window.
// Returns empty string if logDir is empty.
func CreateSessionDir(logDir string) (string, error) {
	if logDir == "" {
		return "", nil
	}

	sessionDirMu.Lock()
	defer sessionDirMu.Unlock()

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", err
	}

	var randBytes [4]byte
	if _, err := crypto_rand.Read(randBytes[:]); err != nil {
		return "", fmt.Errorf("generate random suffix: %w", err)
	}

	ts := time.Now().Format("2006-01-02_15-04-05")
	suffix := hex.EncodeToString(randBytes[:])
	dirName := fmt.Sprintf("%s_%s", ts, suffix)
	dirPath := filepath.Join(logDir, dirName)

	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return "", fmt.Errorf("create session dir: %w", err)
	}
	return dirPath, nil
}

// --- SessionLogger ---

// SessionLogger writes structured JSONL to a session-specific log file.
// All methods are nil-safe — calling any method on a nil receiver is a no-op.
type SessionLogger struct {
	sessionID string
	agentName string
	subAgent  bool
	iteration int
	file      *os.File
	mu        sync.Mutex
	startTime time.Time
}

// NewSessionLogger creates a logger that writes to sessionDir/fileName.
// Returns nil if sessionDir is empty (disabled).
func NewSessionLogger(sessionDir, fileName, sessionID, agentName string, subAgent bool, appendMode bool) (*SessionLogger, error) {
	if sessionDir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, err
	}
	flags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(
		filepath.Join(sessionDir, fileName),
		flags, 0o644,
	)
	if err != nil {
		return nil, err
	}
	return &SessionLogger{
		sessionID: sessionID,
		agentName: agentName,
		subAgent:  subAgent,
		file:      f,
		startTime: time.Now(),
	}, nil
}

// NewMainAgentLogger creates a logger for the main agent, writing to agent.log (append mode).
func NewMainAgentLogger(sessionDir, sessionID string) (*SessionLogger, error) {
	return NewSessionLogger(sessionDir, "agent.log", sessionID, "coder", false, true)
}

// NewSubAgentLogger creates a logger for a sub-agent, writing to subagent-{name}-{shortID}.log.
func NewSubAgentLogger(sessionDir, sessionID, agentName, taskID string) (*SessionLogger, error) {
	fileName := fmt.Sprintf("subagent-%s-%s.log", agentName, SanitizeID(taskID))
	return NewSessionLogger(sessionDir, fileName, sessionID, agentName, true, false)
}

// SanitizeID returns the last 8 characters of an ID, filtering non-alphanumeric characters.
func SanitizeID(id string) string {
	if len(id) > 8 {
		id = id[len(id)-8:]
	}
	result := make([]byte, 0, len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		} else {
			result = append(result, '_')
		}
	}
	return string(result)
}

func (l *SessionLogger) write(entry LogEntry) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	entry.SessionID = l.sessionID
	entry.AgentName = l.agentName
	if l.subAgent {
		entry.SubAgent = true
	}
	entry.Iteration = l.iteration
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	l.file.Write(append(data, '\n'))
}

// SetIteration updates the current loop iteration counter.
func (l *SessionLogger) SetIteration(n int) {
	if l == nil {
		return
	}
	l.iteration = n
}

// LogSessionStart records the beginning of a session.
func (l *SessionLogger) LogSessionStart(d SessionStartData) {
	l.write(LogEntry{Type: EventSessionStart, Data: d})
}

// LogSessionEnd records the end of a session.
func (l *SessionLogger) LogSessionEnd() {
	if l == nil {
		return
	}
	l.write(LogEntry{Type: EventSessionEnd, Data: SessionEndData{
		DurationMs: time.Since(l.startTime).Milliseconds(),
		Iterations: l.iteration,
	}})
}

// LogLLMRequest records an outgoing LLM API call.
func (l *SessionLogger) LogLLMRequest(d LLMRequestData) {
	l.write(LogEntry{Type: EventLLMRequest, Data: d})
}

// LogLLMResponse records the result of an LLM API call.
func (l *SessionLogger) LogLLMResponse(d LLMResponseData) {
	l.write(LogEntry{Type: EventLLMResponse, Data: d})
}

// LogToolStart records the beginning of a tool execution.
func (l *SessionLogger) LogToolStart(d ToolEventData) {
	l.write(LogEntry{Type: EventToolStart, Data: d})
}

// LogToolEnd records the completion of a tool execution.
func (l *SessionLogger) LogToolEnd(d ToolEventData) {
	l.write(LogEntry{Type: EventToolEnd, Data: d})
}

// LogCompact records a message history compaction event.
func (l *SessionLogger) LogCompact(d CompactData) {
	l.write(LogEntry{Type: EventSessionCompact, Data: d})
}

// LogError records an error event.
func (l *SessionLogger) LogError(d ErrorData) {
	l.write(LogEntry{Type: EventError, Data: d})
}

// LogUserMessage records a user message with full content.
func (l *SessionLogger) LogUserMessage(d UserMessageData) {
	l.write(LogEntry{Type: EventUserMessage, Data: d})
}

// LogAssistantMessage records an assistant response with full content.
func (l *SessionLogger) LogAssistantMessage(d AssistantMessageData) {
	l.write(LogEntry{Type: EventAssistantMessage, Data: d})
}

// LogAssistantThinking records chain-of-thought reasoning.
func (l *SessionLogger) LogAssistantThinking(d AssistantThinkingData) {
	l.write(LogEntry{Type: EventAssistantThinking, Data: d})
}

// LogStopDecision records a stop controller policy evaluation.
func (l *SessionLogger) LogStopDecision(d StopDecisionData) {
	l.write(LogEntry{Type: EventStopDecision, Data: d})
}

// LogBudgetDecision records a budget check decision.
func (l *SessionLogger) LogBudgetDecision(d BudgetDecisionData) {
	l.write(LogEntry{Type: EventBudgetDecision, Data: d})
}

// Close closes the underlying log file.
func (l *SessionLogger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.file.Close()
}

// Truncate shortens a string to max length, appending a truncation marker if needed.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
