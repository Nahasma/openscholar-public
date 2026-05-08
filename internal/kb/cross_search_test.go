package kb_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/db"
	"github.com/Nahasma/openscholar-public/internal/kb"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

type crossQuerier interface {
	CrossPaperSearch(ctx context.Context, callLLM kb.LLMCaller, question string, limit int) (*kb.CrossSearchResult, error)
}

func TestCrossPaperSearch_FindsChunkOnlyPaper(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)
	writer := svc.(interface {
		AddParsedPaper(ctx context.Context, paper kb.Paper, parseResult kb.ParseResult, semanticTreeStatus string) (kb.PaperIndexState, error)
	})

	_, err := writer.AddParsedPaper(context.Background(), kb.Paper{PaperID: "p-chunk", Title: "Chunk Paper"}, kb.ParseResult{
		DocType: "pdf",
		Chunks: []kb.PaperChunk{
			{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Content: "novel sparse mixer architecture"},
		},
	}, "not_requested")
	if err != nil {
		t.Fatalf("AddParsedPaper() error = %v", err)
	}

	res, err := svc.(crossQuerier).CrossPaperSearch(context.Background(), func(ctx context.Context, prompt string) (string, error) {
		return "answer from chunks", nil
	}, "what is sparse mixer architecture?", 3)
	if err != nil {
		t.Fatalf("CrossPaperSearch() error = %v", err)
	}
	if !strings.Contains(res.Answer, "answer from chunks") {
		t.Fatalf("expected chunk-based answer, got: %s", res.Answer)
	}
}

func TestSearchAllPaperChunks_TitleOnlyMatch(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	ctx := context.Background()
	if err := q.InsertPaper(ctx, db.InsertPaperParams{
		PaperID: "p-title",
		Title:   "Title Paper",
	}); err != nil {
		t.Fatalf("InsertPaper() error = %v", err)
	}

	if err := q.InsertPaperChunk(ctx, db.InsertPaperChunkParams{
		PaperID:   "p-title",
		ChunkID:   "chunk_1",
		Kind:      "section",
		Title:     sql.NullString{String: "Sparse Mixer Method", Valid: true},
		Content:   "body does not include keyphrase",
		CreatedAt: 123,
	}); err != nil {
		t.Fatalf("InsertPaperChunk() error = %v", err)
	}

	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM paper_chunks_fts WHERE paper_chunks_fts MATCH ?`, "sparse").Scan(&count); err != nil {
		t.Fatalf("raw title/content MATCH query error = %v", err)
	}
	if count == 0 {
		t.Fatal("expected title-only chunk match")
	}
}

func TestCrossPaperSearch_ReturnsDiagnostics(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)
	writer := svc.(interface {
		AddParsedPaper(ctx context.Context, paper kb.Paper, parseResult kb.ParseResult, semanticTreeStatus string) (kb.PaperIndexState, error)
	})

	_, err := writer.AddParsedPaper(context.Background(), kb.Paper{PaperID: "p-diag", Title: "Diag Paper"}, kb.ParseResult{
		DocType: "pdf",
		Chunks: []kb.PaperChunk{
			{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Title: "Sparse Mixer", Content: "detail text"},
		},
	}, "not_requested")
	if err != nil {
		t.Fatalf("AddParsedPaper() error = %v", err)
	}

	res, err := svc.(crossQuerier).CrossPaperSearch(context.Background(), func(ctx context.Context, prompt string) (string, error) {
		return "diag answer", nil
	}, "sparse", 5)
	if err != nil {
		t.Fatalf("CrossPaperSearch() error = %v", err)
	}
	if res.Diagnostics.SanitizedQuery == "" {
		t.Fatal("expected diagnostics.sanitized_query")
	}
	if len(res.Diagnostics.Channels) != 3 {
		t.Fatalf("expected 3 channels, got %d", len(res.Diagnostics.Channels))
	}
	if res.Diagnostics.FinalRoute == "" {
		t.Fatal("expected diagnostics.final_route")
	}
	if res.Diagnostics.FinalRoute != "no_hits" && len(res.Diagnostics.Papers) == 0 {
		t.Fatal("expected per-paper diagnostics when candidates exist")
	}
}
