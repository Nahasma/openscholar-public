-- +goose Up
ALTER TABLE messages ADD COLUMN search_text TEXT NOT NULL DEFAULT '';

CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    message_id UNINDEXED,
    session_id UNINDEXED,
    role UNINDEXED,
    search_text,
    tokenize='unicode61'
);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS messages_ai_fts AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts(rowid, message_id, session_id, role, search_text)
  VALUES (new.rowid, new.id, new.session_id, new.role, new.search_text);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS messages_ad_fts AFTER DELETE ON messages BEGIN
  DELETE FROM messages_fts WHERE rowid = old.rowid;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS messages_au_fts AFTER UPDATE OF search_text, role, session_id ON messages BEGIN
  DELETE FROM messages_fts WHERE rowid = old.rowid;
  INSERT INTO messages_fts(rowid, message_id, session_id, role, search_text)
  VALUES (new.rowid, new.id, new.session_id, new.role, new.search_text);
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS messages_au_fts;
DROP TRIGGER IF EXISTS messages_ad_fts;
DROP TRIGGER IF EXISTS messages_ai_fts;
DROP TABLE IF EXISTS messages_fts;
-- SQLite 不支持稳定的 DROP COLUMN 回滚；search_text 列保留。
