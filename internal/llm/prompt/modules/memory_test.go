package modules

import (
	"strings"
	"testing"
	"time"
)

func TestFormatMemoryContext_Empty(t *testing.T) {
	got := FormatMemoryContext(nil, nil)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestFormatMemoryContext_Fenced(t *testing.T) {
	now := time.Now()
	session := []MemoryEntry{{Content: "session memory", CreatedAt: now}}
	global := []MemoryEntry{{Content: "global memory", CreatedAt: now}}

	got := FormatMemoryContext(session, global)
	if !strings.HasPrefix(got, "<memory-context>\n") {
		t.Fatalf("expected opening memory-context fence, got %q", got)
	}
	if !strings.HasSuffix(got, "</memory-context>") {
		t.Fatalf("expected closing memory-context fence, got %q", got)
	}
	if !strings.Contains(got, "## 会话记忆") {
		t.Fatalf("missing session section: %q", got)
	}
	if !strings.Contains(got, "## 全局记忆") {
		t.Fatalf("missing global section: %q", got)
	}
}
