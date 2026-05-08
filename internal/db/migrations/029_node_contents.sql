-- +goose Up
ALTER TABLE paper_trees ADD COLUMN index_metadata TEXT NOT NULL DEFAULT '{}';

CREATE TABLE IF NOT EXISTS node_contents (
    paper_id    TEXT NOT NULL,
    node_id     TEXT NOT NULL,
    content     TEXT NOT NULL,
    token_count INTEGER,
    source      TEXT,
    created_at  INTEGER DEFAULT (strftime('%s','now')),
    PRIMARY KEY (paper_id, node_id),
    FOREIGN KEY (paper_id, node_id) REFERENCES node_summaries(paper_id, node_id) ON DELETE CASCADE
);

CREATE VIRTUAL TABLE IF NOT EXISTS node_contents_fts USING fts5(
    paper_id UNINDEXED,
    node_id UNINDEXED,
    title,
    content,
    tokenize='unicode61'
);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS node_contents_ai_fts AFTER INSERT ON node_contents BEGIN
    INSERT INTO node_contents_fts(rowid, paper_id, node_id, title, content)
    SELECT
        new.rowid,
        COALESCE(new.paper_id, ''),
        COALESCE(new.node_id, ''),
        COALESCE(ns.title, ''),
        COALESCE(new.content, '')
    FROM node_summaries ns
    WHERE ns.paper_id = new.paper_id AND ns.node_id = new.node_id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS node_contents_ad_fts AFTER DELETE ON node_contents BEGIN
    DELETE FROM node_contents_fts WHERE rowid = old.rowid;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS node_contents_au_fts AFTER UPDATE OF paper_id, node_id, content ON node_contents BEGIN
    DELETE FROM node_contents_fts WHERE rowid = old.rowid;
    INSERT INTO node_contents_fts(rowid, paper_id, node_id, title, content)
    SELECT
        new.rowid,
        COALESCE(new.paper_id, ''),
        COALESCE(new.node_id, ''),
        COALESCE(ns.title, ''),
        COALESCE(new.content, '')
    FROM node_summaries ns
    WHERE ns.paper_id = new.paper_id AND ns.node_id = new.node_id;
END;
-- +goose StatementEnd

INSERT INTO node_contents_fts(rowid, paper_id, node_id, title, content)
SELECT
    nc.rowid,
    COALESCE(nc.paper_id, ''),
    COALESCE(nc.node_id, ''),
    COALESCE(ns.title, ''),
    COALESCE(nc.content, '')
FROM node_contents nc
JOIN node_summaries ns
  ON ns.paper_id = nc.paper_id AND ns.node_id = nc.node_id;

-- +goose Down
DROP TRIGGER IF EXISTS node_contents_au_fts;
DROP TRIGGER IF EXISTS node_contents_ad_fts;
DROP TRIGGER IF EXISTS node_contents_ai_fts;
DROP TABLE IF EXISTS node_contents_fts;
DROP TABLE IF EXISTS node_contents;
-- SQLite does not support a stable DROP COLUMN rollback; paper_trees.index_metadata remains.
