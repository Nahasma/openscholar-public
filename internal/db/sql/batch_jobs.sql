-- name: InsertBatchJob :exec
INSERT INTO batch_jobs (id, file_path, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?);

-- name: UpdateBatchJobStatus :exec
UPDATE batch_jobs SET status = ?, error = ?, paper_id = ?, updated_at = ? WHERE id = ?;

-- name: GetBatchJob :one
SELECT * FROM batch_jobs WHERE id = ?;

-- name: ListPendingBatchJobs :many
SELECT * FROM batch_jobs WHERE status = 'pending' ORDER BY created_at LIMIT ?;

-- name: ListFailedBatchJobs :many
SELECT * FROM batch_jobs WHERE status = 'failed' ORDER BY created_at LIMIT ?;

-- name: CountBatchJobsByStatus :many
SELECT status, COUNT(*) as count FROM batch_jobs GROUP BY status;

-- name: ResetFailedBatchJobs :exec
UPDATE batch_jobs SET status = 'pending', error = NULL, updated_at = ? WHERE status = 'failed';

-- name: IncrementBatchJobAttempt :exec
UPDATE batch_jobs SET attempt_count = attempt_count + 1, updated_at = unixepoch() WHERE id = ?;

-- name: ResetBatchJobForRetry :exec
UPDATE batch_jobs SET status = 'pending', updated_at = unixepoch() WHERE id = ? AND attempt_count < max_attempts;
