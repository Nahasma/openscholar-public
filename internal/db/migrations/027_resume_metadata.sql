-- +goose Up
ALTER TABLE sessions ADD COLUMN project_path TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN worktree_path TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN mode TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN root_session_id TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN forked_from_session_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_sessions_project_updated ON sessions(project_path, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_root_updated ON sessions(root_session_id, updated_at DESC);

-- +goose Down
-- SQLite 不支持稳定的 DROP COLUMN 回滚；此迁移为前向兼容。
