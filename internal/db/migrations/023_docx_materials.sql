-- +goose Up
CREATE TABLE IF NOT EXISTS docx_materials (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id INTEGER NOT NULL REFERENCES docx_documents(id),
    source_path TEXT NOT NULL,
    doc_type    TEXT NOT NULL,
    parsed      INTEGER NOT NULL DEFAULT 0,
    parsed_at   INTEGER,
    created_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_docx_materials_document ON docx_materials(document_id);

-- +goose Down
DROP TABLE IF EXISTS docx_materials;
