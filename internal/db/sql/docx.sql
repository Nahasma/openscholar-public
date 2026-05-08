-- name: CreateDocxDocument :one
INSERT INTO docx_documents (pipeline_id, doc_type, template_id, template_ver, state, source_path, docx_path, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetDocxDocument :one
SELECT * FROM docx_documents WHERE id = ?;

-- name: ListDocxDocumentsByPipeline :many
SELECT * FROM docx_documents WHERE pipeline_id = ? ORDER BY created_at DESC;

-- name: UpdateDocxDocumentState :exec
UPDATE docx_documents SET state = ?, updated_at = ? WHERE id = ?;

-- name: CreateDocxVersion :one
INSERT INTO docx_versions (document_id, phase, snapshot_path, created_by, description, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListDocxVersions :many
SELECT * FROM docx_versions WHERE document_id = ? ORDER BY created_at DESC;

-- name: CreateDocxExport :one
INSERT INTO docx_exports (document_id, format, engine, params_json, output_path, success, error_msg, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListDocxExports :many
SELECT * FROM docx_exports WHERE document_id = ? ORDER BY created_at DESC;

-- name: CreateDocxDiagnostic :exec
INSERT INTO docx_diagnostics (document_id, version_id, rule_id, level, location_json, message, suggestion, auto_fix, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListDocxDiagnostics :many
SELECT * FROM docx_diagnostics WHERE document_id = ? ORDER BY level, rule_id;

-- name: DeleteDocxDiagnosticsByDocument :exec
DELETE FROM docx_diagnostics WHERE document_id = ?;

-- name: CreateDocxMaterial :one
INSERT INTO docx_materials (document_id, source_path, doc_type, parsed, parsed_at, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListDocxMaterials :many
SELECT * FROM docx_materials WHERE document_id = ? ORDER BY created_at;

-- name: UpdateDocxMaterialParsed :exec
UPDATE docx_materials SET parsed = 1, parsed_at = ? WHERE id = ?;
