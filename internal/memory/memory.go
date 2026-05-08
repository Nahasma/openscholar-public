package memory

import (
	"context"
	"fmt"
	"time"
)

// MemoryItem represents a single memory entry.
type MemoryItem struct {
	ID               string         `json:"id"`
	SessionID        string         `json:"session_id,omitempty"`
	Content          string         `json:"content"`
	Metadata         map[string]any `json:"metadata"`
	AccessCount      int            `json:"access_count"`
	OperationHistory []string       `json:"operation_history"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// MemoryAction represents a single action output from the Executor.
type MemoryAction struct {
	Type        string `json:"type"` // INSERT, UPDATE, DELETE, NOOP
	Content     string `json:"content,omitempty"`
	MemoryIndex int    `json:"memory_index,omitempty"`
	Updated     string `json:"updated,omitempty"`
}

// KBCapture carries deterministic KB findings for memory persistence.
type KBCapture struct {
	SessionID  string           `json:"session_id"`
	SourceType string           `json:"source_type"` // e.g. "kb_query", "kb_search"
	Kind       string           `json:"kind"`        // e.g. "kb.answer", "kb.cross_answer"
	Question   string           `json:"question,omitempty"`
	PaperID    string           `json:"paper_id,omitempty"`
	Answer     string           `json:"answer"`
	Sources    []map[string]any `json:"sources,omitempty"`
}

// PromptMemoryOptions configures how memories are retrieved for prompt injection.
type PromptMemoryOptions struct {
	SessionLimit int // max session-scoped memories (default 10)
	GlobalLimit  int // max global memories (default 5)
	MaxItemChars int // truncate individual items beyond this (default 500)
}

// DefaultPromptOptions provides sensible defaults for v1.
var DefaultPromptOptions = PromptMemoryOptions{
	SessionLimit: 10,
	GlobalLimit:  5,
	MaxItemChars: 500,
}

// MemoryAgeNote returns a staleness warning prefix for memories older than 7 days.
// Returns an empty string for recent memories.
func MemoryAgeNote(createdAt time.Time) string {
	age := time.Since(createdAt)
	if age > 7*24*time.Hour {
		days := int(age.Hours() / 24)
		return fmt.Sprintf("[This memory was created %d days ago - please verify if still accurate] ", days)
	}
	return ""
}

// Service is the memory system service interface.
type Service interface {
	Insert(ctx context.Context, item MemoryItem) error
	Update(ctx context.Context, id string, content string, metadata map[string]any) error
	Delete(ctx context.Context, id string) error
	RetrieveForSession(ctx context.Context, sessionID string, limit int) ([]MemoryItem, error)
	RetrieveRecent(ctx context.Context, limit int) ([]MemoryItem, error)
	RetrieveForPrompt(ctx context.Context, sessionID string, opts PromptMemoryOptions) (session []MemoryItem, global []MemoryItem, err error)
	ExecuteSkills(ctx context.Context, sessionID string, sessionText string) error
	CaptureKB(ctx context.Context, capture KBCapture) error
	SetLLMCaller(caller LLMCaller)
}
