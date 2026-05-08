-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS memory_items (
    id TEXT PRIMARY KEY,
    session_id TEXT,                                    -- NULL = 全局记忆
    content TEXT NOT NULL,
    metadata TEXT NOT NULL DEFAULT '{}',                -- JSON 元数据
    access_count INTEGER NOT NULL DEFAULT 0,
    operation_history TEXT NOT NULL DEFAULT '[]',       -- 操作历史 JSON
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_memory_session ON memory_items(session_id);
CREATE INDEX IF NOT EXISTS idx_memory_updated ON memory_items(updated_at DESC);

CREATE TABLE IF NOT EXISTS memory_skill_stats (
    skill_name TEXT PRIMARY KEY,
    usage_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);

CREATE TRIGGER IF NOT EXISTS update_memory_items_updated_at
AFTER UPDATE ON memory_items
BEGIN
    UPDATE memory_items SET updated_at = strftime('%s', 'now')
    WHERE id = new.id;
END;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS update_memory_items_updated_at;
DROP INDEX IF EXISTS idx_memory_updated;
DROP INDEX IF EXISTS idx_memory_session;
DROP TABLE IF EXISTS memory_skill_stats;
DROP TABLE IF EXISTS memory_items;
-- +goose StatementEnd
