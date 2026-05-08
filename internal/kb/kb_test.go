package kb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRRFMerge_SingleList(t *testing.T) {
	list := []RankedItem{
		{PaperID: "p1", NodeID: "n1", Score: 0},
		{PaperID: "p2", NodeID: "n2", Score: 0},
	}
	merged := RRFMerge(60, list)
	require.Len(t, merged, 2)
	// First item should have higher RRF score (1/(60+0+1) > 1/(60+1+1))
	assert.Greater(t, merged[0].Score, merged[1].Score)
}

func TestRRFMerge_MultipleLists(t *testing.T) {
	list1 := []RankedItem{
		{PaperID: "p1", NodeID: "n1"},
		{PaperID: "p2", NodeID: "n2"},
	}
	list2 := []RankedItem{
		{PaperID: "p2", NodeID: "n2"}, // appears in both lists
		{PaperID: "p3", NodeID: "n3"},
	}
	merged := RRFMerge(60, list1, list2)
	require.Len(t, merged, 3)
	// p2:n2 appears in both lists, should have highest combined score
	assert.Equal(t, "p2", merged[0].PaperID)
}

func TestRRFMerge_EmptyLists(t *testing.T) {
	merged := RRFMerge(60)
	assert.Empty(t, merged)
}

func TestRRFMerge_DefaultK(t *testing.T) {
	list := []RankedItem{{PaperID: "p1", NodeID: "n1"}}
	merged := RRFMerge(0, list)
	require.Len(t, merged, 1)
	// With k=60 (default), score = 1/(60+0+1) = 1/61
	expectedScore := 1.0 / 61.0
	assert.InDelta(t, expectedScore, merged[0].Score, 0.0001)
}

func TestFtsToRanked(t *testing.T) {
	ftsResults := []FTSResult{
		{PaperID: "p1", NodeID: "n1", Title: "Test Paper", Snippet: "some text", Rank: -5.0},
		{PaperID: "p2", NodeID: "n2", Title: "Another", Snippet: "more text", Rank: -3.0},
	}
	ranked := ftsToRanked(ftsResults)
	require.Len(t, ranked, 2)
	assert.Equal(t, "p1", ranked[0].PaperID)
	assert.Equal(t, "Test Paper", ranked[0].Title)
	assert.Equal(t, "some text", ranked[0].Snippet)
}

func TestParseTreeJSON(t *testing.T) {
	raw := `{
		"doc_name": "test_paper",
		"doc_description": "A test paper",
		"structure": [
			{
				"node_id": "1",
				"title": "Introduction",
				"start_index": 1,
				"end_index": 3,
				"summary": "Intro summary",
				"nodes": [
					{
						"node_id": "1.1",
						"title": "Background",
						"start_index": 1,
						"end_index": 2,
						"summary": "Background info"
					}
				]
			}
		]
	}`
	tree, err := ParseTreeJSON(raw)
	require.NoError(t, err)
	assert.Equal(t, "test_paper", tree.DocName)
	assert.Equal(t, "A test paper", tree.DocDescription)
	require.Len(t, tree.Structure, 1)
	assert.Equal(t, "Introduction", tree.Structure[0].Title)
	require.Len(t, tree.Structure[0].Nodes, 1)
	assert.Equal(t, "Background", tree.Structure[0].Nodes[0].Title)
}

func TestParseTreeJSON_Invalid(t *testing.T) {
	_, err := ParseTreeJSON("not json")
	assert.Error(t, err)
}

func TestFlattenNodes(t *testing.T) {
	tree := &PaperTree{
		Structure: []TreeNode{
			{
				NodeID: "1", Title: "Intro", StartIndex: 1, EndIndex: 3,
				Nodes: []TreeNode{
					{NodeID: "1.1", Title: "Background", StartIndex: 1, EndIndex: 2},
				},
			},
			{NodeID: "2", Title: "Methods", StartIndex: 4, EndIndex: 6},
		},
	}
	flat := FlattenNodes("paper1", tree)
	assert.Len(t, flat, 3)
	assert.Equal(t, "paper1", flat[0].PaperID)
	assert.Equal(t, "Intro", flat[0].Title)
	assert.Equal(t, "Background", flat[1].Title)
	assert.Equal(t, "Methods", flat[2].Title)
}

func TestFindNodes(t *testing.T) {
	tree := &PaperTree{
		Structure: []TreeNode{
			{
				NodeID: "1", Title: "Intro",
				Nodes: []TreeNode{
					{NodeID: "1.1", Title: "Background"},
					{NodeID: "1.2", Title: "Motivation"},
				},
			},
			{NodeID: "2", Title: "Methods"},
		},
	}
	found := FindNodes(tree, []string{"1.1", "2"})
	assert.Len(t, found, 2)
	titles := map[string]bool{}
	for _, n := range found {
		titles[n.Title] = true
	}
	assert.True(t, titles["Background"])
	assert.True(t, titles["Methods"])
}

func TestFindNodes_NotFound(t *testing.T) {
	tree := &PaperTree{
		Structure: []TreeNode{
			{NodeID: "1", Title: "Intro"},
		},
	}
	found := FindNodes(tree, []string{"nonexistent"})
	assert.Empty(t, found)
}

func TestFormatTreeAsText(t *testing.T) {
	tree := &PaperTree{
		DocDescription: "Test doc",
		Structure: []TreeNode{
			{NodeID: "1", Title: "Intro", StartIndex: 1, EndIndex: 3, Summary: "Introduction section"},
		},
	}
	text := FormatTreeAsText(tree)
	assert.Contains(t, text, "Test doc")
	assert.Contains(t, text, "[1] Intro (pp. 1-3)")
	assert.Contains(t, text, "Introduction section")
}

func TestDefaultIndexOptions(t *testing.T) {
	opts := DefaultIndexOptions()
	assert.True(t, opts.GenerateSummary)
	assert.True(t, opts.GenerateDescription)
	assert.Equal(t, 10, opts.MaxPagesPerNode)
	assert.Equal(t, 20000, opts.MaxTokensPerNode)
}
