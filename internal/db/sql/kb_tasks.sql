-- name: InsertKBTask :exec
INSERT INTO kb_tasks (
    task_id,
    paper_id,
    task_type,
    status,
    progress,
    input_json,
    result_json,
    result_ref,
    error,
    lease_owner,
    lease_until,
    attempt_count,
    created_at,
    updated_at,
    started_at,
    finished_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: EnqueueKBTask :one
INSERT INTO kb_tasks (
    task_id,
    paper_id,
    task_type,
    status,
    progress,
    input_json,
    result_json,
    result_ref,
    error,
    lease_owner,
    lease_until,
    attempt_count,
    created_at,
    updated_at,
    started_at,
    finished_at
) VALUES (
    sqlc.arg(task_id),
    sqlc.arg(paper_id),
    sqlc.arg(task_type),
    'queued',
    0,
    sqlc.arg(input_json),
    NULL,
    NULL,
    NULL,
    NULL,
    NULL,
    0,
    sqlc.arg(created_at),
    sqlc.arg(updated_at),
    NULL,
    NULL
)
ON CONFLICT(task_id) DO UPDATE SET
    status = CASE
        WHEN kb_tasks.status IN ('failed', 'canceled') THEN 'queued'
        ELSE kb_tasks.status
    END,
    input_json = CASE
        WHEN kb_tasks.status IN ('failed', 'canceled') THEN excluded.input_json
        ELSE kb_tasks.input_json
    END,
    error = CASE
        WHEN kb_tasks.status IN ('failed', 'canceled') THEN NULL
        ELSE kb_tasks.error
    END,
    progress = CASE
        WHEN kb_tasks.status IN ('failed', 'canceled') THEN 0
        ELSE kb_tasks.progress
    END,
    lease_owner = CASE
        WHEN kb_tasks.status IN ('failed', 'canceled') THEN NULL
        ELSE kb_tasks.lease_owner
    END,
    lease_until = CASE
        WHEN kb_tasks.status IN ('failed', 'canceled') THEN NULL
        ELSE kb_tasks.lease_until
    END,
    updated_at = excluded.updated_at,
    started_at = CASE
        WHEN kb_tasks.status IN ('failed', 'canceled') THEN NULL
        ELSE kb_tasks.started_at
    END,
    finished_at = CASE
        WHEN kb_tasks.status IN ('failed', 'canceled') THEN NULL
        ELSE kb_tasks.finished_at
    END
RETURNING *;

-- name: ForceRequeueKBTask :one
UPDATE kb_tasks
SET
    status = 'queued',
    progress = 0,
    input_json = sqlc.arg(input_json),
    result_json = NULL,
    result_ref = NULL,
    error = NULL,
    lease_owner = NULL,
    lease_until = NULL,
    updated_at = sqlc.arg(updated_at),
    started_at = NULL,
    finished_at = NULL
WHERE task_id = sqlc.arg(task_id)
RETURNING *;

-- name: GetKBTask :one
SELECT * FROM kb_tasks
WHERE task_id = ?;

-- name: ListKBTasksByPaper :many
SELECT * FROM kb_tasks
WHERE paper_id = ?
ORDER BY created_at DESC, task_id DESC
LIMIT ? OFFSET ?;

-- name: ListPendingKBTasks :many
SELECT * FROM kb_tasks
WHERE status = 'queued'
ORDER BY created_at ASC
LIMIT ?;

-- name: ClaimPendingKBTask :one
UPDATE kb_tasks
SET
    status = 'running',
    lease_owner = sqlc.arg(lease_owner),
    lease_until = sqlc.arg(lease_until),
    attempt_count = attempt_count + 1,
    updated_at = sqlc.arg(updated_at),
    started_at = COALESCE(started_at, sqlc.arg(updated_at))
WHERE task_id = (
    SELECT task_id
    FROM kb_tasks AS t
    WHERE t.status = 'queued'
       OR (t.status = 'running' AND t.lease_until IS NOT NULL AND t.lease_until < sqlc.arg(now_epoch))
    ORDER BY t.created_at ASC
    LIMIT 1
)
RETURNING *;

-- name: ClaimPendingKBTaskByType :one
UPDATE kb_tasks
SET
    status = 'running',
    lease_owner = sqlc.arg(lease_owner),
    lease_until = sqlc.arg(lease_until),
    attempt_count = attempt_count + 1,
    updated_at = sqlc.arg(updated_at),
    started_at = COALESCE(started_at, sqlc.arg(updated_at))
WHERE task_id = (
    SELECT task_id
    FROM kb_tasks AS t
    WHERE t.task_type = sqlc.arg(task_type)
      AND (t.status = 'queued'
        OR (t.status = 'running' AND t.lease_until IS NOT NULL AND t.lease_until < sqlc.arg(now_epoch)))
    ORDER BY t.created_at ASC
    LIMIT 1
)
RETURNING *;

-- name: UpdateKBTaskProgress :exec
UPDATE kb_tasks
SET
    progress = sqlc.arg(progress),
    lease_owner = sqlc.arg(lease_owner),
    lease_until = sqlc.arg(lease_until),
    updated_at = sqlc.arg(updated_at)
WHERE task_id = sqlc.arg(task_id)
  AND status = 'running';

-- name: CompleteKBTask :exec
UPDATE kb_tasks
SET
    status = 'succeeded',
    progress = 1.0,
    result_json = sqlc.arg(result_json),
    result_ref = sqlc.arg(result_ref),
    error = NULL,
    lease_owner = NULL,
    lease_until = NULL,
    updated_at = sqlc.arg(updated_at),
    finished_at = sqlc.arg(updated_at)
WHERE task_id = sqlc.arg(task_id);

-- name: RetryKBTask :exec
UPDATE kb_tasks
SET
    status = 'queued',
    progress = 0,
    error = sqlc.arg(error),
    lease_owner = NULL,
    lease_until = NULL,
    updated_at = sqlc.arg(updated_at)
WHERE task_id = sqlc.arg(task_id)
  AND status = 'running';

-- name: FailKBTask :exec
UPDATE kb_tasks
SET
    status = 'failed',
    error = sqlc.arg(error),
    lease_owner = NULL,
    lease_until = NULL,
    updated_at = sqlc.arg(updated_at),
    finished_at = sqlc.arg(updated_at)
WHERE task_id = sqlc.arg(task_id);

-- name: CancelKBTask :exec
UPDATE kb_tasks
SET
    status = 'canceled',
    error = sqlc.arg(error),
    lease_owner = NULL,
    lease_until = NULL,
    updated_at = sqlc.arg(updated_at),
    finished_at = sqlc.arg(updated_at)
WHERE task_id = sqlc.arg(task_id)
  AND status IN ('queued', 'running');
