-- +goose Up

CREATE TABLE IF NOT EXISTS pipelines (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id),
    topic       TEXT NOT NULL,
    template    TEXT NOT NULL DEFAULT 'empirical',
    mode        TEXT NOT NULL DEFAULT 'default',
    status      TEXT NOT NULL DEFAULT 'planning',
    budget_limit REAL DEFAULT 0,
    budget_spent REAL DEFAULT 0,
    work_dir    TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS pipeline_phases (
    id           TEXT PRIMARY KEY,
    pipeline_id  TEXT NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    phase_order  INTEGER NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',
    checkpoint   INTEGER NOT NULL DEFAULT 0,
    max_workers  INTEGER NOT NULL DEFAULT 2,
    created_at   INTEGER NOT NULL,
    completed_at INTEGER
);

CREATE INDEX idx_phases_pipeline ON pipeline_phases(pipeline_id);

-- +goose Down
DROP TABLE IF EXISTS pipeline_phases;
DROP TABLE IF EXISTS pipelines;
