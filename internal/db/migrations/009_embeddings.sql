-- +goose Up
-- +goose StatementBegin

-- Embedding vectors for node summaries (API-generated)
CREATE TABLE IF NOT EXISTS node_embeddings (
    paper_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    embedding BLOB NOT NULL,
    model TEXT NOT NULL DEFAULT 'text-embedding-3-small',
    dimensions INTEGER NOT NULL DEFAULT 1536,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    PRIMARY KEY (paper_id, node_id),
    FOREIGN KEY (paper_id) REFERENCES papers(paper_id) ON DELETE CASCADE
);

-- Embedding vectors for paper abstracts
CREATE TABLE IF NOT EXISTS paper_embeddings (
    paper_id TEXT PRIMARY KEY,
    embedding BLOB NOT NULL,
    model TEXT NOT NULL DEFAULT 'text-embedding-3-small',
    dimensions INTEGER NOT NULL DEFAULT 1536,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    FOREIGN KEY (paper_id) REFERENCES papers(paper_id) ON DELETE CASCADE
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS paper_embeddings;
DROP TABLE IF EXISTS node_embeddings;
-- +goose StatementEnd
