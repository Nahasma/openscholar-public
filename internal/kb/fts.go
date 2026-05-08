package kb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// FTSSearcher provides FTS5 full-text search on papers and node summaries.
type FTSSearcher struct {
	db *sql.DB
}

// NewFTSSearcher creates a new FTS5 searcher using a raw DB connection.
func NewFTSSearcher(db *sql.DB) *FTSSearcher {
	return &FTSSearcher{db: db}
}

// FTSResult holds a single FTS5 match.
type FTSResult struct {
	PaperID string
	NodeID  string // empty for paper-level matches
	Title   string
	Snippet string
	Rank    float64 // BM25 score (lower = better match)
}

// SearchPapers performs FTS5 full-text search on papers (title, abstract, authors).
func (f *FTSSearcher) SearchPapers(ctx context.Context, query string, limit int) ([]FTSResult, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := f.db.QueryContext(ctx,
		`SELECT paper_id, title, snippet(papers_fts, 2, '<b>', '</b>', '...', 32), rank
		 FROM papers_fts WHERE papers_fts MATCH ? ORDER BY rank LIMIT ?`,
		query, limit)
	if err != nil {
		return nil, fmt.Errorf("FTS5 paper search: %w", err)
	}
	defer rows.Close()

	var results []FTSResult
	for rows.Next() {
		var r FTSResult
		if err := rows.Scan(&r.PaperID, &r.Title, &r.Snippet, &r.Rank); err != nil {
			return nil, fmt.Errorf("FTS5 scan: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// SearchNodes performs FTS5 full-text search on node summaries (title, summary).
func (f *FTSSearcher) SearchNodes(ctx context.Context, query string, limit int) ([]FTSResult, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := f.db.QueryContext(ctx,
		`SELECT paper_id, node_id, title, snippet(node_summaries_fts, 3, '<b>', '</b>', '...', 64), rank
		 FROM node_summaries_fts WHERE node_summaries_fts MATCH ? ORDER BY rank LIMIT ?`,
		query, limit)
	if err != nil {
		return nil, fmt.Errorf("FTS5 node search: %w", err)
	}
	defer rows.Close()

	var results []FTSResult
	for rows.Next() {
		var r FTSResult
		if err := rows.Scan(&r.PaperID, &r.NodeID, &r.Title, &r.Snippet, &r.Rank); err != nil {
			return nil, fmt.Errorf("FTS5 scan: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// FilterByPaperIDs performs FTS5 node search restricted to specific paper IDs.
func (f *FTSSearcher) FilterByPaperIDs(ctx context.Context, query string, paperIDs []string, limit int) ([]FTSResult, error) {
	if len(paperIDs) == 0 {
		return f.SearchNodes(ctx, query, limit)
	}
	if limit <= 0 {
		limit = 50
	}

	// Build IN clause
	placeholders := ""
	args := []any{query}
	for i, id := range paperIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, id)
	}
	args = append(args, limit)

	rows, err := f.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT paper_id, node_id, title, snippet(node_summaries_fts, 3, '<b>', '</b>', '...', 64), rank
		 FROM node_summaries_fts WHERE node_summaries_fts MATCH ? AND paper_id IN (%s) ORDER BY rank LIMIT ?`, placeholders),
		args...)
	if err != nil {
		return nil, fmt.Errorf("FTS5 filtered search: %w", err)
	}
	defer rows.Close()

	var results []FTSResult
	for rows.Next() {
		var r FTSResult
		if err := rows.Scan(&r.PaperID, &r.NodeID, &r.Title, &r.Snippet, &r.Rank); err != nil {
			return nil, fmt.Errorf("FTS5 scan: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// QueryTerms extracts stable alphanumeric query terms for deterministic FTS sanitization.
func QueryTerms(question string) []string {
	fields := strings.FieldsFunc(strings.ToLower(question), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	seen := make(map[string]struct{})
	var terms []string
	for _, field := range fields {
		if len(field) < 3 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		terms = append(terms, field)
	}
	return terms
}

// SanitizeFTSQuery builds a bounded OR query safe for SQLite FTS MATCH.
func SanitizeFTSQuery(question string) string {
	terms := QueryTerms(question)
	if len(terms) == 0 {
		return ""
	}
	if len(terms) > 12 {
		terms = terms[:12]
	}
	quoted := make([]string, len(terms))
	for i, term := range terms {
		quoted[i] = `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " OR ")
}
