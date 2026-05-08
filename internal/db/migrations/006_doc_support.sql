-- +goose Up
-- +goose StatementBegin

-- 扩展 papers 表支持多格式文档
ALTER TABLE papers ADD COLUMN file_path TEXT;
ALTER TABLE papers ADD COLUMN doc_type TEXT DEFAULT 'pdf';

-- 回填: 将现有 pdf_path 复制到 file_path
UPDATE papers SET file_path = pdf_path WHERE pdf_path IS NOT NULL AND pdf_path != '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- SQLite 3.35+ 支持 ALTER TABLE DROP COLUMN
-- 对于旧版本此迁移不可逆

-- +goose StatementEnd
