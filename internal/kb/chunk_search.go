package kb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/openscholar/openscholar/internal/db"
)

func (s *service) searchPaperChunksTitleContent(ctx context.Context, paperID, query string, limit int64) ([]db.PaperChunk, error) {
	if s.db == nil {
		return s.q.SearchPaperChunks(ctx, db.SearchPaperChunksParams{PaperID: paperID, Query: query, Limit: limit})
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
    pc.paper_id,
    pc.chunk_id,
    pc.kind,
    pc.page_start,
    pc.page_end,
    pc.title,
    pc.content,
    pc.token_count,
    pc.source,
    pc.created_at
FROM paper_chunks_fts
JOIN paper_chunks pc ON pc.rowid = paper_chunks_fts.rowid
WHERE paper_chunks_fts.paper_id = ?
  AND paper_chunks_fts MATCH ?
ORDER BY bm25(paper_chunks_fts), COALESCE(pc.page_start, 2147483647), pc.chunk_id
LIMIT ?`, paperID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("paper chunks title/content FTS: %w", err)
	}
	defer rows.Close()
	return scanPaperChunks(rows)
}

func (s *service) searchAllPaperChunksTitleContent(ctx context.Context, query string, limit int64) ([]db.PaperChunk, error) {
	if s.db == nil {
		return s.q.SearchAllPaperChunks(ctx, db.SearchAllPaperChunksParams{Query: query, Limit: limit})
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
    pc.paper_id,
    pc.chunk_id,
    pc.kind,
    pc.page_start,
    pc.page_end,
    pc.title,
    pc.content,
    pc.token_count,
    pc.source,
    pc.created_at
FROM paper_chunks_fts
JOIN paper_chunks pc ON pc.rowid = paper_chunks_fts.rowid
WHERE paper_chunks_fts MATCH ?
ORDER BY bm25(paper_chunks_fts), pc.paper_id, COALESCE(pc.page_start, 2147483647), pc.chunk_id
LIMIT ?`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("all paper chunks title/content FTS: %w", err)
	}
	defer rows.Close()
	return scanPaperChunks(rows)
}

func scanPaperChunks(rows *sql.Rows) ([]db.PaperChunk, error) {
	chunks := make([]db.PaperChunk, 0)
	for rows.Next() {
		var ch db.PaperChunk
		if err := rows.Scan(
			&ch.PaperID,
			&ch.ChunkID,
			&ch.Kind,
			&ch.PageStart,
			&ch.PageEnd,
			&ch.Title,
			&ch.Content,
			&ch.TokenCount,
			&ch.Source,
			&ch.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan paper chunk: %w", err)
		}
		chunks = append(chunks, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return chunks, nil
}
