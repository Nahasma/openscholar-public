package session

import (
	"context"
	"sort"
	"strings"
)

// SearchResult represents a matched session with relevance score.
type SearchResult struct {
	Session Session
	Score   float64
	Snippet string // matching context summary
}

// SearchOptions configures session search behavior.
type SearchOptions struct {
	Query   string
	Limit   int // final result count (default 10)
	MaxScan int // keyword pre-filter candidate count (default 100)
}

// SearchService provides agentic session search capabilities.
type SearchService interface {
	Search(ctx context.Context, opts SearchOptions) ([]SearchResult, error)
}

// SideQuery is a callback for LLM-based semantic reranking.
// Takes a formatted candidate list and query, returns ranked JSON or empty on failure.
type SideQuery func(ctx context.Context, candidates string, query string) (string, error)

type searchService struct {
	sessions  Service
	sideQuery SideQuery
}

// NewSearchService creates a SearchService.
// sideQuery can be nil (keyword-only mode).
func NewSearchService(sessions Service, sideQuery SideQuery) SearchService {
	return &searchService{
		sessions:  sessions,
		sideQuery: sideQuery,
	}
}

// Search finds sessions matching the query using keyword pre-filter + optional LLM rerank.
func (s *searchService) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 10
	}
	if opts.MaxScan <= 0 {
		opts.MaxScan = 100
	}

	// Step 1: Get recent sessions
	allSessions, err := s.sessions.List(ctx)
	if err != nil {
		return nil, err
	}

	// Step 2: Keyword pre-filter
	query := strings.ToLower(opts.Query)
	keywords := strings.Fields(query)

	type scored struct {
		session Session
		score   float64
	}
	var candidates []scored

	for _, sess := range allSessions {
		title := strings.ToLower(sess.Title)
		score := 0.0
		for _, kw := range keywords {
			if strings.Contains(title, kw) {
				score += 1.0
			}
		}
		// Also check Summary, Tags, FirstPrompt, GitBranch if present
		if sess.Summary != "" && strings.Contains(strings.ToLower(sess.Summary), query) {
			score += 2.0
		}
		if sess.FirstPrompt != "" {
			fp := strings.ToLower(sess.FirstPrompt)
			for _, kw := range keywords {
				if strings.Contains(fp, kw) {
					score += 0.5
				}
			}
		}
		if score > 0 {
			candidates = append(candidates, scored{session: sess, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Limit candidates
	if len(candidates) > opts.MaxScan {
		candidates = candidates[:opts.MaxScan]
	}

	// Step 3: Build results (skip LLM rerank for now — Phase 5 baseline)
	results := make([]SearchResult, 0, min(opts.Limit, len(candidates)))
	for i, c := range candidates {
		if i >= opts.Limit {
			break
		}
		snippet := c.session.Title
		if c.session.Summary != "" {
			snippet = c.session.Summary
		}
		results = append(results, SearchResult{
			Session: c.session,
			Score:   c.score,
			Snippet: snippet,
		})
	}

	return results, nil
}
