-- +goose Up

ALTER TABLE skills_index ADD COLUMN resolved_source_id TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS skill_sources (
    id TEXT PRIMARY KEY,
    skill_id TEXT NOT NULL,
    source_tier TEXT NOT NULL,
    source_kind TEXT NOT NULL,
    source_key TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    file_path TEXT NOT NULL DEFAULT '',
    meta_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_skill_sources_skill_id ON skill_sources(skill_id);

-- +goose Down

DROP INDEX IF EXISTS idx_skill_sources_skill_id;
DROP TABLE IF EXISTS skill_sources;

-- SQLite cannot drop columns directly; keep skills_index backward-compatible.
