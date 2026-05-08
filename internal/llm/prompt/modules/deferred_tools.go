package modules

import (
	"fmt"
	"strings"
)

// DeferredToolEntry describes a deferred tool for the system prompt.
type DeferredToolEntry struct {
	Name        string
	Description string
	Domain      string
	CostTier    string
	Intent      string
	Scope       string
}

// NewDeferredToolsModule creates a prompt module that lists available extended tools.
// These tools are not loaded by default — the agent must use ToolSearch to activate them.
func NewDeferredToolsModule(entries []DeferredToolEntry) BaseModule {
	if len(entries) == 0 {
		return NewBaseModule("deferred_tools", "", 6)
	}

	var sb strings.Builder
	sb.WriteString("# Available Extended Tools\n")
	sb.WriteString("Use the ToolSearch tool to activate any of these specialized tools when needed:\n")
	for _, e := range entries {
		// Truncate description to first sentence for conciseness
		desc := firstSentence(e.Description, 100)
		extra := ""
		if e.Domain != "" || e.CostTier != "" || e.Intent != "" || e.Scope != "" {
			extra = fmt.Sprintf(" _(domain=%s, cost=%s, intent=%s, scope=%s)_", emptyDefault(e.Domain, "general"), emptyDefault(e.CostTier, "standard"), emptyDefault(e.Intent, "none"), emptyDefault(e.Scope, "session"))
		}
		fmt.Fprintf(&sb, "- **%s**: %s%s\n", e.Name, desc, extra)
	}

	return NewBaseModule("deferred_tools", sb.String(), 6)
}

func emptyDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func firstSentence(s string, maxLen int) string {
	// Find first period or newline
	for i, c := range s {
		if c == '.' || c == '\n' {
			if i+1 < len(s) {
				return s[:i+1]
			}
		}
		if i >= maxLen {
			return s[:i] + "..."
		}
	}
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
