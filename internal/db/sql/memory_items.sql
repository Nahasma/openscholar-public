-- name: InsertMemoryItem :exec
INSERT INTO memory_items (
    id, session_id, content, metadata, access_count, operation_history, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateMemoryItem :exec
UPDATE memory_items SET content = ?, metadata = ? WHERE id = ?;

-- name: DeleteMemoryItem :exec
DELETE FROM memory_items WHERE id = ?;

-- name: GetMemoryItem :one
SELECT * FROM memory_items WHERE id = ?;

-- name: ListMemoryItemsForSession :many
SELECT * FROM memory_items
WHERE session_id = ? OR session_id IS NULL
ORDER BY updated_at DESC
LIMIT ?;

-- name: ListRecentMemoryItems :many
SELECT * FROM memory_items
ORDER BY updated_at DESC
LIMIT ?;

-- name: ListSessionMemoryItems :many
SELECT * FROM memory_items
WHERE session_id = ?
ORDER BY updated_at DESC
LIMIT ?;

-- name: ListGlobalMemoryItems :many
SELECT * FROM memory_items
WHERE session_id IS NULL
ORDER BY updated_at DESC
LIMIT ?;

-- name: IncrementMemoryAccessCount :exec
UPDATE memory_items SET access_count = access_count + 1 WHERE id = ?;
