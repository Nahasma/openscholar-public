-- +goose Up
-- +goose StatementBegin

-- 缓存 Semantic Scholar 搜索结果，避免重复 API 调用
CREATE TABLE IF NOT EXISTS bib_entries (
    id                  TEXT PRIMARY KEY,
    session_id          TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    -- 论文元数据
    title               TEXT NOT NULL,
    authors             TEXT NOT NULL,       -- JSON array: [{"name": "..."}]
    year                INTEGER,
    venue               TEXT,
    abstract            TEXT,
    -- 外部标识
    doi                 TEXT,
    arxiv_id            TEXT,
    semantic_scholar_id TEXT,
    -- BibTeX 信息
    cite_key            TEXT NOT NULL,
    bib_type            TEXT NOT NULL DEFAULT 'misc',  -- article/inproceedings/misc
    -- 元数据
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_bib_entries_session ON bib_entries(session_id);
CREATE INDEX IF NOT EXISTS idx_bib_entries_cite_key ON bib_entries(cite_key);
CREATE INDEX IF NOT EXISTS idx_bib_entries_doi ON bib_entries(doi);
CREATE INDEX IF NOT EXISTS idx_bib_entries_arxiv ON bib_entries(arxiv_id);

CREATE TRIGGER IF NOT EXISTS update_bib_entries_updated_at
AFTER UPDATE ON bib_entries
BEGIN
    UPDATE bib_entries SET updated_at = strftime('%s', 'now')
    WHERE id = new.id;
END;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS update_bib_entries_updated_at;
DROP INDEX IF EXISTS idx_bib_entries_arxiv;
DROP INDEX IF EXISTS idx_bib_entries_doi;
DROP INDEX IF EXISTS idx_bib_entries_cite_key;
DROP INDEX IF EXISTS idx_bib_entries_session;
DROP TABLE IF EXISTS bib_entries;
-- +goose StatementEnd
