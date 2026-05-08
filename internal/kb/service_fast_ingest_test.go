package kb_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nahasma/openscholar-public/internal/kb"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

func TestAddParsedPaper_WritesChunksAndStateTransactionally(t *testing.T) {
	svc, conn := newServiceWithRawDB(t)
	ctx := context.Background()

	writer, ok := svc.(interface {
		AddParsedPaper(context.Context, kb.Paper, kb.ParseResult, string) (kb.PaperIndexState, error)
	})
	require.True(t, ok)

	paper := kb.Paper{
		PaperID:  "fast-ingest-001",
		Title:    "Fast Ingest Paper",
		FilePath: "/tmp/fast-ingest-001.pdf",
		DocType:  "pdf",
	}
	parseResult := kb.ParseResult{
		PaperTitle: "Fast Ingest Paper",
		DocType:    "pdf",
		Chunks: []kb.PaperChunk{
			{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Content: "intro content", TokenCount: 2, Source: "simple_extract"},
			{ChunkID: "page_2", Kind: "page", PageStart: 2, PageEnd: 2, Content: "method content", TokenCount: 2, Source: "simple_extract"},
		},
		Extractor: "simple_extract",
	}

	state, err := writer.AddParsedPaper(ctx, paper, parseResult, "not_requested")
	require.NoError(t, err)
	assert.Equal(t, "fast-ingest-001", state.PaperID)
	assert.Equal(t, kb.IndexLevelSimpleFullText, state.IndexLevel)
	assert.Equal(t, "ready", state.RawParseStatus)
	assert.Equal(t, "ready", state.FTSStatus)
	assert.Equal(t, "not_requested", state.SemanticTreeStatus)
	assert.True(t, state.ContentAvailable)
	assert.True(t, state.FlatPageIndexAvailable)

	assert.Equal(t, 1, countRows(t, conn, "papers", "paper_id", "fast-ingest-001"))
	assert.Equal(t, 2, countRows(t, conn, "paper_chunks", "paper_id", "fast-ingest-001"))
	assert.Equal(t, 1, countRows(t, conn, "paper_index_states", "paper_id", "fast-ingest-001"))
}

func TestAddParsedPaper_WritesWithoutRawDBHandle(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q)
	ctx := context.Background()

	writer, ok := svc.(interface {
		AddParsedPaper(context.Context, kb.Paper, kb.ParseResult, string) (kb.PaperIndexState, error)
	})
	require.True(t, ok)

	_, err := writer.AddParsedPaper(ctx, kb.Paper{PaperID: "fast-no-raw", Title: "No Raw DB"}, kb.ParseResult{
		Chunks: []kb.PaperChunk{{ChunkID: "page_1", Kind: "page", PageStart: 1, PageEnd: 1, Content: "persisted content"}},
	}, "not_requested")
	require.NoError(t, err)

	assert.Equal(t, 1, countRows(t, conn, "papers", "paper_id", "fast-no-raw"))
	assert.Equal(t, 1, countRows(t, conn, "paper_chunks", "paper_id", "fast-no-raw"))
	assert.Equal(t, 1, countRows(t, conn, "paper_index_states", "paper_id", "fast-no-raw"))
}
