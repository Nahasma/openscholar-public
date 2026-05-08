-- name: GetMessage :one
SELECT
    id,
    session_id,
    role,
    parts,
    search_text,
    model,
    created_at,
    updated_at,
    finished_at,
    input_tokens,
    output_tokens,
    cache_creation_input_tokens,
    cache_read_input_tokens,
    meta
FROM messages
WHERE id = ? LIMIT 1;

-- name: ListMessagesBySession :many
SELECT
    id,
    session_id,
    role,
    parts,
    search_text,
    model,
    created_at,
    updated_at,
    finished_at,
    input_tokens,
    output_tokens,
    cache_creation_input_tokens,
    cache_read_input_tokens,
    meta
FROM messages
WHERE session_id = ?
ORDER BY created_at ASC;

-- name: ListMessagesMissingSearchText :many
SELECT
    id,
    session_id,
    role,
    parts,
    search_text,
    model,
    created_at,
    updated_at,
    finished_at,
    input_tokens,
    output_tokens,
    cache_creation_input_tokens,
    cache_read_input_tokens,
    meta
FROM messages
WHERE search_text = ''
ORDER BY created_at ASC, id ASC
LIMIT ?;

-- name: ListMessagesMissingSearchTextForResumeSearch :many
SELECT
    m.id,
    m.session_id,
    m.role,
    m.parts,
    m.search_text,
    m.model,
    m.created_at,
    m.updated_at,
    m.finished_at,
    m.input_tokens,
    m.output_tokens,
    m.cache_creation_input_tokens,
    m.cache_read_input_tokens,
    m.meta
FROM messages m
JOIN sessions hit ON hit.id = m.session_id
LEFT JOIN sessions root ON root.id = COALESCE(NULLIF(hit.root_session_id, ''), hit.parent_session_id, hit.id)
WHERE m.search_text = ''
  AND (sqlc.arg(token_1) = '' OR instr(lower(m.parts), lower(sqlc.arg(token_1))) > 0)
  AND (sqlc.arg(token_2) = '' OR instr(lower(m.parts), lower(sqlc.arg(token_2))) > 0)
  AND (sqlc.arg(token_3) = '' OR instr(lower(m.parts), lower(sqlc.arg(token_3))) > 0)
  AND (sqlc.arg(token_4) = '' OR instr(lower(m.parts), lower(sqlc.arg(token_4))) > 0)
  AND (sqlc.arg(token_5) = '' OR instr(lower(m.parts), lower(sqlc.arg(token_5))) > 0)
  AND (sqlc.arg(project_path) = '' OR root.project_path = '' OR root.project_path = sqlc.arg(project_path))
ORDER BY m.updated_at DESC, m.id ASC
LIMIT sqlc.arg(limit);

-- name: CreateMessage :one
INSERT INTO messages (
    id,
    session_id,
    role,
    parts,
    search_text,
    model,
    input_tokens,
    output_tokens,
    cache_creation_input_tokens,
    cache_read_input_tokens,
    meta,
    created_at,
    updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now')
)
RETURNING *;

-- name: UpdateMessage :exec
UPDATE messages
SET
    parts = ?,
    search_text = ?,
    finished_at = ?,
    input_tokens = ?,
    output_tokens = ?,
    cache_creation_input_tokens = ?,
    cache_read_input_tokens = ?,
    meta = ?,
    updated_at = strftime('%s', 'now')
WHERE id = ?;

-- name: BackfillMessageSearchText :exec
UPDATE messages
SET search_text = ?
WHERE id = ?;

-- name: SearchRootSessionsByMessageFTS :many
WITH RECURSIVE ancestors(id, parent_session_id, updated_at, project_path, depth) AS (
    SELECT
        hit.id,
        hit.parent_session_id,
        hit.updated_at,
        hit.project_path,
        0
    FROM messages_fts mf
    JOIN messages m ON m.id = mf.message_id
    JOIN sessions hit ON hit.id = m.session_id
    WHERE mf.search_text MATCH sqlc.arg(query)

    UNION ALL

    SELECT
        parent.id,
        parent.parent_session_id,
        parent.updated_at,
        parent.project_path,
        ancestors.depth + 1
    FROM ancestors
    JOIN sessions parent ON parent.id = ancestors.parent_session_id
    WHERE ancestors.parent_session_id IS NOT NULL
      AND ancestors.parent_session_id != ''
      AND ancestors.depth < 32
)
SELECT id
FROM ancestors
WHERE (parent_session_id IS NULL OR parent_session_id = '')
  AND (sqlc.arg(project_path) = '' OR project_path = '' OR project_path = sqlc.arg(project_path))
GROUP BY id
ORDER BY MAX(updated_at) DESC
LIMIT sqlc.arg(limit);

-- name: DeleteMessage :exec
DELETE FROM messages
WHERE id = ?;

-- name: DeleteSessionMessages :exec
DELETE FROM messages
WHERE session_id = ?;
