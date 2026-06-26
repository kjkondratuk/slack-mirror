-- +goose Up
CREATE TABLE files (
    id             TEXT PRIMARY KEY,
    name           TEXT,
    title          TEXT,
    mimetype       TEXT,
    filetype       TEXT,
    size           BIGINT,
    user_id        TEXT,
    mode           TEXT,
    is_external    BOOLEAN,
    storage_uri    TEXT,
    sha256         TEXT,
    download_state TEXT NOT NULL DEFAULT 'pending',
    downloaded_at  TIMESTAMPTZ,
    raw            JSONB NOT NULL,
    created_at     TIMESTAMPTZ
);

CREATE TABLE message_files (
    channel_id TEXT NOT NULL,
    message_ts TEXT NOT NULL,
    file_id    TEXT NOT NULL REFERENCES files(id),
    PRIMARY KEY (channel_id, message_ts, file_id),
    FOREIGN KEY (channel_id, message_ts) REFERENCES messages(channel_id, ts) ON DELETE CASCADE
);
CREATE INDEX message_files_file_idx ON message_files (file_id);

-- +goose Down
DROP TABLE message_files;
DROP TABLE files;
