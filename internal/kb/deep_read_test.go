package kb_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/openscholar/openscholar/internal/kb"
	"github.com/openscholar/openscholar/internal/testutil"
)

func TestDeepRead_MapReduceAndTaskLifecycle(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)

	writer := svc.(interface {
		AddParsedPaper(ctx context.Context, paper kb.Paper, parseResult kb.ParseResult, semanticTreeStatus string) (kb.PaperIndexState, error)
	})
	_, err := writer.AddParsedPaper(context.Background(), kb.Paper{PaperID: "p-dr", Title: "Deep Read"}, kb.ParseResult{DocType: "pdf", Chunks: []kb.PaperChunk{
		{ChunkID: "c1", Kind: "page", PageStart: 1, PageEnd: 1, Content: stringsOfLen(9000)},
		{ChunkID: "c2", Kind: "page", PageStart: 2, PageEnd: 2, Content: stringsOfLen(9000)},
	}}, "not_requested")
	if err != nil {
		t.Fatalf("AddParsedPaper: %v", err)
	}

	deep := svc.(interface {
		EnsureDeepReadTask(ctx context.Context, paperID string, question string, allowCreate bool) (kb.KBTask, error)
		RunDeepRead(ctx context.Context, callLLM kb.LLMCaller, paperID string, question string, opts kb.DeepReadOptions) (*kb.DeepReadTaskResult, error)
		CompleteDeepReadTask(ctx context.Context, taskID string, result *kb.DeepReadTaskResult) error
	})
	task, err := deep.EnsureDeepReadTask(context.Background(), "p-dr", "summarize whole paper in detail", true)
	if err != nil {
		t.Fatalf("EnsureDeepReadTask: %v", err)
	}
	calls := 0
	result, err := deep.RunDeepRead(context.Background(), func(ctx context.Context, prompt string) (string, error) {
		calls++
		if calls <= 2 {
			return fmt.Sprintf("map-%d", calls), nil
		}
		return "final-answer", nil
	}, "p-dr", "summarize whole paper in detail", kb.DeepReadOptions{MaxBatchChars: 10000})
	if err != nil {
		t.Fatalf("RunDeepRead: %v", err)
	}
	if calls != 3 {
		t.Fatalf("LLM calls=%d, want 3 (2 map + 1 reduce)", calls)
	}
	if err := deep.CompleteDeepReadTask(context.Background(), task.TaskID, result); err != nil {
		t.Fatalf("CompleteDeepReadTask: %v", err)
	}
	row, err := q.GetKBTask(context.Background(), task.TaskID)
	if err != nil {
		t.Fatalf("GetKBTask: %v", err)
	}
	if row.Status != kb.KBTaskStatusSucceeded {
		t.Fatalf("status=%s", row.Status)
	}
	var stored kb.DeepReadTaskResult
	if err := json.Unmarshal([]byte(row.ResultJson.String), &stored); err != nil || stored.Answer != "final-answer" {
		t.Fatalf("stored result err=%v answer=%q", err, stored.Answer)
	}
}

func TestDeepRead_TokenBudgetSplitsBatches(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)

	writer := svc.(interface {
		AddParsedPaper(ctx context.Context, paper kb.Paper, parseResult kb.ParseResult, semanticTreeStatus string) (kb.PaperIndexState, error)
		RunDeepRead(ctx context.Context, callLLM kb.LLMCaller, paperID string, question string, opts kb.DeepReadOptions) (*kb.DeepReadTaskResult, error)
	})
	_, err := writer.AddParsedPaper(context.Background(), kb.Paper{PaperID: "p-token", Title: "Token Budget"}, kb.ParseResult{DocType: "pdf", Chunks: []kb.PaperChunk{
		{ChunkID: "c1", Kind: "page", PageStart: 1, PageEnd: 1, Content: stringsOfLen(100), TokenCount: 1000},
		{ChunkID: "c2", Kind: "page", PageStart: 2, PageEnd: 2, Content: stringsOfLen(100), TokenCount: 1000},
		{ChunkID: "c3", Kind: "page", PageStart: 3, PageEnd: 3, Content: stringsOfLen(100), TokenCount: 1000},
	}}, "not_requested")
	if err != nil {
		t.Fatalf("AddParsedPaper: %v", err)
	}

	calls := 0
	_, err = writer.RunDeepRead(context.Background(), func(ctx context.Context, prompt string) (string, error) {
		calls++
		if calls <= 3 {
			return fmt.Sprintf("map-%d", calls), nil
		}
		return "final-answer", nil
	}, "p-token", "summarize whole paper", kb.DeepReadOptions{MaxBatchChars: 100000, MaxBatchTokens: 1500})
	if err != nil {
		t.Fatalf("RunDeepRead: %v", err)
	}
	if calls != 4 {
		t.Fatalf("LLM calls=%d, want 4 (3 map + 1 reduce)", calls)
	}
}

func TestDeepReadWorker_RetryThenFailNoChunks(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)

	writer := svc.(interface {
		AddParsedPaper(ctx context.Context, paper kb.Paper, parseResult kb.ParseResult, semanticTreeStatus string) (kb.PaperIndexState, error)
		EnsureDeepReadTask(ctx context.Context, paperID string, question string, allowCreate bool) (kb.KBTask, error)
	})
	_, err := writer.AddParsedPaper(context.Background(), kb.Paper{PaperID: "p-empty", Title: "Empty"}, kb.ParseResult{}, "not_requested")
	if err != nil {
		t.Fatalf("AddParsedPaper: %v", err)
	}
	task, err := writer.EnsureDeepReadTask(context.Background(), "p-empty", "summarize the paper", true)
	if err != nil {
		t.Fatalf("EnsureDeepReadTask: %v", err)
	}

	calls := 0
	worker, err := kb.NewDeepReadWorker(svc, func(ctx context.Context, prompt string) (string, error) {
		calls++
		return "unused", nil
	}, "dr-test", time.Minute, kb.DeepReadOptions{MaxAttempts: 2})
	if err != nil {
		t.Fatalf("NewDeepReadWorker: %v", err)
	}
	if _, err := worker.RunOnce(context.Background()); err == nil {
		t.Fatalf("RunOnce first attempt unexpectedly succeeded")
	}
	row, err := q.GetKBTask(context.Background(), task.TaskID)
	if err != nil {
		t.Fatalf("GetKBTask: %v", err)
	}
	if row.Status != kb.KBTaskStatusQueued {
		t.Fatalf("after first attempt status=%s, want queued", row.Status)
	}
	if _, err := worker.RunOnce(context.Background()); err == nil {
		t.Fatalf("RunOnce second attempt unexpectedly succeeded")
	}
	row, err = q.GetKBTask(context.Background(), task.TaskID)
	if err != nil {
		t.Fatalf("GetKBTask second: %v", err)
	}
	if row.Status != kb.KBTaskStatusFailed {
		t.Fatalf("after second attempt status=%s, want failed", row.Status)
	}
	if calls != 0 {
		t.Fatalf("LLM calls=%d, want 0 for no chunks", calls)
	}
}

func stringsOfLen(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
