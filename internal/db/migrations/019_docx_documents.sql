-- +goose Up
CREATE TABLE IF NOT EXISTS docx_documents (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id  INTEGER REFERENCES pipelines(id),
    doc_type     TEXT NOT NULL,
    template_id  TEXT,
    template_ver TEXT,
    state        TEXT NOT NULL DEFAULT 'draft',
    source_path  TEXT,
    docx_path    TEXT,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_docx_documents_pipeline ON docx_documents(pipeline_id);
CREATE INDEX IF NOT EXISTS idx_docx_documents_state ON docx_documents(state);

-- +goose Down
DROP TABLE IF EXISTS docx_documents;
