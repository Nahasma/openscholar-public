package modules

import (
	"fmt"
	"strings"
	"time"
)

// MemoryEntry represents a single memory item for prompt injection.
type MemoryEntry struct {
	Content   string
	CreatedAt time.Time
}

// NewMemoryModule creates a prompt module that injects recent memories.
// Priority 1: appears right after BaseModule in the system prompt.
// Returns a DynamicBaseModule so the provider can skip caching this block.
func NewMemoryModule(memories []MemoryEntry) DynamicBaseModule {
	if len(memories) == 0 {
		return NewDynamicBaseModule("memory", "", 1)
	}

	var sb strings.Builder
	sb.WriteString("# Session Memory\nThe following memories have been extracted from previous interactions:\n")
	for i, m := range memories {
		date := m.CreatedAt.Format("2006-01-02")
		fmt.Fprintf(&sb, "%d. [%s] %s\n", i+1, date, m.Content)
	}

	return NewDynamicBaseModule("memory", sb.String(), 1)
}

// FormatMemoryContext formats session and global memories for ephemeral message injection.
// Returns empty string if both slices are empty.
func FormatMemoryContext(session, global []MemoryEntry) string {
	if len(session) == 0 && len(global) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<memory-context>\n")
	sb.WriteString("# Memory Context\n")
	sb.WriteString("以下是从历史交互中提炼的记忆，仅作为补充上下文。如与当前用户要求冲突，以当前要求为准。\n")

	if len(session) > 0 {
		sb.WriteString("\n## 会话记忆\n")
		for i, m := range session {
			date := m.CreatedAt.Format("2006-01-02")
			fmt.Fprintf(&sb, "%d. [%s] %s%s\n", i+1, date, memoryAgeNote(m.CreatedAt), m.Content)
		}
	}

	if len(global) > 0 {
		sb.WriteString("\n## 全局记忆\n")
		for i, m := range global {
			date := m.CreatedAt.Format("2006-01-02")
			fmt.Fprintf(&sb, "%d. [%s] %s%s\n", i+1, date, memoryAgeNote(m.CreatedAt), m.Content)
		}
	}
	sb.WriteString("</memory-context>")

	return sb.String()
}

// memoryAgeNote returns a staleness warning prefix for memories older than 7 days.
func memoryAgeNote(createdAt time.Time) string {
	age := time.Since(createdAt)
	if age > 7*24*time.Hour {
		days := int(age.Hours() / 24)
		return fmt.Sprintf("[This memory was created %d days ago - please verify if still accurate] ", days)
	}
	return ""
}

// FormatMemoriesPrompt formats memory items for direct injection into system prompt.
// Deprecated: use FormatMemoryContext for runtime ephemeral injection.
func FormatMemoriesPrompt(memories []MemoryEntry) string {
	if len(memories) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Session Memory\nThe following memories have been extracted from previous interactions:\n")
	for i, m := range memories {
		date := m.CreatedAt.Format("2006-01-02")
		fmt.Fprintf(&sb, "%d. [%s] %s\n", i+1, date, m.Content)
	}

	return sb.String()
}
