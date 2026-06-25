-- +goose Up
CREATE TABLE channels (
    id          TEXT PRIMARY KEY,
    name        TEXT,
    is_private  BOOLEAN,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id          TEXT PRIMARY KEY,
    username    TEXT,
    real_name   TEXT,
    is_bot      BOOLEAN,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE messages (
    channel_id  TEXT  NOT NULL REFERENCES channels(id),
    ts          TEXT  NOT NULL,
    thread_ts   TEXT,
    user_id     TEXT,
    text        TEXT,
    subtype     TEXT,
    raw         JSONB NOT NULL,
    posted_at   TIMESTAMPTZ,
    edited_at   TIMESTAMPTZ,
    search      TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', coalesce(text,''))) STORED,
    PRIMARY KEY (channel_id, ts)
);

CREATE INDEX messages_search_idx    ON messages USING GIN (search);
CREATE INDEX messages_thread_ts_idx ON messages (channel_id, thread_ts);
CREATE INDEX messages_posted_at_idx ON messages (posted_at);

-- +goose Down
DROP TABLE messages;
DROP TABLE users;
DROP TABLE channels;
