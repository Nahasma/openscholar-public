package kb

import (
	"context"
	"fmt"
	"time"

	"github.com/Nahasma/openscholar-public/internal/db"
)

// KBStats aggregates knowledge base usage statistics.
type KBStats struct {
	TotalPapers  int64         `json:"total_papers"`
	TotalNodes   int64         `json:"total_nodes"`
	TotalQueries int64         `json:"total_queries"`
	TopPapers    []PaperAccess `json:"top_papers"`
	Relations    []PaperRelationInfo `json:"relations"`
}

// PaperAccess tracks access frequency for a paper.
type PaperAccess struct {
	PaperID     string `json:"paper_id"`
	Title       string `json:"title"`
	AccessCount int64  `json:"access_count"`
}

// PaperRelationInfo describes a relationship between two papers.
type PaperRelationInfo struct {
	PaperIDA     string  `json:"paper_id_a"`
	PaperIDB     string  `json:"paper_id_b"`
	RelationType string  `json:"relation_type"`
	Weight       float64 `json:"weight"`
}

// GetStats returns aggregated KB statistics.
func (s *service) GetStats(ctx context.Context) (*KBStats, error) {
	stats := &KBStats{}

	// Total papers
	count, err := s.q.CountPapers(ctx)
	if err == nil {
		stats.TotalPapers = count
	}

	// Total nodes
	nodeCount, err := s.q.CountNodeSummaries(ctx)
	if err == nil {
		stats.TotalNodes = nodeCount
	}

	// Total queries
	queryCount, err := s.q.CountKBUsageStats(ctx)
	if err == nil {
		stats.TotalQueries = queryCount
	}

	// Top accessed papers
	topRows, err := s.q.GetTopAccessedPapers(ctx, 10)
	if err == nil {
		for _, row := range topRows {
			pid := row.PaperID.String
			title := ""
			if paper, err := s.GetPaper(ctx, pid); err == nil {
				title = paper.Title
			}
			stats.TopPapers = append(stats.TopPapers, PaperAccess{
				PaperID:     pid,
				Title:       title,
				AccessCount: row.AccessCount,
			})
		}
	}

	return stats, nil
}

// RecordUsage inserts a usage stat record.
func (s *service) RecordUsage(ctx context.Context, paperID, nodeID, query, searchType string) {
	_ = s.q.InsertUsageStat(ctx, db.InsertUsageStatParams{
		PaperID:    toNullString(paperID),
		NodeID:     toNullString(nodeID),
		Query:      query,
		SearchType: searchType,
		CreatedAt:  time.Now().Unix(),
	})
}

// DiscoverRelations analyzes co-access patterns and creates/updates paper relations.
func (s *service) DiscoverRelations(ctx context.Context) error {
	// Get top papers
	topPapers, err := s.q.GetTopAccessedPapers(ctx, 50)
	if err != nil {
		return fmt.Errorf("get top papers: %w", err)
	}

	now := time.Now().Unix()
	for _, paper := range topPapers {
		pid := paper.PaperID.String
		if pid == "" {
			continue
		}
		// Find co-accessed papers (papers queried in the same queries)
		rows, err := s.db.QueryContext(ctx,
			`SELECT DISTINCT b.paper_id FROM kb_usage_stats a
			 JOIN kb_usage_stats b ON a.query = b.query AND a.paper_id != b.paper_id
			 WHERE a.paper_id = ? AND b.paper_id IS NOT NULL LIMIT 20`,
			pid)
		if err != nil {
			continue
		}

		for rows.Next() {
			var coID string
			if err := rows.Scan(&coID); err != nil {
				continue
			}
			// Ensure consistent ordering
			a, b := pid, coID
			if a > b {
				a, b = b, a
			}
			_ = s.q.UpsertPaperRelation(ctx, db.UpsertPaperRelationParams{
				PaperIDA:     a,
				PaperIDB:     b,
				RelationType: "co_access",
				Weight:       1.0,
				UpdatedAt:    now,
			})
		}
		rows.Close()
	}

	return nil
}
