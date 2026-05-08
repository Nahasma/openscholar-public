-- +goose Up
CREATE TABLE IF NOT EXISTS docx_exports (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id INTEGER NOT NULL REFERENCES docx_documents(id),
    format      TEXT NOT NULL,
    engine      TEXT,
    params_json TEXT,
    output_path TEXT NOT NULL,
    success     INTEGER NOT NULL DEFAULT 1,
    error_msg   TEXT,
    created_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_docx_exports_document ON docx_exports(document_id);

-- +goose Down
DROP TABLE IF EXISTS docx_exports;
