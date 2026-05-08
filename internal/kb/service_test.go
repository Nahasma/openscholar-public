package kb_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nahasma/openscholar-public/internal/kb"
	"github.com/Nahasma/openscholar-public/internal/testutil"
)

func newService(t *testing.T) kb.Service {
	t.Helper()
	conn, q := testutil.SetupTestDB(t)
	return kb.NewService(q, conn)
}

func samplePaper() (kb.Paper, kb.PaperTree) {
	paper := kb.Paper{
		PaperID:  "paper-001",
		Title:    "Attention Is All You Need",
		Authors:  []kb.Author{{Name: "Vaswani"}, {Name: "Shazeer"}},
		Year:     2017,
		Venue:    "NeurIPS",
		Abstract: "We propose a new architecture...",
		DOI:      "10.5555/3295222.3295349",
	}
	tree := kb.PaperTree{
		DocName:        "attention_paper",
		DocDescription: "Transformer architecture paper",
		Structure: []kb.TreeNode{
			{
				NodeID: "1", Title: "Introduction", StartIndex: 1, EndIndex: 3,
				Summary: "Introduction to attention mechanisms",
				Nodes: []kb.TreeNode{
					{NodeID: "1.1", Title: "Background", StartIndex: 1, EndIndex: 2, Summary: "Background info"},
				},
			},
			{NodeID: "2", Title: "Methods", StartIndex: 4, EndIndex: 8, Summary: "Transformer architecture"},
		},
	}
	return paper, tree
}

func TestKBService_AddAndGetPaper(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	paper, tree := samplePaper()
	err := svc.AddPaper(ctx, paper, tree)
	require.NoError(t, err)

	got, err := svc.GetPaper(ctx, "paper-001")
	require.NoError(t, err)
	assert.Equal(t, "Attention Is All You Need", got.Title)
	assert.Len(t, got.Authors, 2)
	assert.Equal(t, 2017, got.Year)
	assert.Equal(t, "NeurIPS", got.Venue)
}

func TestKBService_GetPaper_NotFound(t *testing.T) {
	svc := newService(t)
	_, err := svc.GetPaper(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestKBService_GetTree(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	paper, tree := samplePaper()
	svc.AddPaper(ctx, paper, tree)

	got, err := svc.GetTree(ctx, "paper-001")
	require.NoError(t, err)
	assert.Equal(t, "Transformer architecture paper", got.DocDescription)
	require.Len(t, got.Structure, 2)
	assert.Equal(t, "Introduction", got.Structure[0].Title)
	assert.Len(t, got.Structure[0].Nodes, 1)
}

func TestKBService_ListPapers(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	for i := range 5 {
		p := kb.Paper{PaperID: fmt.Sprintf("paper-%d", i), Title: fmt.Sprintf("Paper %d", i)}
		svc.AddPaper(ctx, p, kb.PaperTree{})
	}

	// Limit 3, offset 0
	list, err := svc.ListPapers(ctx, 3, 0)
	require.NoError(t, err)
	assert.Len(t, list, 3)

	// Limit 3, offset 3
	list, err = svc.ListPapers(ctx, 3, 3)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestKBService_SearchByTitle(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	svc.AddPaper(ctx, kb.Paper{PaperID: "p1", Title: "Deep Learning Basics"}, kb.PaperTree{})
	svc.AddPaper(ctx, kb.Paper{PaperID: "p2", Title: "Transformer Architecture"}, kb.PaperTree{})
	svc.AddPaper(ctx, kb.Paper{PaperID: "p3", Title: "Deep Reinforcement Learning"}, kb.PaperTree{})

	results, err := svc.SearchByTitle(ctx, "Deep", 10)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestKBService_RemovePaper(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	paper, tree := samplePaper()
	svc.AddPaper(ctx, paper, tree)

	err := svc.RemovePaper(ctx, "paper-001")
	require.NoError(t, err)

	_, err = svc.GetPaper(ctx, "paper-001")
	assert.Error(t, err)
}

func TestKBService_CountPapers(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	count, _ := svc.CountPapers(ctx)
	assert.Equal(t, int64(0), count)

	svc.AddPaper(ctx, kb.Paper{PaperID: "p1", Title: "Paper 1"}, kb.PaperTree{})
	svc.AddPaper(ctx, kb.Paper{PaperID: "p2", Title: "Paper 2"}, kb.PaperTree{})

	count, err := svc.CountPapers(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestKBService_FindByDOI(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	paper, tree := samplePaper()
	svc.AddPaper(ctx, paper, tree)

	got, err := svc.FindByDOI(ctx, "10.5555/3295222.3295349")
	require.NoError(t, err)
	assert.Equal(t, "Attention Is All You Need", got.Title)
}

func TestKBService_FindByArxivID(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	paper := kb.Paper{PaperID: "p-arxiv", Title: "Arxiv Paper", ArxivID: "2301.12345"}
	svc.AddPaper(ctx, paper, kb.PaperTree{})

	got, err := svc.FindByArxivID(ctx, "2301.12345")
	require.NoError(t, err)
	assert.Equal(t, "Arxiv Paper", got.Title)
}

func TestKBService_GetNodeSummaries(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	paper, tree := samplePaper()
	svc.AddPaper(ctx, paper, tree)

	summaries, err := svc.GetNodeSummaries(ctx, "paper-001")
	require.NoError(t, err)
	// 3 nodes: Introduction(1), Background(1.1), Methods(2)
	assert.Len(t, summaries, 3)
}

func TestKBService_AddIndexedPaperPersistsIndexMetadata(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	indexed, ok := svc.(interface {
		AddIndexedPaper(context.Context, kb.Paper, kb.IndexResult) error
		GetIndexMetadata(context.Context, string) (kb.IndexMetadata, error)
	})
	require.True(t, ok)

	result := kb.IndexResult{
		Tree: kb.PaperTree{
			DocName: "degraded",
			Structure: []kb.TreeNode{{
				NodeID: "page_1", Title: "Page 1", StartIndex: 1, EndIndex: 1,
				Summary: "front summary", Content: "front summary plus full content",
			}},
		},
		ModelUsed:        "simple_extract",
		IndexLevel:       kb.IndexLevelSimpleFullText,
		Extractor:        "simple_extract",
		FallbackReason:   "PageIndex failed: LibreSSL 2.8.3",
		ContentAvailable: true,
		ContentBytes:     31,
	}
	err := indexed.AddIndexedPaper(ctx, kb.Paper{PaperID: "idx-meta", Title: "Indexed Metadata"}, result)
	require.NoError(t, err)

	meta, err := indexed.GetIndexMetadata(ctx, "idx-meta")
	require.NoError(t, err)
	assert.Equal(t, kb.IndexLevelSimpleFullText, meta.IndexLevel)
	assert.Equal(t, "simple_extract", meta.Extractor)
	assert.Equal(t, "PageIndex failed: LibreSSL 2.8.3", meta.FallbackReason)
	assert.True(t, meta.ContentAvailable)
	assert.Greater(t, meta.ContentBytes, 0)
}

func TestKBService_AddIndexedPaperWritesRawCompatibilityState(t *testing.T) {
	svc, conn := newServiceWithRawDB(t)
	ctx := context.Background()

	indexed, ok := svc.(interface {
		AddIndexedPaper(context.Context, kb.Paper, kb.IndexResult) error
	})
	require.True(t, ok)

	result := kb.IndexResult{
		Tree: kb.PaperTree{
			DocName: "full-tree",
			Structure: []kb.TreeNode{{
				NodeID: "n1", Title: "Method", StartIndex: 1, EndIndex: 1,
				Summary: "method summary", Content: "full raw method content",
			}},
		},
		ModelUsed:        "pageindex",
		IndexLevel:       kb.IndexLevelFullTree,
		Extractor:        "pageindex",
		ContentAvailable: true,
		TreeAvailable:    true,
	}
	require.NoError(t, indexed.AddIndexedPaper(ctx, kb.Paper{PaperID: "idx-compat", Title: "Compat"}, result))

	assert.Equal(t, 1, countRows(t, conn, "paper_chunks", "paper_id", "idx-compat"))
	assert.Equal(t, 1, countRows(t, conn, "paper_index_states", "paper_id", "idx-compat"))

	var semanticAvailable int
	require.NoError(t, conn.QueryRow(`SELECT semantic_tree_available FROM paper_index_states WHERE paper_id = ?`, "idx-compat").Scan(&semanticAvailable))
	assert.Equal(t, 1, semanticAvailable)
}

func TestKBService_AddPaperSummaryTreeIsSemanticReady(t *testing.T) {
	svc, conn := newServiceWithRawDB(t)
	ctx := context.Background()

	tree := kb.PaperTree{
		DocName: "summary-tree",
		Structure: []kb.TreeNode{{
			NodeID: "n1", Title: "Method", StartIndex: 1, EndIndex: 1,
			Summary: "summary-only method evidence",
		}},
	}
	require.NoError(t, svc.AddPaper(ctx, kb.Paper{PaperID: "summary-compat", Title: "Summary Compat"}, tree))

	var semanticStatus string
	var semanticAvailable int
	require.NoError(t, conn.QueryRow(`SELECT semantic_tree_status, semantic_tree_available FROM paper_index_states WHERE paper_id = ?`, "summary-compat").Scan(&semanticStatus, &semanticAvailable))
	assert.Equal(t, "ready", semanticStatus)
	assert.Equal(t, 1, semanticAvailable)
	assert.Equal(t, 1, countRows(t, conn, "paper_chunks", "paper_id", "summary-compat"))
}

func TestKBService_IndexMetadataReconcilesMissingContentRows(t *testing.T) {
	svc, conn := newServiceWithRawDB(t)
	ctx := context.Background()

	indexed, ok := svc.(interface {
		AddIndexedPaper(context.Context, kb.Paper, kb.IndexResult) error
		GetIndexMetadata(context.Context, string) (kb.IndexMetadata, error)
	})
	require.True(t, ok)

	result := kb.IndexResult{
		Tree: kb.PaperTree{
			DocName: "content-missing",
			Structure: []kb.TreeNode{{
				NodeID: "page_1", Title: "Page 1", StartIndex: 1, EndIndex: 1,
				Summary: "front summary", Content: "persisted content",
			}},
		},
		ModelUsed:        "simple_extract",
		IndexLevel:       kb.IndexLevelSimpleFullText,
		Extractor:        "simple_extract",
		ContentAvailable: true,
		ContentBytes:     17,
	}
	require.NoError(t, indexed.AddIndexedPaper(ctx, kb.Paper{PaperID: "idx-missing", Title: "Missing Content"}, result))
	_, err := conn.ExecContext(ctx, "DELETE FROM node_contents WHERE paper_id = ?", "idx-missing")
	require.NoError(t, err)

	meta, err := indexed.GetIndexMetadata(ctx, "idx-missing")
	require.NoError(t, err)
	assert.False(t, meta.ContentAvailable)
	assert.Equal(t, 0, meta.ContentBytes)
	assert.Contains(t, meta.MissingCapabilities, "full_content")
}
