package magicdoc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- ParseHeader tests ----------

func TestParseHeader_Basic(t *testing.T) {
	content := `# MAGIC DOC: My Notes
Keep this document up to date with the latest research findings.
Summarize key points clearly.

Some other content here.`

	doc, ok := ParseHeader(content)
	if !ok {
		t.Fatal("expected ParseHeader to return true")
	}
	if doc.Title != "My Notes" {
		t.Errorf("expected title 'My Notes', got %q", doc.Title)
	}
	if !strings.Contains(doc.Instruction, "Keep this document") {
		t.Errorf("expected instruction to contain first line, got %q", doc.Instruction)
	}
	if !strings.Contains(doc.Instruction, "Summarize key points") {
		t.Errorf("expected instruction to contain second line, got %q", doc.Instruction)
	}
}

func TestParseHeader_CaseInsensitive(t *testing.T) {
	content := `# magic doc: lowercase title
Some instruction here.`

	doc, ok := ParseHeader(content)
	if !ok {
		t.Fatal("expected ParseHeader to return true for lowercase")
	}
	if doc.Title != "lowercase title" {
		t.Errorf("expected title 'lowercase title', got %q", doc.Title)
	}
}

func TestParseHeader_NoMagicDoc(t *testing.T) {
	content := `# Regular Document
Some content here.
No magic doc header.`

	_, ok := ParseHeader(content)
	if ok {
		t.Fatal("expected ParseHeader to return false for non-magic doc")
	}
}

func TestParseHeader_EmptyInstruction(t *testing.T) {
	content := `# MAGIC DOC: Title Only

Content after blank line.`

	doc, ok := ParseHeader(content)
	if !ok {
		t.Fatal("expected ParseHeader to return true")
	}
	if doc.Title != "Title Only" {
		t.Errorf("expected title 'Title Only', got %q", doc.Title)
	}
	if doc.Instruction != "" {
		t.Errorf("expected empty instruction, got %q", doc.Instruction)
	}
}

func TestParseHeader_InstructionStopsAtBlankLine(t *testing.T) {
	content := `# MAGIC DOC: Test
Line one instruction.

This should not be in instruction.`

	doc, ok := ParseHeader(content)
	if !ok {
		t.Fatal("expected ParseHeader to return true")
	}
	if strings.Contains(doc.Instruction, "This should not") {
		t.Errorf("instruction should stop at blank line, got %q", doc.Instruction)
	}
	if !strings.Contains(doc.Instruction, "Line one") {
		t.Errorf("instruction should contain 'Line one', got %q", doc.Instruction)
	}
}

// ---------- Registry tests ----------

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	doc := Document{Title: "Test Doc", Instruction: "keep up to date"}
	r.Register("session1", "/path/to/file.md", doc)

	got, found := r.Get("session1", "/path/to/file.md")
	if !found {
		t.Fatal("expected to find registered document")
	}
	if got.Title != "Test Doc" {
		t.Errorf("expected title 'Test Doc', got %q", got.Title)
	}
	if got.SessionID != "session1" {
		t.Errorf("expected SessionID 'session1', got %q", got.SessionID)
	}
	if got.Path != "/path/to/file.md" {
		t.Errorf("expected path '/path/to/file.md', got %q", got.Path)
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	r := NewRegistry()
	_, found := r.Get("missing-session", "/some/path")
	if found {
		t.Fatal("expected not to find unregistered document")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register("s1", "/a.md", Document{Title: "A"})
	r.Register("s1", "/b.md", Document{Title: "B"})
	r.Register("s2", "/c.md", Document{Title: "C"})

	docs := r.List("s1")
	if len(docs) != 2 {
		t.Errorf("expected 2 docs for session s1, got %d", len(docs))
	}

	docs2 := r.List("s2")
	if len(docs2) != 1 {
		t.Errorf("expected 1 doc for session s2, got %d", len(docs2))
	}
}

func TestRegistry_MarkUpdated(t *testing.T) {
	r := NewRegistry()
	r.Register("s1", "/a.md", Document{Title: "A"})

	now := time.Now()
	r.MarkUpdated("s1", "/a.md", now)

	doc, _ := r.Get("s1", "/a.md")
	if !doc.LastUpdatedAt.Equal(now) {
		t.Errorf("expected LastUpdatedAt to be updated")
	}
}

// ---------- Service / OnFileRead tests ----------

func TestService_OnFileRead_RegistersMagicDoc(t *testing.T) {
	svc := NewService(60 * time.Second)

	evt := FileReadEvent{
		SessionID: "sess1",
		FilePath:  "/doc/notes.md",
		Content:   "# MAGIC DOC: Research Notes\nUpdate regularly.\n\nOther content.",
		ReadAt:    time.Now(),
	}
	svc.OnFileRead(evt)

	docs := svc.List("sess1")
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].Title != "Research Notes" {
		t.Errorf("expected title 'Research Notes', got %q", docs[0].Title)
	}
}

func TestService_OnFileRead_IgnoresNonMagicDoc(t *testing.T) {
	svc := NewService(60 * time.Second)

	evt := FileReadEvent{
		SessionID: "sess1",
		FilePath:  "/doc/regular.md",
		Content:   "# Regular Document\nJust some content.",
		ReadAt:    time.Now(),
	}
	svc.OnFileRead(evt)

	docs := svc.List("sess1")
	if len(docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(docs))
	}
}

// ---------- PendingUpdates tests ----------

func TestService_PendingUpdates(t *testing.T) {
	svc := NewService(60 * time.Second)

	// Read a magic doc
	readTime := time.Now()
	evt := FileReadEvent{
		SessionID: "sess1",
		FilePath:  "/doc/notes.md",
		Content:   "# MAGIC DOC: Notes\nKeep updated.",
		ReadAt:    readTime,
	}
	svc.OnFileRead(evt)

	// Should be pending (never updated)
	pending := svc.PendingUpdates("sess1")
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending doc, got %d", len(pending))
	}
}

func TestService_PendingUpdates_NoneAfterUpdate(t *testing.T) {
	impl := NewService(0).(*serviceImpl)

	// Register a doc with LastReadAt in the past
	pastTime := time.Now().Add(-2 * time.Minute)
	doc := Document{
		SessionID:   "sess1",
		Path:        "/doc/notes.md",
		Title:       "Notes",
		Instruction: "keep up to date",
		LastReadAt:  pastTime,
	}
	impl.registry.Register("sess1", "/doc/notes.md", doc)
	// Re-set LastReadAt since Register may preserve existing values
	if existing, ok := impl.registry.Get("sess1", "/doc/notes.md"); ok {
		existing.LastReadAt = pastTime
		impl.registry.Register("sess1", "/doc/notes.md", existing)
	}

	// Mark as updated at a time after read
	impl.registry.MarkUpdated("sess1", "/doc/notes.md", time.Now())

	pending := impl.PendingUpdates("sess1")
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after update, got %d", len(pending))
	}
}

// ---------- RunUpdate tests ----------

func TestService_RunUpdate_Basic(t *testing.T) {
	svc := NewService(0) // no rate limit for tests

	// Create a temp file with magic doc header
	dir := t.TempDir()
	filePath := filepath.Join(dir, "notes.md")
	originalContent := "# MAGIC DOC: Test Notes\nUpdate this document.\n\nOriginal content here, enough text to exceed threshold."
	if err := os.WriteFile(filePath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Register via OnFileRead
	svc.OnFileRead(FileReadEvent{
		SessionID: "s1",
		FilePath:  filePath,
		Content:   originalContent,
		ReadAt:    time.Now(),
	})

	docs := svc.List("s1")
	if len(docs) == 0 {
		t.Fatal("expected doc to be registered")
	}

	// Mock query that returns substantially different content
	mockQuery := func(_ context.Context, prompt string) (string, error) {
		return originalContent + "\n\nUpdated section with new content added by the LLM system.", nil
	}

	if err := svc.RunUpdate(context.Background(), docs[0], mockQuery); err != nil {
		t.Fatalf("RunUpdate failed: %v", err)
	}

	// Verify file was updated
	written, _ := os.ReadFile(filePath)
	if !strings.Contains(string(written), "Updated section") {
		t.Errorf("expected file to contain updated content, got: %s", string(written))
	}
}

func TestService_RunUpdate_SkipSmallDiff(t *testing.T) {
	svc := NewService(0)

	dir := t.TempDir()
	filePath := filepath.Join(dir, "notes.md")
	// Content that is exactly equal to what LLM returns (diff = 0)
	originalContent := "# MAGIC DOC: Test\nInstruction.\n\nSome content."
	if err := os.WriteFile(filePath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	svc.OnFileRead(FileReadEvent{
		SessionID: "s1",
		FilePath:  filePath,
		Content:   originalContent,
		ReadAt:    time.Now(),
	})

	docs := svc.List("s1")
	if len(docs) == 0 {
		t.Fatal("expected doc to be registered")
	}

	callCount := 0
	// Mock returns content with < 10 char diff (same content)
	mockQuery := func(_ context.Context, _ string) (string, error) {
		callCount++
		return originalContent, nil // exact same — diff = 0
	}

	if err := svc.RunUpdate(context.Background(), docs[0], mockQuery); err != nil {
		t.Fatalf("RunUpdate returned error: %v", err)
	}

	// File should NOT be updated (diff < 10)
	if callCount != 1 {
		t.Errorf("expected query to be called once, got %d", callCount)
	}

	// Verify LastUpdatedAt was NOT set (since we skipped)
	impl := svc.(*serviceImpl)
	registered, _ := impl.registry.Get("s1", filePath)
	if !registered.LastUpdatedAt.IsZero() {
		t.Errorf("expected LastUpdatedAt to remain zero when diff < 10")
	}
}

func TestService_RunUpdate_UnregisteredFile(t *testing.T) {
	svc := NewService(0)

	doc := Document{
		SessionID: "s1",
		Path:      "/nonexistent/file.md",
		Title:     "Unknown",
	}

	err := svc.RunUpdate(context.Background(), doc, func(_ context.Context, _ string) (string, error) {
		return "content", nil
	})

	if err == nil {
		t.Fatal("expected error for unregistered file")
	}
}

// ---------- Session isolation test ----------

func TestService_SessionIsolation(t *testing.T) {
	svc := NewService(60 * time.Second)

	svc.OnFileRead(FileReadEvent{
		SessionID: "session-A",
		FilePath:  "/doc/a.md",
		Content:   "# MAGIC DOC: Doc A\nInstruction A.",
		ReadAt:    time.Now(),
	})
	svc.OnFileRead(FileReadEvent{
		SessionID: "session-B",
		FilePath:  "/doc/b.md",
		Content:   "# MAGIC DOC: Doc B\nInstruction B.",
		ReadAt:    time.Now(),
	})

	docsA := svc.List("session-A")
	docsB := svc.List("session-B")

	if len(docsA) != 1 {
		t.Errorf("expected 1 doc for session-A, got %d", len(docsA))
	}
	if len(docsB) != 1 {
		t.Errorf("expected 1 doc for session-B, got %d", len(docsB))
	}
	if docsA[0].Title == docsB[0].Title {
		t.Errorf("session docs should be isolated, but titles match")
	}

	// Pending updates should also be isolated
	pendingA := svc.PendingUpdates("session-A")
	pendingB := svc.PendingUpdates("session-B")

	if len(pendingA) != 1 || len(pendingB) != 1 {
		t.Errorf("each session should have 1 pending update, got A=%d B=%d", len(pendingA), len(pendingB))
	}
}

// ---------- Context key test ----------

func TestServiceFromContext(t *testing.T) {
	svc := NewService(60 * time.Second)
	ctx := context.WithValue(context.Background(), ServiceCtxKey, svc)

	got := ServiceFromContext(ctx)
	if got == nil {
		t.Fatal("expected to get service from context")
	}
}

func TestServiceFromContext_Missing(t *testing.T) {
	got := ServiceFromContext(context.Background())
	if got != nil {
		t.Fatal("expected nil when service not in context")
	}
}
