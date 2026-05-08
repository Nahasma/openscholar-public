package kb_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/kb"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

type parsedPaperWriter interface {
	AddParsedPaper(ctx context.Context, paper kb.Paper, parseResult kb.ParseResult, semanticTreeStatus string) (kb.PaperIndexState, error)
}

type paperQuerier interface {
	QueryPaper(ctx context.Context, callLLM kb.LLMCaller, paperID string, question string, opts kb.QueryOptions) (*kb.SearchResult, error)
}

func TestQueryPaper_MetadataRouteZeroLLM(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)

	writer := svc.(parsedPaperWriter)
	_, err := writer.AddParsedPaper(context.Background(), kb.Paper{PaperID: "p-meta", Title: "Meta Paper", Year: 2025}, kb.ParseResult{
		DocType: "pdf",
		Chunks:  []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Content: "A method"}},
	}, "not_requested")
	if err != nil {
		t.Fatalf("AddParsedPaper() error = %v", err)
	}

	llmCalls := 0
	res, err := svc.(paperQuerier).QueryPaper(context.Background(), func(ctx context.Context, prompt string) (string, error) {
		llmCalls++
		return "", nil
	}, "p-meta", "title year indexing status", kb.QueryOptions{})
	if err != nil {
		t.Fatalf("QueryPaper() error = %v", err)
	}
	if llmCalls != 0 {
		t.Fatalf("LLM calls = %d, want 0", llmCalls)
	}
	if res.Diagnostics.Route != "metadata" {
		t.Fatalf("route = %q, want metadata", res.Diagnostics.Route)
	}
	if res.Diagnostics.LLMCallCount != 0 {
		t.Fatalf("llm_call_count = %d, want 0", res.Diagnostics.LLMCallCount)
	}
}

func TestQueryPaper_MetadataQuestionWordsStayMetadata(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)

	writer := svc.(parsedPaperWriter)
	_, err := writer.AddParsedPaper(context.Background(), kb.Paper{PaperID: "p-meta-words", Title: "Meta Words", FilePath: "/tmp/private-paper.pdf"}, kb.ParseResult{
		DocType:    "pdf",
		TotalPages: 3,
		Chunks:     []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Content: "A method"}},
	}, "not_requested")
	if err != nil {
		t.Fatalf("AddParsedPaper() error = %v", err)
	}

	llmCalls := 0
	res, err := svc.(paperQuerier).QueryPaper(context.Background(), func(ctx context.Context, prompt string) (string, error) {
		llmCalls++
		return "", nil
	}, "p-meta-words", "what is the title and how many pages?", kb.QueryOptions{})
	if err != nil {
		t.Fatalf("QueryPaper() error = %v", err)
	}
	if llmCalls != 0 {
		t.Fatalf("LLM calls = %d, want 0", llmCalls)
	}
	if res.Diagnostics.Route != "metadata" {
		t.Fatalf("route = %q, want metadata", res.Diagnostics.Route)
	}
	if strings.Contains(res.Answer, "/tmp/") {
		t.Fatalf("metadata title/pages answer leaked file path: %s", res.Answer)
	}
}

func TestQueryPaper_LocalPagesWithoutTreeRow(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)

	writer := svc.(parsedPaperWriter)
	_, err := writer.AddParsedPaper(context.Background(), kb.Paper{PaperID: "p-local", Title: "Local Paper"}, kb.ParseResult{
		DocType: "pdf",
		Chunks:  []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Title: "Method", Content: "The method uses sparse attention."}},
	}, "not_requested")
	if err != nil {
		t.Fatalf("AddParsedPaper() error = %v", err)
	}

	llmCalls := 0
	res, err := svc.(paperQuerier).QueryPaper(context.Background(), func(ctx context.Context, prompt string) (string, error) {
		llmCalls++
		if !strings.Contains(prompt, "sparse attention") {
			t.Fatalf("prompt missing chunk content: %s", prompt)
		}
		return "It uses sparse attention (p. 1).", nil
	}, "p-local", "what method is used?", kb.QueryOptions{})
	if err != nil {
		t.Fatalf("QueryPaper() error = %v", err)
	}
	if llmCalls != 1 {
		t.Fatalf("LLM calls = %d, want 1", llmCalls)
	}
	if res.Diagnostics.Route != "local_pages" {
		t.Fatalf("route = %q, want local_pages", res.Diagnostics.Route)
	}
	if res.Diagnostics.ChunkHitCount == 0 {
		t.Fatal("expected chunk hits")
	}
	if len(res.Sources) == 0 || res.Sources[0].NodeID != "page_1" {
		t.Fatalf("sources = %#v, want page_1", res.Sources)
	}
}

func TestQueryPaper_UsesCandidateHintChunks(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)

	writer := svc.(parsedPaperWriter)
	_, err := writer.AddParsedPaper(context.Background(), kb.Paper{PaperID: "p-hints", Title: "Hint Paper"}, kb.ParseResult{
		DocType: "pdf",
		Chunks: []kb.PaperChunk{
			{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Content: "generic background"},
			{ChunkID: "page_7", Kind: "page", PageStart: 7, PageEnd: 7, Content: "candidate-only evidence token"},
		},
	}, "not_requested")
	if err != nil {
		t.Fatalf("AddParsedPaper() error = %v", err)
	}

	res, err := svc.(paperQuerier).QueryPaper(context.Background(), func(ctx context.Context, prompt string) (string, error) {
		if !strings.Contains(prompt, "candidate-only evidence token") {
			t.Fatalf("prompt missing hinted chunk: %s", prompt)
		}
		return "hinted answer (p. 7)", nil
	}, "p-hints", "unmatched query terms", kb.QueryOptions{CandidateHints: []string{"page_7"}})
	if err != nil {
		t.Fatalf("QueryPaper() error = %v", err)
	}
	if len(res.Sources) == 0 || res.Sources[0].NodeID != "page_7" {
		t.Fatalf("sources = %#v, want page_7", res.Sources)
	}
}

func TestQueryPaper_SemanticFallbackToLocalPages(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)

	writer := svc.(parsedPaperWriter)
	_, err := writer.AddParsedPaper(context.Background(), kb.Paper{PaperID: "p-sem", Title: "Semantic Paper"}, kb.ParseResult{
		DocType: "pdf",
		Chunks:  []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Content: "Fallback chunk content."}},
	}, "ready")
	if err != nil {
		t.Fatalf("AddParsedPaper() error = %v", err)
	}

	res, err := svc.(paperQuerier).QueryPaper(context.Background(), func(ctx context.Context, prompt string) (string, error) {
		return "Fallback answer (p. 1).", nil
	}, "p-sem", "explain the section result", kb.QueryOptions{PreferredRoute: "semantic_tree"})
	if err != nil {
		t.Fatalf("QueryPaper() error = %v", err)
	}
	if res.Diagnostics.Route != "local_pages" {
		t.Fatalf("route = %q, want local_pages", res.Diagnostics.Route)
	}
	if !res.Diagnostics.FallbackUsed {
		t.Fatal("expected fallback used")
	}
	if res.Diagnostics.LLMCallCount != 1 {
		t.Fatalf("llm_call_count = %d, want 1", res.Diagnostics.LLMCallCount)
	}
}

func TestQueryPaper_AddPaperSummaryTreeUsesSemanticRoute(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)

	err := svc.AddPaper(context.Background(), kb.Paper{PaperID: "p-summary-tree", Title: "Summary Tree"}, kb.PaperTree{
		DocName: "Summary Tree",
		Structure: []kb.TreeNode{{
			NodeID: "n1", Title: "Method", StartIndex: 1, EndIndex: 1,
			Summary: "summary-only method evidence",
		}},
	})
	if err != nil {
		t.Fatalf("AddPaper() error = %v", err)
	}

	calls := 0
	res, err := svc.(paperQuerier).QueryPaper(context.Background(), func(ctx context.Context, prompt string) (string, error) {
		calls++
		if calls == 1 {
			return `{"thinking":"summary tree","node_list":["n1"]}`, nil
		}
		if !strings.Contains(prompt, "summary-only method evidence") {
			t.Fatalf("answer prompt missing summary context: %s", prompt)
		}
		return "summary answer (p. 1)", nil
	}, "p-summary-tree", "what method evidence is available?", kb.QueryOptions{})
	if err != nil {
		t.Fatalf("QueryPaper() error = %v", err)
	}
	if res.Diagnostics.Route != "semantic_tree" {
		t.Fatalf("route = %q, want semantic_tree", res.Diagnostics.Route)
	}
	if calls != 2 {
		t.Fatalf("LLM calls = %d, want 2", calls)
	}
}

func TestQueryPaper_LocalPagesEmptyContextDoesNotCallLLM(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)

	writer := svc.(parsedPaperWriter)
	_, err := writer.AddParsedPaper(context.Background(), kb.Paper{PaperID: "p-empty-local", Title: "Empty Local"}, kb.ParseResult{
		DocType: "pdf",
		Chunks:  []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Title: "Method", Content: ""}},
	}, "not_requested")
	if err != nil {
		t.Fatalf("AddParsedPaper() error = %v", err)
	}

	calls := 0
	res, err := svc.(paperQuerier).QueryPaper(context.Background(), func(ctx context.Context, prompt string) (string, error) {
		calls++
		return "should not call", nil
	}, "p-empty-local", "method", kb.QueryOptions{PreferredRoute: "local_pages"})
	if err != nil {
		t.Fatalf("QueryPaper() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("LLM calls = %d, want 0", calls)
	}
	if res.Diagnostics.Route != "local_pages" {
		t.Fatalf("route = %q, want local_pages", res.Diagnostics.Route)
	}
}

func TestQueryPaper_DeepReadTaskRoute_EnqueueAndReturnStatus(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)

	writer := svc.(parsedPaperWriter)
	_, err := writer.AddParsedPaper(context.Background(), kb.Paper{PaperID: "p-deep", Title: "Deep Paper"}, kb.ParseResult{
		DocType:      "pdf",
		ContentBytes: 400000,
		Chunks:       []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Content: "A long paper."}},
	}, "not_requested")
	if err != nil {
		t.Fatalf("AddParsedPaper() error = %v", err)
	}

	res, err := svc.(paperQuerier).QueryPaper(context.Background(), func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	}, "p-deep", "summarize the whole paper in detail", kb.QueryOptions{AllowTaskCreate: true})
	if err != nil {
		t.Fatalf("QueryPaper() error = %v", err)
	}
	if res.Diagnostics.Route != "deep_read_task" {
		t.Fatalf("route = %q, want deep_read_task", res.Diagnostics.Route)
	}
	if res.Diagnostics.TaskID == "" || res.Diagnostics.TaskStatus == "" {
		t.Fatalf("expected task id/status, got diagnostics=%+v", res.Diagnostics)
	}
}
