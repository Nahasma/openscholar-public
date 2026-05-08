-- +goose Up
-- +goose StatementBegin

-- 论文元数据（全局，非 session 级别）
CREATE TABLE IF NOT EXISTS papers (
    paper_id    TEXT PRIMARY KEY,
    title       TEXT NOT NULL,
    authors     TEXT,                                    -- JSON array: [{"name": "..."}]
    year        INTEGER,
    venue       TEXT,
    abstract    TEXT,
    doi         TEXT,
    arxiv_id    TEXT,
    pdf_path    TEXT,
    indexed_at  INTEGER DEFAULT (strftime('%s','now'))
);

CREATE INDEX IF NOT EXISTS idx_papers_year ON papers(year);
CREATE INDEX IF NOT EXISTS idx_papers_venue ON papers(venue);
CREATE INDEX IF NOT EXISTS idx_papers_doi ON papers(doi);
CREATE INDEX IF NOT EXISTS idx_papers_arxiv ON papers(arxiv_id);

-- 论文树索引（每篇论文一棵树）
CREATE TABLE IF NOT EXISTS paper_trees (
    paper_id        TEXT PRIMARY KEY REFERENCES papers(paper_id) ON DELETE CASCADE,
    tree_json       TEXT NOT NULL,                       -- PageIndex 完整树结构 JSON
    doc_description TEXT,                                -- LLM 生成的一句话描述
    total_pages     INTEGER,
    total_tokens    INTEGER,
    model_used      TEXT,
    version         INTEGER DEFAULT 1,
    created_at      INTEGER DEFAULT (strftime('%s','now'))
);

-- 节点摘要（扁平化，用于 SQL 查询和 FTS5）
CREATE TABLE IF NOT EXISTS node_summaries (
    paper_id    TEXT REFERENCES papers(paper_id) ON DELETE CASCADE,
    node_id     TEXT,                                    -- 四位零填充（如 "0003"）
    title       TEXT,
    start_page  INTEGER,
    end_page    INTEGER,
    summary     TEXT NOT NULL,
    PRIMARY KEY (paper_id, node_id)
);

CREATE INDEX IF NOT EXISTS idx_node_summaries_paper ON node_summaries(paper_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_node_summaries_paper;
DROP TABLE IF EXISTS node_summaries;
DROP TABLE IF EXISTS paper_trees;
DROP INDEX IF EXISTS idx_papers_arxiv;
DROP INDEX IF EXISTS idx_papers_doi;
DROP INDEX IF EXISTS idx_papers_venue;
DROP INDEX IF EXISTS idx_papers_year;
DROP TABLE IF EXISTS papers;
-- +goose StatementEnd
