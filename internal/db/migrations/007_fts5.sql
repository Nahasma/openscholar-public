-- +goose Up
-- +goose StatementBegin

-- FTS5 full-text index for papers (title, abstract, authors)
CREATE VIRTUAL TABLE IF NOT EXISTS papers_fts USING fts5(
    paper_id UNINDEXED,
    title,
    abstract,
    authors
);

-- FTS5 full-text index for node summaries (title, summary)
CREATE VIRTUAL TABLE IF NOT EXISTS node_summaries_fts USING fts5(
    paper_id UNINDEXED,
    node_id UNINDEXED,
    title,
    summary
);

-- Sync triggers: papers → papers_fts
CREATE TRIGGER IF NOT EXISTS papers_fts_ai AFTER INSERT ON papers BEGIN
    INSERT INTO papers_fts(paper_id, title, abstract, authors)
    VALUES (new.paper_id, new.title, COALESCE(new.abstract, ''), COALESCE(new.authors, ''));
END;

CREATE TRIGGER IF NOT EXISTS papers_fts_ad AFTER DELETE ON papers BEGIN
    INSERT INTO papers_fts(papers_fts, paper_id, title, abstract, authors)
    VALUES ('delete', old.paper_id, old.title, COALESCE(old.abstract, ''), COALESCE(old.authors, ''));
END;

-- Sync triggers: node_summaries → node_summaries_fts
CREATE TRIGGER IF NOT EXISTS nodes_fts_ai AFTER INSERT ON node_summaries BEGIN
    INSERT INTO node_summaries_fts(paper_id, node_id, title, summary)
    VALUES (new.paper_id, COALESCE(new.node_id, ''), COALESCE(new.title, ''), new.summary);
END;

CREATE TRIGGER IF NOT EXISTS nodes_fts_ad AFTER DELETE ON node_summaries BEGIN
    INSERT INTO node_summaries_fts(node_summaries_fts, paper_id, node_id, title, summary)
    VALUES ('delete', old.paper_id, COALESCE(old.node_id, ''), COALESCE(old.title, ''), old.summary);
END;

-- Backfill existing data
INSERT INTO papers_fts(paper_id, title, abstract, authors)
SELECT paper_id, title, COALESCE(abstract, ''), COALESCE(authors, '') FROM papers;

INSERT INTO node_summaries_fts(paper_id, node_id, title, summary)
SELECT paper_id, COALESCE(node_id, ''), COALESCE(title, ''), summary FROM node_summaries;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS nodes_fts_ad;
DROP TRIGGER IF EXISTS nodes_fts_ai;
DROP TRIGGER IF EXISTS papers_fts_ad;
DROP TRIGGER IF EXISTS papers_fts_ai;
DROP TABLE IF EXISTS node_summaries_fts;
DROP TABLE IF EXISTS papers_fts;
-- +goose StatementEnd
