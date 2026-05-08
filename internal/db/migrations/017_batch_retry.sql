-- +goose Up
-- +goose StatementBegin
ALTER TABLE batch_jobs ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE batch_jobs ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 3;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite 不支持 DROP COLUMN，此处无操作
-- +goose StatementEnd
