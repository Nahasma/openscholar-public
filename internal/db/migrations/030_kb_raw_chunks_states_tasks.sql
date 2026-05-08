-- +goose Up
CREATE TABLE IF NOT EXISTS paper_chunks (
    paper_id    TEXT NOT NULL,
    chunk_id    TEXT NOT NULL,
    kind        TEXT NOT NULL,
    page_start  INTEGER,
    page_end    INTEGER,
    title       TEXT,
    content     TEXT NOT NULL,
    token_count INTEGER,
    source      TEXT,
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    PRIMARY KEY (paper_id, chunk_id),
    FOREIGN KEY (paper_id) REFERENCES papers(paper_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_paper_chunks_paper ON paper_chunks(paper_id);
CREATE INDEX IF NOT EXISTS idx_paper_chunks_kind ON paper_chunks(kind);

CREATE VIRTUAL TABLE IF NOT EXISTS paper_chunks_fts USING fts5(
    paper_id UNINDEXED,
    chunk_id UNINDEXED,
    kind UNINDEXED,
    page_start UNINDEXED,
    page_end UNINDEXED,
    title,
    content,
    tokenize='unicode61'
);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS paper_chunks_ai_fts AFTER INSERT ON paper_chunks BEGIN
    INSERT INTO paper_chunks_fts(rowid, paper_id, chunk_id, kind, page_start, page_end, title, content)
    VALUES (
        new.rowid,
        COALESCE(new.paper_id, ''),
        COALESCE(new.chunk_id, ''),
        COALESCE(new.kind, ''),
        COALESCE(new.page_start, 0),
        COALESCE(new.page_end, 0),
        COALESCE(new.title, ''),
        COALESCE(new.content, '')
    );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS paper_chunks_ad_fts AFTER DELETE ON paper_chunks BEGIN
    DELETE FROM paper_chunks_fts WHERE rowid = old.rowid;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS paper_chunks_au_fts AFTER UPDATE OF paper_id, chunk_id, kind, page_start, page_end, title, content ON paper_chunks BEGIN
    DELETE FROM paper_chunks_fts WHERE rowid = old.rowid;
    INSERT INTO paper_chunks_fts(rowid, paper_id, chunk_id, kind, page_start, page_end, title, content)
    VALUES (
        new.rowid,
        COALESCE(new.paper_id, ''),
        COALESCE(new.chunk_id, ''),
        COALESCE(new.kind, ''),
        COALESCE(new.page_start, 0),
        COALESCE(new.page_end, 0),
        COALESCE(new.title, ''),
        COALESCE(new.content, '')
    );
END;
-- +goose StatementEnd

INSERT OR IGNORE INTO paper_chunks (
    paper_id,
    chunk_id,
    kind,
    page_start,
    page_end,
    title,
    content,
    token_count,
    source,
    created_at
)
SELECT
    nc.paper_id,
    nc.node_id,
    CASE WHEN nc.node_id LIKE 'page_%' THEN 'page' ELSE 'section' END,
    ns.start_page,
    ns.end_page,
    COALESCE(ns.title, nc.node_id),
    COALESCE(nc.content, ''),
    nc.token_count,
    COALESCE(nc.source, 'legacy_node_contents'),
    COALESCE(nc.created_at, strftime('%s','now'))
FROM node_contents nc
LEFT JOIN node_summaries ns
  ON ns.paper_id = nc.paper_id
 AND ns.node_id = nc.node_id;

INSERT OR IGNORE INTO paper_chunks (
    paper_id,
    chunk_id,
    kind,
    page_start,
    page_end,
    title,
    content,
    token_count,
    source,
    created_at
)
SELECT
    ns.paper_id,
    ns.node_id,
    CASE WHEN ns.node_id LIKE 'page_%' THEN 'page' ELSE 'section' END,
    ns.start_page,
    ns.end_page,
    COALESCE(ns.title, ns.node_id),
    '',
    NULL,
    'legacy_outline',
    strftime('%s','now')
FROM node_summaries ns
LEFT JOIN node_contents nc
  ON nc.paper_id = ns.paper_id
 AND nc.node_id = ns.node_id
WHERE nc.paper_id IS NULL;

INSERT OR REPLACE INTO paper_chunks_fts(rowid, paper_id, chunk_id, kind, page_start, page_end, title, content)
SELECT
    pc.rowid,
    COALESCE(pc.paper_id, ''),
    COALESCE(pc.chunk_id, ''),
    COALESCE(pc.kind, ''),
    COALESCE(pc.page_start, 0),
    COALESCE(pc.page_end, 0),
    COALESCE(pc.title, ''),
    COALESCE(pc.content, '')
FROM paper_chunks pc;

CREATE TABLE IF NOT EXISTS paper_index_states (
    paper_id                   TEXT PRIMARY KEY,
    index_level                TEXT,
    raw_parse_status           TEXT,
    content_available          INTEGER NOT NULL DEFAULT 0,
    content_bytes              INTEGER NOT NULL DEFAULT 0,
    flat_page_index_available  INTEGER NOT NULL DEFAULT 0,
    fts_status                 TEXT,
    semantic_tree_status       TEXT,
    semantic_tree_available    INTEGER NOT NULL DEFAULT 0,
    semantic_tree_error        TEXT,
    semantic_tree_task_id      TEXT,
    extractor                  TEXT,
    fallback_reason            TEXT,
    total_pages                INTEGER,
    total_tokens               INTEGER,
    source_file_available      INTEGER,
    file_hash                  TEXT,
    updated_at                 INTEGER NOT NULL,
    FOREIGN KEY (paper_id) REFERENCES papers(paper_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_paper_index_states_level ON paper_index_states(index_level);
CREATE INDEX IF NOT EXISTS idx_paper_index_states_raw_parse ON paper_index_states(raw_parse_status);
CREATE INDEX IF NOT EXISTS idx_paper_index_states_semantic ON paper_index_states(semantic_tree_status);

INSERT INTO paper_index_states (
    paper_id,
    index_level,
    raw_parse_status,
    content_available,
    content_bytes,
    flat_page_index_available,
    fts_status,
    semantic_tree_status,
    semantic_tree_available,
    semantic_tree_error,
    semantic_tree_task_id,
    extractor,
    fallback_reason,
    total_pages,
    total_tokens,
    source_file_available,
    file_hash,
    updated_at
)
SELECT
    p.paper_id,
    CASE
        WHEN COALESCE(c.content_bytes, 0) > 0 AND COALESCE(t.semantic_tree_available, 0) = 1 THEN 'full_tree'
        WHEN COALESCE(c.content_bytes, 0) > 0 THEN 'simple_full_text'
        WHEN COALESCE(s.summary_count, 0) > 0 THEN 'summary_only'
        ELSE 'unknown'
    END,
    CASE
        WHEN COALESCE(c.content_bytes, 0) > 0 THEN 'ready'
        WHEN COALESCE(s.summary_count, 0) > 0 THEN 'partial'
        ELSE 'pending'
    END,
    CASE WHEN COALESCE(c.content_bytes, 0) > 0 THEN 1 ELSE 0 END,
    COALESCE(c.content_bytes, 0),
    CASE
        WHEN COALESCE(t.flat_page_legacy, 0) = 1 THEN 1
        WHEN COALESCE(pg.page_chunk_count, 0) > 0 THEN 1
        ELSE 0
    END,
    CASE
        WHEN COALESCE(c.chunk_count, 0) > 0 THEN 'ready'
        ELSE 'missing'
    END,
    CASE
        WHEN COALESCE(t.semantic_tree_available, 0) = 1 THEN 'ready'
        ELSE 'unknown'
    END,
    COALESCE(t.semantic_tree_available, 0),
    NULL,
    NULL,
    t.extractor,
    t.fallback_reason,
    t.total_pages,
    t.total_tokens,
    CASE
        WHEN COALESCE(p.file_path, p.pdf_path, '') != '' THEN 1
        ELSE 0
    END,
    NULL,
    strftime('%s','now')
FROM papers p
LEFT JOIN (
    SELECT
        paper_id,
        COUNT(*) AS chunk_count,
        SUM(LENGTH(COALESCE(content, ''))) AS content_bytes
    FROM paper_chunks
    GROUP BY paper_id
) c ON c.paper_id = p.paper_id
LEFT JOIN (
    SELECT
        paper_id,
        SUM(CASE WHEN chunk_id LIKE 'page_%' OR kind = 'page' THEN 1 ELSE 0 END) AS page_chunk_count
    FROM paper_chunks
    GROUP BY paper_id
) pg ON pg.paper_id = p.paper_id
LEFT JOIN (
    SELECT
        paper_id,
        COUNT(*) AS summary_count
    FROM node_summaries
    GROUP BY paper_id
) s ON s.paper_id = p.paper_id
LEFT JOIN (
    SELECT
        pt.paper_id,
        pt.total_pages,
        pt.total_tokens,
        COALESCE(json_extract(pt.index_metadata, '$.extractor'), '') AS extractor,
        COALESCE(json_extract(pt.index_metadata, '$.fallback_reason'), '') AS fallback_reason,
        CASE
            WHEN pt.model_used = 'simple_extract'
              OR COALESCE(json_extract(pt.index_metadata, '$.extractor'), '') = 'simple_extract'
            THEN 1
            ELSE 0
        END AS flat_page_legacy,
        CASE
            WHEN pt.model_used = 'simple_extract'
              OR COALESCE(json_extract(pt.index_metadata, '$.extractor'), '') = 'simple_extract'
            THEN 0
            ELSE 1
        END AS semantic_tree_available
    FROM paper_trees pt
) t ON t.paper_id = p.paper_id
ON CONFLICT(paper_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS kb_tasks (
    task_id        TEXT PRIMARY KEY,
    paper_id       TEXT,
    task_type      TEXT NOT NULL,
    status         TEXT NOT NULL,
    progress       REAL,
    input_json     TEXT,
    result_json    TEXT,
    result_ref     TEXT,
    error          TEXT,
    lease_owner    TEXT,
    lease_until    INTEGER,
    attempt_count  INTEGER NOT NULL DEFAULT 0,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    FOREIGN KEY (paper_id) REFERENCES papers(paper_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_kb_tasks_paper ON kb_tasks(paper_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_kb_tasks_status ON kb_tasks(status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_kb_tasks_lease ON kb_tasks(lease_until);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_tasks_lease;
DROP INDEX IF EXISTS idx_kb_tasks_status;
DROP INDEX IF EXISTS idx_kb_tasks_paper;
DROP TABLE IF EXISTS kb_tasks;

DROP INDEX IF EXISTS idx_paper_index_states_semantic;
DROP INDEX IF EXISTS idx_paper_index_states_raw_parse;
DROP INDEX IF EXISTS idx_paper_index_states_level;
DROP TABLE IF EXISTS paper_index_states;

DROP TRIGGER IF EXISTS paper_chunks_au_fts;
DROP TRIGGER IF EXISTS paper_chunks_ad_fts;
DROP TRIGGER IF EXISTS paper_chunks_ai_fts;
DROP TABLE IF EXISTS paper_chunks_fts;
DROP INDEX IF EXISTS idx_paper_chunks_kind;
DROP INDEX IF EXISTS idx_paper_chunks_paper;
DROP TABLE IF EXISTS paper_chunks;
