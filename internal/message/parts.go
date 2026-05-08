package message

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Nahasma/openscholar-public/internal/llm/models"
)

type MessageRole string

const (
	Assistant MessageRole = "assistant"
	User      MessageRole = "user"
	System    MessageRole = "system"
	Tool      MessageRole = "tool"
)

type FinishReason string

const (
	FinishReasonEndTurn          FinishReason = "end_turn"
	FinishReasonMaxTokens        FinishReason = "max_tokens"
	FinishReasonToolUse          FinishReason = "tool_use"
	FinishReasonCanceled         FinishReason = "canceled"
	FinishReasonError            FinishReason = "error"
	FinishReasonPermissionDenied FinishReason = "permission_denied"
	FinishReasonUnknown          FinishReason = "unknown"
)

type ContentPart interface {
	isPart()
}

type ReasoningContent struct {
	Thinking string `json:"thinking"`
}

func (rc ReasoningContent) String() string { return rc.Thinking }
func (ReasoningContent) isPart()           {}

type TextContent struct {
	Text string `json:"text"`
}

func (tc TextContent) String() string { return tc.Text }
func (TextContent) isPart()           {}

type ImageURLContent struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

func (ImageURLContent) isPart() {}

type BinaryContent struct {
	Path     string `json:"path"`
	MIMEType string `json:"mime_type"`
	Data     []byte `json:"data"`
}

func (bc BinaryContent) String(provider models.ModelProvider) string {
	base64Encoded := base64.StdEncoding.EncodeToString(bc.Data)
	if provider == models.ProviderOpenAI {
		return "data:" + bc.MIMEType + ";base64," + base64Encoded
	}
	return base64Encoded
}

func (BinaryContent) isPart() {}

// ToolCallState represents the lifecycle state of a tool call.
type ToolCallState string

const (
	ToolCallQueued    ToolCallState = "queued"
	ToolCallRunning   ToolCallState = "running"
	ToolCallCompleted ToolCallState = "completed"
	ToolCallErrored   ToolCallState = "errored"
	ToolCallCanceled  ToolCallState = "canceled"
)

type ToolCall struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	BatchID    string        `json:"batch_id,omitempty"` // Consecutive same-tool batch within one assistant turn
	Input      string        `json:"input"`
	Type       string        `json:"type"`
	Finished   bool          `json:"finished"`              // Retained for backward compatibility
	State      ToolCallState `json:"state,omitempty"`       // Explicit lifecycle state
	InputDone  bool          `json:"input_done,omitempty"`  // Whether the streamed tool input is complete
	StartedAt  int64         `json:"started_at,omitempty"`  // Unix timestamp when execution started
	FinishedAt int64         `json:"finished_at,omitempty"` // Unix timestamp when execution ended
	Order      int           `json:"order,omitempty"`       // Position in batch (0-based)
}

// EffectiveState returns the tool call's state, falling back to legacy Finished field
// for backward compatibility with old sessions that lack an explicit State.
func (tc ToolCall) EffectiveState() ToolCallState {
	if tc.State != "" {
		return tc.State
	}
	// Legacy fallback: derive state from Finished bool
	if tc.Finished {
		return ToolCallCompleted
	}
	return ToolCallRunning
}

// IsTerminalToolState reports whether a lifecycle state means execution has ended.
func IsTerminalToolState(state ToolCallState) bool {
	switch state {
	case ToolCallCompleted, ToolCallErrored, ToolCallCanceled:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the tool call execution has ended.
func (tc ToolCall) IsTerminal() bool {
	return IsTerminalToolState(tc.EffectiveState())
}

func (ToolCall) isPart() {}

type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	Metadata   string `json:"metadata"`
	IsError    bool   `json:"is_error"`
}

func (ToolResult) isPart() {}

type Finish struct {
	Reason FinishReason `json:"reason"`
	Time   int64        `json:"time"`
}

func (Finish) isPart() {}

type Usage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	Estimated                bool  `json:"estimated,omitempty"`
}

func (u Usage) IsZero() bool {
	return u.InputTokens == 0 &&
		u.OutputTokens == 0 &&
		u.CacheCreationInputTokens == 0 &&
		u.CacheReadInputTokens == 0
}

type Message struct {
	ID        string
	Role      MessageRole
	SessionID string
	Parts     []ContentPart
	Model     models.ModelID
	Usage     Usage
	Meta      map[string]any
	CreatedAt int64
	UpdatedAt int64
}

func (m *Message) Content() TextContent {
	for _, part := range m.Parts {
		if c, ok := part.(TextContent); ok {
			return c
		}
	}
	return TextContent{}
}

func (m *Message) ToolCalls() []ToolCall {
	var toolCalls []ToolCall
	for _, part := range m.Parts {
		if c, ok := part.(ToolCall); ok {
			toolCalls = append(toolCalls, c)
		}
	}
	return toolCalls
}

func (m *Message) ToolResults() []ToolResult {
	var toolResults []ToolResult
	for _, part := range m.Parts {
		if c, ok := part.(ToolResult); ok {
			toolResults = append(toolResults, c)
		}
	}
	return toolResults
}

func (m *Message) IsFinished() bool {
	for _, part := range m.Parts {
		if _, ok := part.(Finish); ok {
			return true
		}
	}
	return false
}

func (m *Message) FinishPart() *Finish {
	for _, part := range m.Parts {
		if c, ok := part.(Finish); ok {
			return &c
		}
	}
	return nil
}

func (m *Message) FinishReason() FinishReason {
	for _, part := range m.Parts {
		if c, ok := part.(Finish); ok {
			return c.Reason
		}
	}
	return ""
}

func (m *Message) AppendContent(delta string) {
	for i, part := range m.Parts {
		if c, ok := part.(TextContent); ok {
			m.Parts[i] = TextContent{Text: c.Text + delta}
			return
		}
	}
	m.Parts = append(m.Parts, TextContent{Text: delta})
}

// SanitizeTextContent applies a sanitizer function to all TextContent parts.
// Used as a post-processing step after message completion to clean up any
// provider-specific tags that leaked through during streaming.
func (m *Message) SanitizeTextContent(sanitize func(string) string) {
	for i, part := range m.Parts {
		if c, ok := part.(TextContent); ok {
			cleaned := sanitize(c.Text)
			if cleaned != c.Text {
				m.Parts[i] = TextContent{Text: cleaned}
			}
		}
	}
}

func (m *Message) AppendReasoningContent(delta string) {
	for i, part := range m.Parts {
		if c, ok := part.(ReasoningContent); ok {
			m.Parts[i] = ReasoningContent{Thinking: c.Thinking + delta}
			return
		}
	}
	m.Parts = append(m.Parts, ReasoningContent{Thinking: delta})
}

func (m *Message) AddToolCall(tc ToolCall) {
	for i, part := range m.Parts {
		if c, ok := part.(ToolCall); ok && c.ID == tc.ID {
			m.Parts[i] = tc
			return
		}
	}
	m.Parts = append(m.Parts, tc)
}

func (m *Message) SetToolCalls(tc []ToolCall) {
	parts := make([]ContentPart, 0)
	for _, part := range m.Parts {
		if _, ok := part.(ToolCall); ok {
			continue
		}
		parts = append(parts, part)
	}
	m.Parts = parts
	for _, toolCall := range tc {
		if normalized, ok := NormalizeToolCallInput(toolCall.Input); ok {
			toolCall.Input = normalized
			m.Parts = append(m.Parts, toolCall)
		}
		// Drop tool calls that cannot be normalized
	}
}

// FinishToolCall marks a tool call as completed (execution done).
// Sets both Finished=true and State=completed with a finish timestamp.
func (m *Message) FinishToolCall(toolCallID string) {
	for i, part := range m.Parts {
		if c, ok := part.(ToolCall); ok && c.ID == toolCallID {
			m.Parts[i] = ToolCall{
				ID: c.ID, Name: c.Name, BatchID: c.BatchID, Input: c.Input,
				Type: c.Type, Finished: true,
				State: ToolCallCompleted, InputDone: true, StartedAt: c.StartedAt,
				FinishedAt: time.Now().Unix(), Order: c.Order,
			}
			return
		}
	}
}

// MarkToolCallInputDone marks a tool call's input as fully received from the LLM stream.
// This does not change execution state because "input received" != "execution done".
// Used by processEvent on EventToolUseStop.
func (m *Message) MarkToolCallInputDone(toolCallID string) {
	for i, part := range m.Parts {
		if c, ok := part.(ToolCall); ok && c.ID == toolCallID {
			m.Parts[i] = ToolCall{
				ID: c.ID, Name: c.Name, BatchID: c.BatchID, Input: c.Input,
				Type: c.Type, Finished: c.Finished,
				State: c.State, InputDone: true, StartedAt: c.StartedAt,
				FinishedAt: c.FinishedAt, Order: c.Order,
			}
			return
		}
	}
}

// StartToolCall transitions a tool call from queued to running.
func (m *Message) StartToolCall(toolCallID string) {
	for i, part := range m.Parts {
		if c, ok := part.(ToolCall); ok && c.ID == toolCallID {
			m.Parts[i] = ToolCall{
				ID: c.ID, Name: c.Name, BatchID: c.BatchID, Input: c.Input,
				Type: c.Type, Finished: c.Finished,
				State: ToolCallRunning, InputDone: c.InputDone, StartedAt: time.Now().Unix(),
				Order: c.Order,
			}
			return
		}
	}
}

// CancelToolCall transitions a tool call to canceled state.
func (m *Message) CancelToolCall(toolCallID string) {
	for i, part := range m.Parts {
		if c, ok := part.(ToolCall); ok && c.ID == toolCallID {
			m.Parts[i] = ToolCall{
				ID: c.ID, Name: c.Name, BatchID: c.BatchID, Input: c.Input,
				Type: c.Type, Finished: true,
				State: ToolCallCanceled, InputDone: true, StartedAt: c.StartedAt,
				FinishedAt: time.Now().Unix(), Order: c.Order,
			}
			return
		}
	}
}

// ErrorToolCall transitions a tool call to errored state.
func (m *Message) ErrorToolCall(toolCallID string) {
	for i, part := range m.Parts {
		if c, ok := part.(ToolCall); ok && c.ID == toolCallID {
			m.Parts[i] = ToolCall{
				ID: c.ID, Name: c.Name, BatchID: c.BatchID, Input: c.Input,
				Type: c.Type, Finished: true,
				State: ToolCallErrored, InputDone: true, StartedAt: c.StartedAt,
				FinishedAt: time.Now().Unix(), Order: c.Order,
			}
			return
		}
	}
}

func (m *Message) AddFinish(reason FinishReason) {
	for i, part := range m.Parts {
		if _, ok := part.(Finish); ok {
			m.Parts = slices.Delete(m.Parts, i, i+1)
			break
		}
	}
	m.Parts = append(m.Parts, Finish{Reason: reason, Time: time.Now().Unix()})
}

func (m *Message) AppendToolInput(toolCallID, input string) {
	for i, part := range m.Parts {
		if c, ok := part.(ToolCall); ok && c.ID == toolCallID {
			m.Parts[i] = ToolCall{
				ID: c.ID, Name: c.Name, BatchID: c.BatchID,
				Input: c.Input + input,
				Type:  c.Type, Finished: c.Finished,
				State: c.State, InputDone: c.InputDone, StartedAt: c.StartedAt,
				FinishedAt: c.FinishedAt, Order: c.Order,
			}
			return
		}
	}
}

func (m *Message) CleanIncompleteToolCalls() {
	cleaned := make([]ContentPart, 0, len(m.Parts))
	for _, part := range m.Parts {
		if tc, ok := part.(ToolCall); ok {
			if normalized, ok := NormalizeToolCallInput(tc.Input); ok {
				tc.Input = normalized
				cleaned = append(cleaned, tc)
			}
			continue
		}
		cleaned = append(cleaned, part)
	}
	m.Parts = cleaned
}

func (m *Message) AddToolResult(tr ToolResult) {
	m.Parts = append(m.Parts, tr)
}

// InitToolCallsQueued sets all tool calls in the message to queued state with order indices.
// Called after the assistant message is complete and before tool execution begins.
func (m *Message) InitToolCallsQueued() {
	order := 0
	batchSeq := 0
	currentBatchID := ""
	prevToolName := ""
	for i, part := range m.Parts {
		if c, ok := part.(ToolCall); ok {
			// Batch consecutive same-name calls into a stable batch ID.
			if c.Name != prevToolName {
				batchSeq++
				currentBatchID = c.Name + "#" + strconv.Itoa(batchSeq)
				prevToolName = c.Name
			}
			m.Parts[i] = ToolCall{
				ID: c.ID, Name: c.Name, BatchID: currentBatchID, Input: c.Input,
				Type: c.Type, Finished: false,
				State: ToolCallQueued, InputDone: true, Order: order,
			}
			order++
		}
	}
}

// NormalizeToolCallInput attempts to normalize a raw tool call input into a
// valid JSON object string. It handles double-encoded strings and other
// provider quirks. Returns the original bytes (preserving number precision)
// and true on success, or ("", false) if the input cannot be validated as a
// JSON object.
func NormalizeToolCallInput(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}

	// Fast path: already a valid JSON object — return as-is to preserve numbers
	if isJSONObject(raw) {
		return raw, true
	}

	// Unwrap: if top-level is a JSON string, decode one layer and retry
	var str string
	if err := json.Unmarshal([]byte(raw), &str); err == nil {
		if isJSONObject(str) {
			return str, true
		}
	}

	return "", false
}

// isJSONObject checks whether s is a syntactically valid JSON object (starts with '{').
func isJSONObject(s string) bool {
	trimmed := strings.TrimSpace(s)
	return len(trimmed) >= 2 && trimmed[0] == '{' && json.Valid([]byte(trimmed))
}

type Attachment struct {
	FilePath string
	FileName string
	MimeType string
	Content  []byte
}
