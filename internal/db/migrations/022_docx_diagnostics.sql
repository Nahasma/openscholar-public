-- +goose Up
CREATE TABLE IF NOT EXISTS docx_diagnostics (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id   INTEGER NOT NULL REFERENCES docx_documents(id),
    version_id    INTEGER REFERENCES docx_versions(id),
    rule_id       TEXT NOT NULL,
    level         TEXT NOT NULL,
    location_json TEXT,
    message       TEXT NOT NULL,
    suggestion    TEXT,
    auto_fix      INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_docx_diagnostics_document ON docx_diagnostics(document_id);
CREATE INDEX IF NOT EXISTS idx_docx_diagnostics_rule ON docx_diagnostics(rule_id);

-- +goose Down
DROP TABLE IF EXISTS docx_diagnostics;
