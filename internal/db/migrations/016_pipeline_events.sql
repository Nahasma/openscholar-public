-- +goose Up
CREATE TABLE IF NOT EXISTS pipeline_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id TEXT NOT NULL,
    event_type  TEXT NOT NULL,
    payload     TEXT DEFAULT '{}',
    created_at  INTEGER NOT NULL
);
CREATE INDEX idx_pipeline_events_pipeline ON pipeline_events(pipeline_id, created_at);
CREATE INDEX idx_pipeline_events_type ON pipeline_events(event_type, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_pipeline_events_type;
DROP INDEX IF EXISTS idx_pipeline_events_pipeline;
DROP TABLE IF EXISTS pipeline_events;
