package research

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoveltyCheck_Low(t *testing.T) {
	searchFunc := func(ctx context.Context, query string) ([]SearchResult, error) {
		return []SearchResult{
			{Title: "Some Paper", Source: "scholar"},
		}, nil
	}
	evaluateFunc := func(ctx context.Context, design string, paper SearchResult) (*SimilarPaper, error) {
		return &SimilarPaper{
			Title:           paper.Title,
			Source:          paper.Source,
			SimilarityScore: 0.3,
		}, nil
	}

	result, err := RunNoveltyCheck(context.Background(), "my design", []string{"keyword"},
		searchFunc, evaluateFunc, 0.7, 0.85)
	require.NoError(t, err)
	assert.Equal(t, "low", result.Risk)
	assert.Len(t, result.SimilarPapers, 0) // below threshold
}

func TestNoveltyCheck_Medium(t *testing.T) {
	searchFunc := func(ctx context.Context, query string) ([]SearchResult, error) {
		return []SearchResult{
			{Title: "Similar Paper", Source: "arxiv"},
		}, nil
	}
	evaluateFunc := func(ctx context.Context, design string, paper SearchResult) (*SimilarPaper, error) {
		return &SimilarPaper{
			Title:           paper.Title,
			Source:          paper.Source,
			SimilarityScore: 0.75,
			OverlapAreas:    []string{"method"},
		}, nil
	}

	result, err := RunNoveltyCheck(context.Background(), "my design", []string{"keyword"},
		searchFunc, evaluateFunc, 0.7, 0.85)
	require.NoError(t, err)
	assert.Equal(t, "medium", result.Risk)
	assert.Len(t, result.SimilarPapers, 1)
}

func TestNoveltyCheck_High(t *testing.T) {
	searchFunc := func(ctx context.Context, query string) ([]SearchResult, error) {
		return []SearchResult{
			{Title: "Very Similar Paper", Source: "kb"},
		}, nil
	}
	evaluateFunc := func(ctx context.Context, design string, paper SearchResult) (*SimilarPaper, error) {
		return &SimilarPaper{
			Title:           paper.Title,
			Source:          paper.Source,
			SimilarityScore: 0.9,
			OverlapAreas:    []string{"method", "dataset", "approach"},
		}, nil
	}

	result, err := RunNoveltyCheck(context.Background(), "my design", []string{"keyword"},
		searchFunc, evaluateFunc, 0.7, 0.85)
	require.NoError(t, err)
	assert.Equal(t, "high", result.Risk)
}

func TestNoveltyCheck_NoKeywords(t *testing.T) {
	result, err := RunNoveltyCheck(context.Background(), "design", nil, nil, nil, 0.7, 0.85)
	require.NoError(t, err)
	assert.Equal(t, "low", result.Risk)
}

func TestNoveltyCheck_Deduplication(t *testing.T) {
	callCount := 0
	searchFunc := func(ctx context.Context, query string) ([]SearchResult, error) {
		return []SearchResult{
			{Title: "Same Paper", Source: "scholar"},
		}, nil
	}
	evaluateFunc := func(ctx context.Context, design string, paper SearchResult) (*SimilarPaper, error) {
		callCount++
		return &SimilarPaper{
			Title:           paper.Title,
			SimilarityScore: 0.5,
		}, nil
	}

	// 2 keywords both return "Same Paper" → should deduplicate
	result, err := RunNoveltyCheck(context.Background(), "design", []string{"kw1", "kw2"},
		searchFunc, evaluateFunc, 0.7, 0.85)
	require.NoError(t, err)
	assert.Equal(t, "low", result.Risk)
	assert.Equal(t, 1, callCount) // only evaluated once due to dedup
}

func TestWriteNoveltyReport(t *testing.T) {
	dir := t.TempDir()
	result := &NoveltyResult{
		Risk:   "medium",
		Report: "# Novelty Report\n\nSome findings.",
	}
	err := WriteNoveltyReport(dir, result)
	require.NoError(t, err)

	// Verify file exists
	assert.FileExists(t, dir+"/.handoff/02-novelty-check.md")
}

func TestGenerateNoveltyReport(t *testing.T) {
	result := &NoveltyResult{
		Risk: "high",
		SimilarPapers: []SimilarPaper{
			{Title: "Paper A", Source: "scholar", SimilarityScore: 0.9, OverlapAreas: []string{"method"}},
		},
	}
	result.Report = generateNoveltyReport(result)
	assert.Contains(t, result.Report, "high")
	assert.Contains(t, result.Report, "Paper A")
	assert.Contains(t, result.Report, "0.90")
}
