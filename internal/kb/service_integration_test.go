package kb_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openscholar/openscholar/internal/kb"
	"github.com/openscholar/openscholar/internal/testutil"
)

// TestKBSearch_FTS5Query_Integration verifies that, after adding a paper, the
// FTS5 full-text search index picks up the content and returns matching results.
func TestKBSearch_FTS5Query_Integration(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)
	fts := kb.NewFTSSearcher(conn)
	ctx := context.Background()

	// Add a paper with distinctive title and abstract terms.
	paper := kb.Paper{
		PaperID:  "fts-001",
		Title:    "Neural Scaling Laws",
		Authors:  []kb.Author{{Name: "Kaplan, J."}, {Name: "McCandlish, S."}},
		Year:     2020,
		Venue:    "arXiv",
		Abstract: "We study empirical scaling laws for language model performance.",
	}
	tree := kb.PaperTree{
		DocName:        "scaling_laws",
		DocDescription: "Scaling laws for LLMs",
		Structure: []kb.TreeNode{
			{
				NodeID:     "1",
				Title:      "Introduction",
				StartIndex: 1,
				EndIndex:   3,
				Summary:    "Overview of neural scaling phenomena",
			},
			{
				NodeID:     "2",
				Title:      "Empirical Results",
				StartIndex: 4,
				EndIndex:   10,
				Summary:    "Scaling exponents for compute, data and parameters",
			},
		},
	}
	require.NoError(t, svc.AddPaper(ctx, paper, tree))

	// FTS5 search on paper title/abstract should find the paper.
	paperResults, err := fts.SearchPapers(ctx, "scaling", 10)
	require.NoError(t, err)
	require.NotEmpty(t, paperResults, "FTS5 should find papers matching 'scaling'")

	found := false
	for _, r := range paperResults {
		if r.PaperID == "fts-001" {
			found = true
			assert.Equal(t, "Neural Scaling Laws", r.Title)
			break
		}
	}
	assert.True(t, found, "paper fts-001 should be in FTS5 results")

	// FTS5 search on node summaries should also return matching nodes.
	nodeResults, err := fts.SearchNodes(ctx, "empirical", 10)
	require.NoError(t, err)
	require.NotEmpty(t, nodeResults, "FTS5 should find nodes matching 'empirical'")

	nodeFound := false
	for _, r := range nodeResults {
		if r.PaperID == "fts-001" {
			nodeFound = true
			break
		}
	}
	assert.True(t, nodeFound, "node results should include paper fts-001")
}

// TestKBSearch_FTS5Query_NoMatch verifies that searching for a term that does
// not appear in any indexed paper returns an empty slice (not an error).
func TestKBSearch_FTS5Query_NoMatch(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	fts := kb.NewFTSSearcher(conn)
	svc := kb.NewService(q, conn)
	ctx := context.Background()

	require.NoError(t, svc.AddPaper(ctx, kb.Paper{
		PaperID:  "fts-002",
		Title:    "Quantum Computing Basics",
		Abstract: "An introduction to quantum gates.",
	}, kb.PaperTree{}))

	results, err := fts.SearchPapers(ctx, "nonexistenttermxyz", 10)
	require.NoError(t, err)
	assert.Empty(t, results, "no results expected for unknown query term")
}

// TestKBSearch_FTS5_FilterByPaperIDs verifies that FilterByPaperIDs restricts
// node-level results to the specified set of paper IDs.
func TestKBSearch_FTS5_FilterByPaperIDs(t *testing.T) {
	conn, q := testutil.SetupTestDB(t)
	svc := kb.NewService(q, conn)
	fts := kb.NewFTSSearcher(conn)
	ctx := context.Background()

	// Add two papers, both with nodes mentioning "transformer".
	for _, pid := range []string{"filter-a", "filter-b"} {
		err := svc.AddPaper(ctx, kb.Paper{
			PaperID:  pid,
			Title:    "Transformer Paper " + pid,
			Abstract: "Transformers are ubiquitous in NLP.",
		}, kb.PaperTree{
			Structure: []kb.TreeNode{
				{
					NodeID:     "1",
					Title:      "Background",
					StartIndex: 1,
					EndIndex:   2,
					Summary:    "Transformer attention mechanism explained",
				},
			},
		})
		require.NoError(t, err)
	}

	// Filter to only paper "filter-a".
	results, err := fts.FilterByPaperIDs(ctx, "transformer", []string{"filter-a"}, 10)
	require.NoError(t, err)
	for _, r := range results {
		assert.Equal(t, "filter-a", r.PaperID,
			"FilterByPaperIDs should only return nodes for the specified paper")
	}
}
