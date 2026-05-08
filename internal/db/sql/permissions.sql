-- name: CreatePermissionRule :one
INSERT INTO permission_rules (id, tool_name, action, path, decision, scope)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPermissionRule :one
SELECT * FROM permission_rules
WHERE tool_name = ? AND action = ? AND path = ? AND scope = 'always'
LIMIT 1;

-- name: ListPermissionRules :many
SELECT * FROM permission_rules
WHERE scope = 'always'
ORDER BY created_at DESC;

-- name: DeletePermissionRule :exec
DELETE FROM permission_rules WHERE id = ?;

-- name: DeleteSessionPermissions :exec
DELETE FROM permission_rules WHERE scope = 'session';
