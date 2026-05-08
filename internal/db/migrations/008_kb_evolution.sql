-- +goose Up
-- +goose StatementBegin

-- Usage statistics for KB search tracking
CREATE TABLE IF NOT EXISTS kb_usage_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    paper_id TEXT REFERENCES papers(paper_id) ON DELETE CASCADE,
    node_id TEXT,
    query TEXT NOT NULL,
    search_type TEXT NOT NULL DEFAULT 'tree',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE INDEX IF NOT EXISTS idx_usage_paper ON kb_usage_stats(paper_id);
CREATE INDEX IF NOT EXISTS idx_usage_created ON kb_usage_stats(created_at);

-- Paper-to-paper co-access relations
CREATE TABLE IF NOT EXISTS paper_relations (
    paper_id_a TEXT NOT NULL REFERENCES papers(paper_id) ON DELETE CASCADE,
    paper_id_b TEXT NOT NULL REFERENCES papers(paper_id) ON DELETE CASCADE,
    relation_type TEXT NOT NULL DEFAULT 'co_access',
    weight REAL NOT NULL DEFAULT 1.0,
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    PRIMARY KEY (paper_id_a, paper_id_b, relation_type)
);

-- Research topic clusters
CREATE TABLE IF NOT EXISTS research_topics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    paper_ids TEXT NOT NULL DEFAULT '[]',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS research_topics;
DROP TABLE IF EXISTS paper_relations;
DROP INDEX IF EXISTS idx_usage_created;
DROP INDEX IF EXISTS idx_usage_paper;
DROP TABLE IF EXISTS kb_usage_stats;
-- +goose StatementEnd
