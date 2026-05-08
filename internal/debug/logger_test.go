package debug

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewSessionLogger(t *testing.T) {
	dir := t.TempDir()
	sessionID := "test-session-123"

	l, err := NewSessionLogger(dir, "agent.log", sessionID, "coder", false, true)
	if err != nil {
		t.Fatalf("NewSessionLogger: %v", err)
	}
	defer l.Close()

	if l == nil {
		t.Fatal("expected non-nil logger")
	}

	// Check log file exists
	path := filepath.Join(dir, "agent.log")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("log file not created: %v", err)
	}
}

func TestNewSessionLogger_DisabledWhenEmpty(t *testing.T) {
	l, err := NewSessionLogger("", "agent.log", "sid", "coder", false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l != nil {
		t.Fatal("expected nil logger when sessionDir is empty")
	}
}

func TestNilLoggerSafe(t *testing.T) {
	var l *SessionLogger
	// All of these should be no-ops, not panics
	l.LogSessionStart(SessionStartData{Model: "test"})
	l.LogSessionEnd()
	l.LogLLMRequest(LLMRequestData{})
	l.LogLLMResponse(LLMResponseData{})
	l.LogToolStart(ToolEventData{})
	l.LogToolEnd(ToolEventData{})
	l.LogCompact(CompactData{})
	l.LogError(ErrorData{})
	l.SetIteration(1)
	l.Close()
}

func TestSessionLoggerWrites(t *testing.T) {
	dir := t.TempDir()
	sessionID := "write-test"

	l, err := NewMainAgentLogger(dir, sessionID)
	if err != nil {
		t.Fatalf("NewMainAgentLogger: %v", err)
	}

	l.LogSessionStart(SessionStartData{Model: "glm-5"})
	l.SetIteration(1)
	l.LogLLMRequest(LLMRequestData{Model: "glm-5", MessageCount: 3, ToolCount: 8})
	l.LogToolStart(ToolEventData{ToolName: "Bash", ToolCallID: "tc1", Input: "echo hello"})
	l.LogToolEnd(ToolEventData{ToolName: "Bash", ToolCallID: "tc1", DurationMs: 42, Output: "hello"})
	l.LogLLMResponse(LLMResponseData{Model: "glm-5", InputTokens: 100, OutputTokens: 50, Cost: 0.001})
	l.LogCompact(CompactData{MessagesBefore: 20, TokensBefore: 100000})
	l.LogError(ErrorData{Message: "test error", Context: "testing"})
	l.LogSessionEnd()
	l.Close()

	// Read and verify the log file
	data, err := os.ReadFile(filepath.Join(dir, "agent.log"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 8 {
		t.Fatalf("expected 8 log lines, got %d", len(lines))
	}

	// Verify each line is valid JSON with expected type
	expectedTypes := []EventType{
		EventSessionStart, EventLLMRequest, EventToolStart, EventToolEnd,
		EventLLMResponse, EventSessionCompact, EventError, EventSessionEnd,
	}
	for i, line := range lines {
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d: invalid JSON: %v", i, err)
		}
		if entry.Type != expectedTypes[i] {
			t.Errorf("line %d: expected type %s, got %s", i, expectedTypes[i], entry.Type)
		}
		if entry.SessionID != sessionID {
			t.Errorf("line %d: expected session_id %s, got %s", i, sessionID, entry.SessionID)
		}
		if entry.AgentName != "coder" {
			t.Errorf("line %d: expected agent coder, got %s", i, entry.AgentName)
		}
		if entry.Timestamp == "" {
			t.Errorf("line %d: empty timestamp", i)
		}
	}
}

func TestSubAgentLogger(t *testing.T) {
	dir := t.TempDir()

	l, err := NewSubAgentLogger(dir, "sub-test", "explore", "toolu_01ABC123XYZ")
	if err != nil {
		t.Fatalf("NewSubAgentLogger: %v", err)
	}
	l.LogSessionStart(SessionStartData{Model: "test"})
	l.Close()

	// Verify file name uses sanitized short ID (last 8 chars of "toolu_01ABC123XYZ" = "BC123XYZ")
	expectedFile := "subagent-explore-BC123XYZ.log"
	path := filepath.Join(dir, expectedFile)
	if _, err := os.Stat(path); err != nil {
		// List files in dir for debugging
		entries, _ := os.ReadDir(dir)
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("expected file %s not found, got: %v", expectedFile, names)
	}

	data, _ := os.ReadFile(path)
	var entry LogEntry
	json.Unmarshal([]byte(strings.TrimSpace(string(data))), &entry)

	if !entry.SubAgent {
		t.Error("expected sub_agent to be true")
	}
	if entry.AgentName != "explore" {
		t.Errorf("expected agent explore, got %s", entry.AgentName)
	}
}

func TestMainAgentLoggerAppends(t *testing.T) {
	dir := t.TempDir()

	// First write
	l1, _ := NewMainAgentLogger(dir, "session-1")
	l1.LogSessionStart(SessionStartData{Model: "m1"})
	l1.LogSessionEnd()
	l1.Close()

	// Second write (should append, not overwrite)
	l2, _ := NewMainAgentLogger(dir, "session-1")
	l2.LogSessionStart(SessionStartData{Model: "m2"})
	l2.LogSessionEnd()
	l2.Close()

	data, _ := os.ReadFile(filepath.Join(dir, "agent.log"))
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 log lines (2 sessions appended), got %d", len(lines))
	}
}

func TestCreateSessionDir(t *testing.T) {
	dir := t.TempDir()

	sessionDir, err := CreateSessionDir(dir)
	if err != nil {
		t.Fatalf("CreateSessionDir: %v", err)
	}
	if sessionDir == "" {
		t.Fatal("expected non-empty session dir")
	}

	// Verify directory exists
	info, err := os.Stat(sessionDir)
	if err != nil {
		t.Fatalf("session dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory, got file")
	}

	// Verify naming format (YYYY-MM-DD_HH-MM-SS_xxxxxxxx)
	name := filepath.Base(sessionDir)
	if len(name) != 28 { // 2006-01-02_15-04-05_<8hex>
		t.Errorf("unexpected dir name length: %q (len=%d)", name, len(name))
	}
}

func TestCreateSessionDir_Collision(t *testing.T) {
	dir := t.TempDir()

	// Two rapid calls within the same second should still produce unique dirs
	// due to the random hex suffix.
	dir1, err := CreateSessionDir(dir)
	if err != nil {
		t.Fatalf("CreateSessionDir first call: %v", err)
	}
	dir2, err := CreateSessionDir(dir)
	if err != nil {
		t.Fatalf("CreateSessionDir second call: %v", err)
	}
	if dir1 == dir2 {
		t.Error("expected different dirs for same-second calls")
	}
	if _, err := os.Stat(dir1); err != nil {
		t.Fatalf("dir1 not created: %v", err)
	}
	if _, err := os.Stat(dir2); err != nil {
		t.Fatalf("dir2 not created: %v", err)
	}
}

func TestCreateSessionDir_Empty(t *testing.T) {
	dir, err := CreateSessionDir("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "" {
		t.Fatal("expected empty string for empty logDir")
	}
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"toolu_01ABC123XYZ", "BC123XYZ"}, // last 8 chars
		{"short", "short"},
		{"abc!@#de", "abc___de"},   // exactly 8 chars, no truncation
		{"abc!@#def", "bc___def"},  // 9 chars → last 8 → "bc!@#def" → sanitized
		{"12345678", "12345678"},
		{"123456789", "23456789"},
	}
	for _, tc := range tests {
		got := SanitizeID(tc.input)
		if got != tc.want {
			t.Errorf("SanitizeID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	short := "hello"
	if got := Truncate(short, 10); got != short {
		t.Errorf("expected %q, got %q", short, got)
	}

	long := strings.Repeat("a", 1000)
	got := Truncate(long, 500)
	if len(got) != 500+len("...[truncated]") {
		t.Errorf("expected truncated length, got %d", len(got))
	}
	if !strings.HasSuffix(got, "...[truncated]") {
		t.Error("expected truncation marker")
	}
}

func TestRotateLogs(t *testing.T) {
	dir := t.TempDir()
	oldTime := time.Now().Add(-8 * 24 * time.Hour)

	// Create an old session directory
	oldDir := filepath.Join(dir, "2026-03-18_10-00-00")
	os.MkdirAll(oldDir, 0o755)
	os.WriteFile(filepath.Join(oldDir, "agent.log"), []byte("old\n"), 0o644)
	os.Chtimes(oldDir, oldTime, oldTime)

	// Create a new session directory
	newDir := filepath.Join(dir, "2026-03-26_10-00-00")
	os.MkdirAll(newDir, 0o755)
	os.WriteFile(filepath.Join(newDir, "agent.log"), []byte("new\n"), 0o644)

	// Create an old legacy .jsonl file
	oldPath := filepath.Join(dir, "old-session.jsonl")
	os.WriteFile(oldPath, []byte("old\n"), 0o644)
	os.Chtimes(oldPath, oldTime, oldTime)

	// Create a non-log file (should not be deleted)
	otherPath := filepath.Join(dir, "notes.txt")
	os.WriteFile(otherPath, []byte("keep\n"), 0o644)
	os.Chtimes(otherPath, oldTime, oldTime)

	err := RotateLogs(dir, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("RotateLogs: %v", err)
	}

	// Old session dir should be deleted
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("old session dir should have been deleted")
	}
	// New session dir should remain
	if _, err := os.Stat(newDir); err != nil {
		t.Error("new session dir should still exist")
	}
	// Old .jsonl should be deleted
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old jsonl file should have been deleted")
	}
	// Non-log file should remain
	if _, err := os.Stat(otherPath); err != nil {
		t.Error("non-log file should still exist")
	}
}

func TestCreateSessionDir_ConcurrentCreation(t *testing.T) {
	tmpDir := t.TempDir()
	const goroutines = 20

	var wg sync.WaitGroup
	dirs := make([]string, goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			dirs[idx], errs[idx] = CreateSessionDir(tmpDir)
		}(i)
	}
	wg.Wait()

	// Verify no errors
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d failed: %v", i, err)
		}
	}

	// Verify all directories are unique
	seen := make(map[string]bool)
	for _, d := range dirs {
		if seen[d] {
			t.Fatalf("duplicate dir: %s", d)
		}
		seen[d] = true
		if _, err := os.Stat(d); err != nil {
			t.Fatalf("dir not created: %s: %v", d, err)
		}
	}
}

func TestRotateLogs_NonExistentDir(t *testing.T) {
	err := RotateLogs("/nonexistent/dir/path", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got %v", err)
	}
}

func TestSessionLogDirContext(t *testing.T) {
	ctx := context.Background()

	// Empty by default
	if dir := SessionLogDirFromCtx(ctx); dir != "" {
		t.Errorf("expected empty, got %q", dir)
	}

	// Set and retrieve
	ctx = WithSessionLogDir(ctx, "/tmp/logs/2026-03-26_14-30-05")
	if dir := SessionLogDirFromCtx(ctx); dir != "/tmp/logs/2026-03-26_14-30-05" {
		t.Errorf("expected session dir, got %q", dir)
	}
}
