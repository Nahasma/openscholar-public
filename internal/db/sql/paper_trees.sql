-- name: InsertPaperTree :exec
INSERT INTO paper_trees (
    paper_id, tree_json, doc_description, total_pages, total_tokens, model_used, version, created_at, index_metadata
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetPaperTree :one
SELECT * FROM paper_trees WHERE paper_id = ?;

-- name: UpdatePaperTreeVersion :exec
UPDATE paper_trees
SET tree_json = ?, doc_description = ?, version = version + 1
WHERE paper_id = ?;

-- name: DeletePaperTree :exec
DELETE FROM paper_trees WHERE paper_id = ?;
