package kb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MetaFilter represents parsed metadata filter conditions.
type MetaFilter struct {
	Year     *int   `json:"year,omitempty"`
	YearFrom *int   `json:"year_from,omitempty"`
	YearTo   *int   `json:"year_to,omitempty"`
	Venue    string `json:"venue,omitempty"`
	Author   string `json:"author,omitempty"`
}

// IsEmpty returns true if no filters are set.
func (f *MetaFilter) IsEmpty() bool {
	return f.Year == nil && f.YearFrom == nil && f.YearTo == nil && f.Venue == "" && f.Author == ""
}

// ParseMetaFilter uses LLM to extract metadata filters from a natural language query.
// Returns the filter, the remaining semantic query (metadata parts stripped), and error.
func ParseMetaFilter(ctx context.Context, callLLM LLMCaller, query string) (*MetaFilter, string, error) {
	prompt := fmt.Sprintf(metaFilterPrompt, query)

	response, err := callLLM(ctx, prompt)
	if err != nil {
		return &MetaFilter{}, query, nil // non-fatal: return original query
	}

	var result struct {
		MetaFilter
		SemanticQuery string `json:"semantic_query"`
	}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		// Try to extract JSON from response
		start := strings.Index(response, "{")
		end := strings.LastIndex(response, "}")
		if start >= 0 && end > start {
			_ = json.Unmarshal([]byte(response[start:end+1]), &result)
		}
	}

	semanticQuery := result.SemanticQuery
	if semanticQuery == "" {
		semanticQuery = query
	}

	return &result.MetaFilter, semanticQuery, nil
}

// FilterPapers runs a SQL query with WHERE clauses for year/venue/author.
func (f *FTSSearcher) FilterPapers(ctx context.Context, filter MetaFilter) ([]string, error) {
	var conditions []string
	var args []any

	if filter.Year != nil {
		conditions = append(conditions, "year = ?")
		args = append(args, *filter.Year)
	}
	if filter.YearFrom != nil {
		conditions = append(conditions, "year >= ?")
		args = append(args, *filter.YearFrom)
	}
	if filter.YearTo != nil {
		conditions = append(conditions, "year <= ?")
		args = append(args, *filter.YearTo)
	}
	if filter.Venue != "" {
		conditions = append(conditions, "venue LIKE ?")
		args = append(args, "%"+filter.Venue+"%")
	}
	if filter.Author != "" {
		conditions = append(conditions, "authors LIKE ?")
		args = append(args, "%"+filter.Author+"%")
	}

	if len(conditions) == 0 {
		return nil, nil
	}

	query := "SELECT paper_id FROM papers WHERE " + strings.Join(conditions, " AND ")
	rows, err := f.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("metadata filter: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Ensure f.db is accessible (uses the same *sql.DB from FTSSearcher)
var _ = (*FTSSearcher)(nil)

const metaFilterPrompt = `Extract metadata filters from this research query. Return JSON only.

Query: "%s"

Extract any of these filters if present:
- year: exact year (integer)
- year_from: start year for ranges (integer)
- year_to: end year for ranges (integer)
- venue: conference or journal name (string)
- author: author name (string)
- semantic_query: the remaining query after removing metadata parts (string)

Example: "2024 NeurIPS papers on diffusion" → {"year": 2024, "venue": "NeurIPS", "semantic_query": "diffusion"}

If no metadata filters found, return: {"semantic_query": "<original query>"}

JSON:`
