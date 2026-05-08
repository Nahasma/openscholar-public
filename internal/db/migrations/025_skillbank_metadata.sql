-- +goose Up

ALTER TABLE skills_index ADD COLUMN meta_json TEXT NOT NULL DEFAULT '{}';

-- +goose Down

-- SQLite cannot drop columns directly; keep schema backward-compatible.
