-- name: UpsertPaperIndexState :exec
INSERT INTO paper_index_states (
    paper_id,
    index_level,
    raw_parse_status,
    content_available,
    content_bytes,
    flat_page_index_available,
    fts_status,
    semantic_tree_status,
    semantic_tree_available,
    semantic_tree_error,
    semantic_tree_task_id,
    extractor,
    fallback_reason,
    total_pages,
    total_tokens,
    source_file_available,
    file_hash,
    updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(paper_id) DO UPDATE SET
    index_level = excluded.index_level,
    raw_parse_status = excluded.raw_parse_status,
    content_available = excluded.content_available,
    content_bytes = excluded.content_bytes,
    flat_page_index_available = excluded.flat_page_index_available,
    fts_status = excluded.fts_status,
    semantic_tree_status = excluded.semantic_tree_status,
    semantic_tree_available = excluded.semantic_tree_available,
    semantic_tree_error = excluded.semantic_tree_error,
    semantic_tree_task_id = excluded.semantic_tree_task_id,
    extractor = excluded.extractor,
    fallback_reason = excluded.fallback_reason,
    total_pages = excluded.total_pages,
    total_tokens = excluded.total_tokens,
    source_file_available = excluded.source_file_available,
    file_hash = excluded.file_hash,
    updated_at = excluded.updated_at;

-- name: GetPaperIndexState :one
SELECT * FROM paper_index_states
WHERE paper_id = ?;

-- name: ListPaperIndexStates :many
SELECT * FROM paper_index_states
ORDER BY updated_at DESC, paper_id
LIMIT ? OFFSET ?;

-- name: UpdateRawParseState :exec
UPDATE paper_index_states
SET
    raw_parse_status = sqlc.arg(raw_parse_status),
    index_level = sqlc.arg(index_level),
    content_available = sqlc.arg(content_available),
    content_bytes = sqlc.arg(content_bytes),
    extractor = sqlc.arg(extractor),
    fallback_reason = sqlc.arg(fallback_reason),
    total_pages = sqlc.arg(total_pages),
    total_tokens = sqlc.arg(total_tokens),
    updated_at = sqlc.arg(updated_at)
WHERE paper_id = sqlc.arg(paper_id);

-- name: UpdateFTSState :exec
UPDATE paper_index_states
SET
    fts_status = sqlc.arg(fts_status),
    updated_at = sqlc.arg(updated_at)
WHERE paper_id = sqlc.arg(paper_id);

-- name: UpdateSemanticTreeState :exec
UPDATE paper_index_states
SET
    semantic_tree_status = sqlc.arg(semantic_tree_status),
    semantic_tree_available = sqlc.arg(semantic_tree_available),
    semantic_tree_error = sqlc.arg(semantic_tree_error),
    semantic_tree_task_id = sqlc.arg(semantic_tree_task_id),
    updated_at = sqlc.arg(updated_at)
WHERE paper_id = sqlc.arg(paper_id);

-- name: CountPaperIndexStatesByStatus :one
SELECT COUNT(*) FROM paper_index_states
WHERE raw_parse_status = sqlc.arg(raw_parse_status)
  AND fts_status = sqlc.arg(fts_status)
  AND semantic_tree_status = sqlc.arg(semantic_tree_status);
