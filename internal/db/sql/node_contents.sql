-- name: InsertNodeContent :exec
INSERT INTO node_contents (
    paper_id, node_id, content, token_count, source
) VALUES (?, ?, ?, ?, ?);

-- name: GetNodeContent :one
SELECT * FROM node_contents WHERE paper_id = ? AND node_id = ?;

-- name: GetNodeContents :many
SELECT * FROM node_contents WHERE paper_id = ? ORDER BY node_id;

-- name: DeleteNodeContents :exec
DELETE FROM node_contents WHERE paper_id = ?;

-- name: CountNodeContents :one
SELECT COUNT(*) FROM node_contents WHERE paper_id = ?;

-- name: SearchNodeContents :many
SELECT
    nc.paper_id,
    nc.node_id,
    ns.title,
    nc.content,
    nc.token_count,
    nc.source,
    nc.created_at
FROM node_contents_fts f
JOIN node_contents nc ON nc.rowid = f.rowid
JOIN node_summaries ns
  ON ns.paper_id = nc.paper_id
 AND ns.node_id = nc.node_id
WHERE f.paper_id = sqlc.arg(paper_id)
  AND f.content MATCH sqlc.arg(query)
ORDER BY bm25(node_contents_fts), nc.node_id
LIMIT sqlc.arg(limit);
