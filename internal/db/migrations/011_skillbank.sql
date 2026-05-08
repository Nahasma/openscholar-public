-- +goose Up

CREATE TABLE IF NOT EXISTS skills_index (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    category TEXT NOT NULL,
    tags TEXT NOT NULL DEFAULT '[]',
    version INTEGER NOT NULL DEFAULT 1,
    author TEXT NOT NULL DEFAULT 'system',
    usage_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    file_path TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_skills_category ON skills_index(category);
CREATE INDEX IF NOT EXISTS idx_skills_usage ON skills_index(usage_count DESC);

CREATE VIRTUAL TABLE IF NOT EXISTS skills_fts USING fts5(
    id UNINDEXED,
    name,
    description,
    tags,
    content
);

-- Sync triggers: skills_index → skills_fts
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS skills_fts_insert AFTER INSERT ON skills_index BEGIN
    INSERT INTO skills_fts(id, name, description, tags, content)
    VALUES (new.id, new.name, new.description, new.tags, '');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS skills_fts_update AFTER UPDATE ON skills_index BEGIN
    DELETE FROM skills_fts WHERE id = old.id;
    INSERT INTO skills_fts(id, name, description, tags, content)
    VALUES (new.id, new.name, new.description, new.tags, '');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS skills_fts_delete AFTER DELETE ON skills_index BEGIN
    DELETE FROM skills_fts WHERE id = old.id;
END;
-- +goose StatementEnd

-- +goose Down

DROP TRIGGER IF EXISTS skills_fts_delete;
DROP TRIGGER IF EXISTS skills_fts_update;
DROP TRIGGER IF EXISTS skills_fts_insert;
DROP TABLE IF EXISTS skills_fts;
DROP INDEX IF EXISTS idx_skills_usage;
DROP INDEX IF EXISTS idx_skills_category;
DROP TABLE IF EXISTS skills_index;
