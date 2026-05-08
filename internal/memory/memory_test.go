package memory

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/pressly/goose/v3"

	"github.com/openscholar/openscholar/internal/db"
)

func TestMemoryAgeNote(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		createdAt time.Time
		wantEmpty bool
	}{
		{"recent", now.Add(-1 * time.Hour), true},
		{"6_days", now.Add(-6 * 24 * time.Hour), true},
		{"8_days", now.Add(-8 * 24 * time.Hour), false},
		{"30_days", now.Add(-30 * 24 * time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note := MemoryAgeNote(tt.createdAt)
			if (note == "") != tt.wantEmpty {
				t.Errorf("MemoryAgeNote(%v) = %q, wantEmpty = %v", tt.createdAt, note, tt.wantEmpty)
			}
		})
	}
}

func TestSetLLMCaller(t *testing.T) {
	svc := NewService(nil, nil, nil).(*memoryService)
	if svc.callLLM != nil {
		t.Fatalf("expected nil callLLM initially")
	}

	svc.SetLLMCaller(func(ctx context.Context, prompt string) (string, error) {
		return "ok", nil
	})
	if svc.callLLM == nil {
		t.Fatalf("expected callLLM to be set")
	}
}

func TestCaptureKB_SessionScopedWithMetadata(t *testing.T) {
	_, q := setupMemoryTestDB(t)
	createSessionForMemoryTest(t, q, "sess-1")
	svc := NewService(q, nil, nil)

	err := svc.CaptureKB(context.Background(), KBCapture{
		SessionID:  "sess-1",
		SourceType: "kb_query",
		Kind:       "kb.answer",
		Question:   "What is the method?",
		PaperID:    "paper-1",
		Answer:     "The paper proposes method X.",
		Sources: []map[string]any{
			{"node_id": "n1", "start_page": 2, "end_page": 3},
		},
	})
	if err != nil {
		t.Fatalf("CaptureKB() error = %v", err)
	}

	items, err := svc.RetrieveForSession(context.Background(), "sess-1", 10)
	if err != nil {
		t.Fatalf("RetrieveForSession() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 memory item, got %d", len(items))
	}

	md := items[0].Metadata
	if md["source_type"] != "kb_query" {
		t.Fatalf("source_type = %v, want kb_query", md["source_type"])
	}
	if md["question"] != "What is the method?" {
		t.Fatalf("question = %v, want %q", md["question"], "What is the method?")
	}
	if md["paper_id"] != "paper-1" {
		t.Fatalf("paper_id = %v, want paper-1", md["paper_id"])
	}
	if _, ok := md["sources"]; !ok {
		t.Fatalf("expected sources metadata")
	}
}

func TestCaptureKB_WithoutSessionSkipsGlobalFallback(t *testing.T) {
	_, q := setupMemoryTestDB(t)
	svc := NewService(q, nil, nil)

	err := svc.CaptureKB(context.Background(), KBCapture{
		SessionID:  "",
		SourceType: "kb_query",
		Kind:       "kb.answer",
		Question:   "q",
		Answer:     "a",
	})
	if err != nil {
		t.Fatalf("CaptureKB() error = %v", err)
	}

	items, err := svc.RetrieveRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("RetrieveRecent() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 memory items, got %d", len(items))
	}
}

func createSessionForMemoryTest(t *testing.T, q db.Querier, sessionID string) {
	t.Helper()
	_, err := q.CreateSession(context.Background(), db.CreateSessionParams{
		ID:    sessionID,
		Title: "memory test",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
}

var memoryGooseMu sync.Mutex

func setupMemoryTestDB(t *testing.T) (*sql.DB, db.Querier) {
	t.Helper()

	tmpFile := filepath.Join(t.TempDir(), "memory-test.db")
	conn, err := sql.Open("sqlite3", tmpFile)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := conn.Exec(pragma); err != nil {
			t.Fatalf("failed to set pragma %s: %v", pragma, err)
		}
	}

	memoryGooseMu.Lock()
	goose.SetBaseFS(db.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		memoryGooseMu.Unlock()
		t.Fatalf("failed to set goose dialect: %v", err)
	}
	if err := goose.Up(conn, "migrations"); err != nil {
		memoryGooseMu.Unlock()
		t.Fatalf("failed to run migrations: %v", err)
	}
	memoryGooseMu.Unlock()

	return conn, db.New(conn)
}
