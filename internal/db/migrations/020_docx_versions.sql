-- +goose Up
CREATE TABLE IF NOT EXISTS docx_versions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id   INTEGER NOT NULL REFERENCES docx_documents(id),
    phase         TEXT NOT NULL,
    snapshot_path TEXT NOT NULL,
    created_by    TEXT NOT NULL DEFAULT 'ai',
    description   TEXT,
    created_at    INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_docx_versions_document ON docx_versions(document_id);

-- +goose Down
DROP TABLE IF EXISTS docx_versions;
