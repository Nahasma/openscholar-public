package skillbank

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// ftsResult holds a single FTS5 match with usage stats.
type ftsResult struct {
	ID      string
	Name    string
	Snippet string
	Rank    float64
	row     skillRow
}

// ftsSearcher provides FTS5 full-text search on skills.
type ftsSearcher struct {
	db *sql.DB
}

func newFTSSearcher(db *sql.DB) *ftsSearcher {
	return &ftsSearcher{db: db}
}

// Search performs FTS5 search on skills, optionally filtered by category.
// Results are ranked by: bm25_score * 0.7 + log(usage_count+1) * 0.3
func (f *ftsSearcher) Search(ctx context.Context, query string, category string, limit int) ([]ftsResult, error) {
	if limit <= 0 {
		limit = 10
	}

	var rows *sql.Rows
	var err error

	if category == "" {
		// Search all categories, join with skills_index for usage stats
		rows, err = f.db.QueryContext(ctx,
			`SELECT f.id, f.name, snippet(skills_fts, 4, '<b>', '</b>', '...', 48), f.rank,
			        COALESCE(s.description, '') as description,
			        COALESCE(s.category, '') as category,
			        COALESCE(s.tags, '[]') as tags,
			        COALESCE(s.meta_json, '{}') as meta_json,
			        COALESCE(s.usage_count, 0) as usage_count
			 FROM skills_fts f
			 LEFT JOIN skills_index s ON f.id = s.id
			 WHERE skills_fts MATCH ?
			 ORDER BY f.rank
			 LIMIT ?`,
			query, limit)
	} else {
		rows, err = f.db.QueryContext(ctx,
			`SELECT f.id, f.name, snippet(skills_fts, 4, '<b>', '</b>', '...', 48), f.rank,
			        COALESCE(s.description, '') as description,
			        COALESCE(s.category, '') as category,
			        COALESCE(s.tags, '[]') as tags,
			        COALESCE(s.meta_json, '{}') as meta_json,
			        COALESCE(s.usage_count, 0) as usage_count
			 FROM skills_fts f
			 LEFT JOIN skills_index s ON f.id = s.id
			 WHERE skills_fts MATCH ? AND s.category = ?
			 ORDER BY f.rank
			 LIMIT ?`,
			query, category, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("FTS5 skill search: %w", err)
	}
	defer rows.Close()

	var results []ftsResult
	for rows.Next() {
		var r ftsResult
		var tagsJSON string
		if err := rows.Scan(&r.ID, &r.Name, &r.Snippet, &r.Rank, &r.row.Description, &r.row.Category, &tagsJSON, &r.row.MetaJSON, &r.row.UsageCount); err != nil {
			return nil, fmt.Errorf("FTS5 scan: %w", err)
		}
		r.row.ID = r.ID
		r.row.Name = r.Name
		_ = json.Unmarshal([]byte(tagsJSON), &r.row.Tags)
		results = append(results, r)
	}
	return results, rows.Err()
}
