-- name: CreateSession :one
INSERT INTO sessions (
    id,
    parent_session_id,
    title,
    message_count,
    prompt_tokens,
    completion_tokens,
    cost,
    summary,
    tags,
    first_prompt,
    git_branch,
    project_path,
    worktree_path,
    mode,
    root_session_id,
    forked_from_session_id,
    summary_message_id,
    updated_at,
    created_at
) VALUES (
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    null,
    strftime('%s', 'now'),
    strftime('%s', 'now')
) RETURNING *;

-- name: GetSessionByID :one
SELECT *
FROM sessions
WHERE id = ? LIMIT 1;

-- name: ListSessions :many
SELECT *
FROM sessions
WHERE parent_session_id is NULL
ORDER BY created_at DESC;

-- name: ListSessionsForResume :many
SELECT *
FROM sessions
WHERE parent_session_id IS NULL
  AND message_count > 0
  AND (sqlc.arg(project_path) = '' OR project_path = '' OR project_path = sqlc.arg(project_path))
ORDER BY updated_at DESC
LIMIT sqlc.arg(limit);

-- name: ResolveSessionByExactTitle :many
SELECT *
FROM sessions
WHERE parent_session_id IS NULL
  AND title = sqlc.arg(title)
  AND (sqlc.arg(project_path) = '' OR project_path = '' OR project_path = sqlc.arg(project_path))
ORDER BY updated_at DESC
LIMIT sqlc.arg(limit);

-- name: SearchSessionsByMetadata :many
SELECT *
FROM sessions
WHERE parent_session_id IS NULL
  AND (sqlc.arg(project_path) = '' OR project_path = '' OR project_path = sqlc.arg(project_path))
  AND (
      instr(lower(title), lower(sqlc.arg(query))) > 0
      OR instr(lower(summary), lower(sqlc.arg(query))) > 0
      OR instr(lower(first_prompt), lower(sqlc.arg(query))) > 0
      OR instr(lower(tags), lower(sqlc.arg(query))) > 0
      OR instr(lower(git_branch), lower(sqlc.arg(query))) > 0
  )
ORDER BY updated_at DESC
LIMIT sqlc.arg(limit);

-- name: UpdateSession :one
UPDATE sessions
SET
    title = ?,
    prompt_tokens = ?,
    completion_tokens = ?,
    summary_message_id = ?,
    cost = ?,
    summary = ?,
    tags = ?,
    first_prompt = ?,
    git_branch = ?,
    project_path = ?,
    worktree_path = ?,
    mode = ?,
    root_session_id = ?,
    forked_from_session_id = ?
WHERE id = ?
RETURNING *;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE id = ?;
