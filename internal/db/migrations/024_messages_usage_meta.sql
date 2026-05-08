-- +goose Up
ALTER TABLE messages ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN cache_creation_input_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN cache_read_input_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN meta TEXT NOT NULL DEFAULT '{}';

-- +goose Down
-- SQLite 不支持稳定的 DROP COLUMN 回滚；此迁移为前向兼容。
