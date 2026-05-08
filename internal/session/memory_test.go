package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryShouldTrigger_FirstTime(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultMemoryTrigger()
	mm := NewMemoryManager(dir, cfg)

	// Below thresholds
	if mm.ShouldTrigger("s1", 5000, 3) {
		t.Error("expected false when tokens and tool calls below threshold")
	}

	// Tokens OK but tool calls not enough
	if mm.ShouldTrigger("s2", 10000, 4) {
		t.Error("expected false when tool calls below MinToolCallsSinceWrite")
	}

	// Both meet threshold
	if !mm.ShouldTrigger("s3", 10000, 5) {
		t.Error("expected true when both thresholds met on first trigger")
	}
}

func TestMemoryShouldTrigger_Incremental(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultMemoryTrigger()
	mm := NewMemoryManager(dir, cfg)

	mgr := mm.(*memoryManager)

	// Simulate a previous trigger by setting state
	mgr.mu.Lock()
	mgr.states["s1"] = &RollingMemory{
		SessionID:        "s1",
		FilePath:         mgr.notesPath("s1"),
		LastTokenCount:   10000,
		LastToolCallSeen: 5,
	}
	mgr.mu.Unlock()

	// Delta not enough
	if mm.ShouldTrigger("s1", 13000, 9) {
		t.Error("expected false: delta tokens 3000 < 5000")
	}

	// Tool calls delta not enough
	if mm.ShouldTrigger("s1", 16000, 9) {
		t.Error("expected false: delta tool calls 4 < 5")
	}

	// Both deltas sufficient
	if !mm.ShouldTrigger("s1", 15000, 10) {
		t.Error("expected true: delta tokens 5000, delta tool calls 5")
	}
}

func TestMemoryShouldTrigger_False(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultMemoryTrigger()
	mm := NewMemoryManager(dir, cfg)

	// Zero tokens, zero tool calls — should not trigger
	if mm.ShouldTrigger("s1", 0, 0) {
		t.Error("expected false for zero tokens and tool calls")
	}
}

func TestMemoryUpdateNotes(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultMemoryTrigger()
	mm := NewMemoryManager(dir, cfg)

	ctx := context.Background()
	sessionID := "test-session"
	mockNotes := "Updated notes from LLM."

	query := func(_ context.Context, prompt string) (string, error) {
		if len(prompt) == 0 {
			t.Error("prompt should not be empty")
		}
		return mockNotes, nil
	}

	if err := mm.UpdateNotes(ctx, sessionID, "recent turns content", query); err != nil {
		t.Fatalf("UpdateNotes failed: %v", err)
	}

	notesPath := filepath.Join(dir, "session-memory", sessionID, "notes.md")
	data, err := os.ReadFile(notesPath)
	if err != nil {
		t.Fatalf("failed to read notes file: %v", err)
	}
	if string(data) != mockNotes {
		t.Errorf("expected %q, got %q", mockNotes, string(data))
	}
}

func TestMemoryUpdateNotes_Truncation(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultMemoryTrigger()
	cfg.MaxFileBytes = 10
	mm := NewMemoryManager(dir, cfg)

	ctx := context.Background()
	sessionID := "trunc-session"

	longNotes := "This is a very long note that exceeds the max file bytes limit."
	query := func(_ context.Context, _ string) (string, error) {
		return longNotes, nil
	}

	if err := mm.UpdateNotes(ctx, sessionID, "turns", query); err != nil {
		t.Fatalf("UpdateNotes failed: %v", err)
	}

	notesPath := filepath.Join(dir, "session-memory", sessionID, "notes.md")
	data, err := os.ReadFile(notesPath)
	if err != nil {
		t.Fatalf("failed to read notes file: %v", err)
	}
	if len(data) > cfg.MaxFileBytes {
		t.Errorf("expected file to be truncated to %d bytes, got %d", cfg.MaxFileBytes, len(data))
	}
}

func TestMemoryLoadForPrompt(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultMemoryTrigger()
	mm := NewMemoryManager(dir, cfg)

	ctx := context.Background()
	sessionID := "load-session"

	// Write notes manually
	notesDir := filepath.Join(dir, "session-memory", sessionID)
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	expected := "# Session Notes\n\nSome important notes."
	if err := os.WriteFile(filepath.Join(notesDir, "notes.md"), []byte(expected), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	content, err := mm.LoadForPrompt(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadForPrompt failed: %v", err)
	}
	if content != expected {
		t.Errorf("expected %q, got %q", expected, content)
	}
}

func TestMemoryLoadForPrompt_NotExist(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultMemoryTrigger()
	mm := NewMemoryManager(dir, cfg)

	ctx := context.Background()

	content, err := mm.LoadForPrompt(ctx, "nonexistent-session")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty string for missing file, got %q", content)
	}
}
