-- +goose Up
ALTER TABLE kb_tasks ADD COLUMN started_at INTEGER;
ALTER TABLE kb_tasks ADD COLUMN finished_at INTEGER;

UPDATE kb_tasks SET status = 'queued' WHERE status = 'pending';
UPDATE kb_tasks SET status = 'succeeded' WHERE status = 'completed';

-- +goose Down
UPDATE kb_tasks SET status = 'pending' WHERE status = 'queued';
UPDATE kb_tasks SET status = 'completed' WHERE status = 'succeeded';
