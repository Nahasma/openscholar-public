package kb

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
)

// VectorSearcher performs cosine similarity search over stored embeddings.
type VectorSearcher struct {
	db       *sql.DB
	embedder *EmbeddingProvider
}

// NewVectorSearcher creates a vector searcher.
func NewVectorSearcher(db *sql.DB, embedder *EmbeddingProvider) *VectorSearcher {
	return &VectorSearcher{db: db, embedder: embedder}
}

// SearchNodesWithFallback performs vector search, falling back to FTS5 on failure.
func (v *VectorSearcher) SearchNodesWithFallback(ctx context.Context, fts *FTSSearcher, query string, limit int) ([]RankedItem, error) {
	results, err := v.SearchNodes(ctx, query, limit)
	if err != nil && fts != nil {
		log.Printf("vector search failed, falling back to FTS5: %v", err)
		ftsResults, ftsErr := fts.SearchNodes(ctx, query, limit)
		if ftsErr != nil {
			return nil, fmt.Errorf("both vector and FTS5 search failed: vector=%v, fts5=%v", err, ftsErr)
		}
		return ftsToRanked(ftsResults), nil
	}
	return results, err
}

// SearchNodes embeds the query and finds the most similar node embeddings.
func (v *VectorSearcher) SearchNodes(ctx context.Context, query string, limit int) ([]RankedItem, error) {
	if limit <= 0 {
		limit = 20
	}

	// 1. Embed the query
	queryVec, err := v.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// 2. Load all node embeddings (O(N) scan — fine for <100K nodes)
	rows, err := v.db.QueryContext(ctx,
		`SELECT ne.paper_id, ne.node_id, ne.embedding, ns.title
		 FROM node_embeddings ne
		 LEFT JOIN node_summaries ns ON ne.paper_id = ns.paper_id AND ne.node_id = ns.node_id`)
	if err != nil {
		return nil, fmt.Errorf("load embeddings: %w", err)
	}
	defer rows.Close()

	type scored struct {
		paperID string
		nodeID  string
		title   string
		score   float64
	}
	var candidates []scored

	for rows.Next() {
		var paperID, nodeID string
		var embBlob []byte
		var title sql.NullString
		if err := rows.Scan(&paperID, &nodeID, &embBlob, &title); err != nil {
			continue
		}

		nodeVec := DecodeEmbedding(embBlob)
		sim := CosineSimilarity(queryVec, nodeVec)
		candidates = append(candidates, scored{
			paperID: paperID,
			nodeID:  nodeID,
			title:   title.String,
			score:   sim,
		})
	}

	// 3. Sort by similarity (descending)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// 4. Return top-N
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	results := make([]RankedItem, len(candidates))
	for i, c := range candidates {
		results[i] = RankedItem{
			PaperID: c.paperID,
			NodeID:  c.nodeID,
			Title:   c.title,
			Score:   c.score,
		}
	}
	return results, nil
}

// StoreNodeEmbedding saves an embedding for a node summary.
func (v *VectorSearcher) StoreNodeEmbedding(ctx context.Context, paperID, nodeID string, embedding []float32) error {
	_, err := v.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO node_embeddings (paper_id, node_id, embedding, model, dimensions)
		 VALUES (?, ?, ?, 'text-embedding-3-small', ?)`,
		paperID, nodeID, EncodeEmbedding(embedding), len(embedding))
	return err
}

// GenerateAndStoreEmbeddings generates embeddings for all node summaries of a paper.
func (v *VectorSearcher) GenerateAndStoreEmbeddings(ctx context.Context, paperID string, nodes []NodeSummary) error {
	if len(nodes) == 0 {
		return nil
	}

	// Batch embed all summaries
	texts := make([]string, len(nodes))
	for i, n := range nodes {
		texts[i] = n.Title + ": " + n.Summary
	}

	embeddings, err := v.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("batch embed: %w", err)
	}

	// Store each embedding
	for i, emb := range embeddings {
		if err := v.StoreNodeEmbedding(ctx, paperID, nodes[i].NodeID, emb); err != nil {
			return fmt.Errorf("store embedding for node %s: %w", nodes[i].NodeID, err)
		}
	}
	return nil
}
