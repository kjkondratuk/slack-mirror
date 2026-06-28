-- +goose Up
CREATE TABLE channels (
    id TEXT PRIMARY KEY, name TEXT, is_private INTEGER,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE users (
    id TEXT PRIMARY KEY, username TEXT, real_name TEXT, is_bot INTEGER,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE messages (
    channel_id TEXT NOT NULL REFERENCES channels(id),
    ts TEXT NOT NULL, thread_ts TEXT, user_id TEXT, text TEXT, subtype TEXT,
    raw TEXT NOT NULL, posted_at TEXT, edited_at TEXT,
    PRIMARY KEY (channel_id, ts)
);
CREATE INDEX messages_thread_ts_idx ON messages (channel_id, thread_ts);
CREATE INDEX messages_posted_at_idx ON messages (posted_at);

CREATE VIRTUAL TABLE messages_fts USING fts5(text, content='messages', content_rowid='rowid');

-- +goose StatementBegin
CREATE TRIGGER messages_ai AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts(rowid, text) VALUES (new.rowid, new.text);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER messages_ad AFTER DELETE ON messages BEGIN
  INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.rowid, old.text);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER messages_au AFTER UPDATE ON messages BEGIN
  INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.rowid, old.text);
  INSERT INTO messages_fts(rowid, text) VALUES (new.rowid, new.text);
END;
-- +goose StatementEnd

CREATE TABLE files (
    id TEXT PRIMARY KEY, name TEXT, title TEXT, mimetype TEXT, filetype TEXT,
    size INTEGER, user_id TEXT, mode TEXT, is_external INTEGER,
    storage_uri TEXT, sha256 TEXT, download_state TEXT NOT NULL DEFAULT 'pending',
    downloaded_at TEXT, raw TEXT NOT NULL, created_at TEXT
);
CREATE TABLE message_files (
    channel_id TEXT NOT NULL, message_ts TEXT NOT NULL,
    file_id TEXT NOT NULL REFERENCES files(id),
    PRIMARY KEY (channel_id, message_ts, file_id),
    FOREIGN KEY (channel_id, message_ts) REFERENCES messages(channel_id, ts) ON DELETE CASCADE
);
CREATE INDEX message_files_file_idx ON message_files (file_id);

-- +goose Down
DROP TABLE message_files;
DROP TABLE files;
DROP TRIGGER messages_au;
DROP TRIGGER messages_ad;
DROP TRIGGER messages_ai;
DROP TABLE messages_fts;
DROP TABLE messages;
DROP TABLE users;
DROP TABLE channels;
