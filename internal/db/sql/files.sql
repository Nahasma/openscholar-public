-- name: CreateFile :one
INSERT INTO files (
    id,
    session_id,
    path,
    content,
    version,
    created_at,
    updated_at
) VALUES (
    ?, ?, ?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now')
)
RETURNING *;

-- name: GetFile :one
SELECT *
FROM files
WHERE id = ? LIMIT 1;

-- name: ListFilesBySession :many
SELECT *
FROM files
WHERE session_id = ?
ORDER BY created_at ASC;

-- name: GetFileByPath :one
SELECT *
FROM files
WHERE path = ? AND session_id = ?
ORDER BY created_at DESC
LIMIT 1;

-- name: DeleteFile :exec
DELETE FROM files
WHERE id = ?;
