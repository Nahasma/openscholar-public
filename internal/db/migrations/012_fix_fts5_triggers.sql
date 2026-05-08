-- +goose Up
-- +goose StatementBegin

-- Fix: FTS5 delete triggers used the content-synced 'delete' command
-- (INSERT INTO fts(fts,...) VALUES ('delete',...)) on standalone FTS5 tables.
-- Standalone tables require DELETE FROM fts WHERE rowid = ... instead.
-- Also standardize COALESCE handling across all insert triggers.

-- Drop old triggers
DROP TRIGGER IF EXISTS papers_fts_ai;
DROP TRIGGER IF EXISTS papers_fts_ad;
DROP TRIGGER IF EXISTS nodes_fts_ai;
DROP TRIGGER IF EXISTS nodes_fts_ad;

-- Recreate papers insert trigger with consistent COALESCE
CREATE TRIGGER papers_fts_ai AFTER INSERT ON papers BEGIN
    INSERT INTO papers_fts(rowid, paper_id, title, abstract, authors)
    VALUES (new.rowid, new.paper_id, COALESCE(new.title, ''), COALESCE(new.abstract, ''), COALESCE(new.authors, ''));
END;

-- Fix: use DELETE FROM for standalone FTS5 table
CREATE TRIGGER papers_fts_ad AFTER DELETE ON papers BEGIN
    DELETE FROM papers_fts WHERE rowid = old.rowid;
END;

-- Recreate node_summaries insert trigger with consistent COALESCE
CREATE TRIGGER nodes_fts_ai AFTER INSERT ON node_summaries BEGIN
    INSERT INTO node_summaries_fts(rowid, paper_id, node_id, title, summary)
    VALUES (new.rowid, COALESCE(new.paper_id, ''), COALESCE(new.node_id, ''), COALESCE(new.title, ''), COALESCE(new.summary, ''));
END;

-- Fix: use DELETE FROM for standalone FTS5 table
CREATE TRIGGER nodes_fts_ad AFTER DELETE ON node_summaries BEGIN
    DELETE FROM node_summaries_fts WHERE rowid = old.rowid;
END;

-- Rebuild FTS5 indexes to fix existing data inconsistencies
DELETE FROM papers_fts;
INSERT INTO papers_fts(rowid, paper_id, title, abstract, authors)
SELECT rowid, paper_id, COALESCE(title, ''), COALESCE(abstract, ''), COALESCE(authors, '') FROM papers;

DELETE FROM node_summaries_fts;
INSERT INTO node_summaries_fts(rowid, paper_id, node_id, title, summary)
SELECT rowid, COALESCE(paper_id, ''), COALESCE(node_id, ''), COALESCE(title, ''), COALESCE(summary, '') FROM node_summaries;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Restore original triggers
DROP TRIGGER IF EXISTS papers_fts_ai;
DROP TRIGGER IF EXISTS papers_fts_ad;
DROP TRIGGER IF EXISTS nodes_fts_ai;
DROP TRIGGER IF EXISTS nodes_fts_ad;

CREATE TRIGGER papers_fts_ai AFTER INSERT ON papers BEGIN
    INSERT INTO papers_fts(paper_id, title, abstract, authors)
    VALUES (new.paper_id, new.title, COALESCE(new.abstract, ''), COALESCE(new.authors, ''));
END;
CREATE TRIGGER papers_fts_ad AFTER DELETE ON papers BEGIN
    INSERT INTO papers_fts(papers_fts, paper_id, title, abstract, authors)
    VALUES ('delete', old.paper_id, old.title, COALESCE(old.abstract, ''), COALESCE(old.authors, ''));
END;
CREATE TRIGGER nodes_fts_ai AFTER INSERT ON node_summaries BEGIN
    INSERT INTO node_summaries_fts(paper_id, node_id, title, summary)
    VALUES (new.paper_id, COALESCE(new.node_id, ''), COALESCE(new.title, ''), new.summary);
END;
CREATE TRIGGER nodes_fts_ad AFTER DELETE ON node_summaries BEGIN
    INSERT INTO node_summaries_fts(node_summaries_fts, paper_id, node_id, title, summary)
    VALUES ('delete', old.paper_id, COALESCE(old.node_id, ''), COALESCE(old.title, ''), old.summary);
END;

-- +goose StatementEnd
