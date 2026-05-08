-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS permission_rules (
    id          TEXT PRIMARY KEY,
    tool_name   TEXT NOT NULL,
    action      TEXT NOT NULL DEFAULT '',
    path        TEXT NOT NULL DEFAULT '',
    decision    TEXT NOT NULL,
    scope       TEXT NOT NULL DEFAULT 'session',
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_permission_rules_tool_path
    ON permission_rules(tool_name, path);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_permission_rules_tool_path;
DROP TABLE IF EXISTS permission_rules;
-- +goose StatementEnd
