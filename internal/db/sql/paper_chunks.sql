-- name: InsertPaperChunk :exec
INSERT INTO paper_chunks (
    paper_id,
    chunk_id,
    kind,
    page_start,
    page_end,
    title,
    content,
    token_count,
    source,
    created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: BatchInsertPaperChunks :exec
INSERT INTO paper_chunks (
    paper_id,
    chunk_id,
    kind,
    page_start,
    page_end,
    title,
    content,
    token_count,
    source,
    created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetPaperChunks :many
SELECT * FROM paper_chunks
WHERE paper_id = ?
ORDER BY COALESCE(page_start, 2147483647), chunk_id;

-- name: GetPaperChunk :one
SELECT * FROM paper_chunks
WHERE paper_id = ? AND chunk_id = ?;

-- name: CountPaperChunks :one
SELECT COUNT(*) FROM paper_chunks
WHERE paper_id = ?;

-- name: SumPaperChunkBytes :one
SELECT CAST(COALESCE(SUM(LENGTH(COALESCE(content, ''))), 0) AS INTEGER) FROM paper_chunks
WHERE paper_id = ?;

-- name: SearchPaperChunks :many
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
WHERE paper_chunks_fts.paper_id = sqlc.arg(paper_id)
  AND paper_chunks_fts.content MATCH sqlc.arg(query)
ORDER BY bm25(paper_chunks_fts), COALESCE(pc.page_start, 2147483647), pc.chunk_id
LIMIT sqlc.arg(limit);

-- name: SearchAllPaperChunks :many
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
WHERE paper_chunks_fts.content MATCH sqlc.arg(query)
ORDER BY bm25(paper_chunks_fts), pc.paper_id, COALESCE(pc.page_start, 2147483647), pc.chunk_id
LIMIT sqlc.arg(limit);

-- name: DeletePaperChunks :exec
DELETE FROM paper_chunks WHERE paper_id = ?;

-- name: ClearPaperChunksFTS :exec
DELETE FROM paper_chunks_fts;

-- name: RebuildPaperChunksFTS :exec
INSERT OR REPLACE INTO paper_chunks_fts(rowid, paper_id, chunk_id, kind, page_start, page_end, title, content)
SELECT
    pc.rowid,
    COALESCE(pc.paper_id, ''),
    COALESCE(pc.chunk_id, ''),
    COALESCE(pc.kind, ''),
    COALESCE(pc.page_start, 0),
    COALESCE(pc.page_end, 0),
    COALESCE(pc.title, ''),
    COALESCE(pc.content, '')
FROM paper_chunks pc;

-- name: DeletePaperChunksFTSForPaper :exec
DELETE FROM paper_chunks_fts
WHERE paper_id = sqlc.arg(target_paper_id);

-- name: RebuildPaperChunksFTSForPaper :exec
INSERT OR REPLACE INTO paper_chunks_fts(rowid, paper_id, chunk_id, kind, page_start, page_end, title, content)
SELECT
    pc.rowid,
    COALESCE(pc.paper_id, ''),
    COALESCE(pc.chunk_id, ''),
    COALESCE(pc.kind, ''),
    COALESCE(pc.page_start, 0),
    COALESCE(pc.page_end, 0),
    COALESCE(pc.title, ''),
    COALESCE(pc.content, '')
FROM paper_chunks pc
WHERE pc.paper_id = sqlc.arg(target_paper_id);
