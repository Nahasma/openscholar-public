-- +goose Up
CREATE TABLE IF NOT EXISTS experiment_nodes (
    id           TEXT PRIMARY KEY,
    parent_id    TEXT,
    pipeline_id  TEXT NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    node_type    TEXT NOT NULL DEFAULT 'preliminary',
    status       TEXT NOT NULL DEFAULT 'pending',
    description  TEXT,
    code_path    TEXT,
    metrics      TEXT,
    error_log    TEXT,
    score        REAL DEFAULT 0,
    depth        INTEGER DEFAULT 0,
    created_at   INTEGER NOT NULL
);

CREATE INDEX idx_nodes_pipeline ON experiment_nodes(pipeline_id);
CREATE INDEX idx_nodes_parent ON experiment_nodes(parent_id);

-- +goose Down
DROP INDEX IF EXISTS idx_nodes_parent;
DROP INDEX IF EXISTS idx_nodes_pipeline;
DROP TABLE IF EXISTS experiment_nodes;
