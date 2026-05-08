-- +goose Up
ALTER TABLE sessions ADD COLUMN summary TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN tags TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN first_prompt TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN git_branch TEXT DEFAULT '';

-- +goose Down
-- SQLite does not support DROP COLUMN before 3.35.0; these columns will be ignored.
