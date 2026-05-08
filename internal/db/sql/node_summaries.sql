-- name: InsertNodeSummary :exec
INSERT INTO node_summaries (
    paper_id, node_id, title, start_page, end_page, summary
) VALUES (?, ?, ?, ?, ?, ?);

-- name: GetNodeSummaries :many
SELECT * FROM node_summaries WHERE paper_id = ? ORDER BY node_id;

-- name: GetNodeSummary :one
SELECT * FROM node_summaries WHERE paper_id = ? AND node_id = ?;

-- name: DeleteNodeSummaries :exec
DELETE FROM node_summaries WHERE paper_id = ?;

-- name: SearchNodeSummaries :many
SELECT * FROM node_summaries
WHERE summary LIKE '%' || ? || '%'
ORDER BY paper_id, node_id
LIMIT ?;
