# slack-mirror

Mirror the **current state** of selected Slack channels into Postgres so the content stays
searchable by a person or an LLM agent after Slack's free-tier 90-day window hides it.

This is a **mirror, not an archive**: it stores only the latest representation of each
message. Edit a message in Slack and the stored row updates; delete it and the stored row
is deleted. There is no revision history. The image is generic and business-agnostic —
configured entirely via environment variables.

## How it works

One Go binary, two subcommands:

- `slack-mirror serve` — a long-running consumer that subscribes to Slack message events
  over **Socket Mode** (an outbound WebSocket; no inbound endpoint) and writes each new
  message, edit, and delete to Postgres in near-real-time.
- `slack-mirror backfill` — a one-shot pass that pages the currently-visible history of the
  allowlisted channels into the same tables, then exits.

Every write is **idempotent** (upsert/delete keyed on `(channel_id, ts)`), so Slack's
at-least-once delivery and any brief overlap during a redeploy converge to the same state.

```
            Socket Mode (WSS, outbound)
   Slack  ───────────────────────────────►  serve  ──►  Postgres
                                              │            ▲
   Slack Web API (conversations.history) ─────┘  backfill ─┘
```

## Quick start (local)

1. Start a Postgres (any instance works; here's a throwaway one):
   ```bash
   docker run --rm -d --name slackmirror-pg \
     -e POSTGRES_PASSWORD=pg -e POSTGRES_DB=mirror -p 5432:5432 postgres:16
   ```
2. Create a Slack app (see **Slack app setup** below) and export its tokens:
   ```bash
   export SLACK_APP_TOKEN=xapp-...      # app-level token, connections:write
   export SLACK_BOT_TOKEN=xoxb-...      # bot token
   export DATABASE_URL='postgres://postgres:pg@localhost:5432/mirror?sslmode=disable'
   ```
3. Run the consumer (migrations are applied automatically on startup):
   ```bash
   go run ./cmd/slack-mirror serve
   ```
4. Post a message in a channel the bot is a member of, then query it:
   ```bash
   psql "$DATABASE_URL" -c 'select channel_id, ts, text from messages;'
   ```

## Configuration

All configuration is via environment variables.

| Variable | Required | Description |
|---|---|---|
| `SLACK_APP_TOKEN` | yes (serve) | App-level token (`xapp-…`) with `connections:write`, for Socket Mode |
| `SLACK_BOT_TOKEN` | yes | Bot token (`xoxb-…`) — Web API calls + event payloads |
| `STORE_BACKEND` | no | Storage backend: `postgres` (default) / `sqlite` |
| `SQLITE_PATH` | no | SQLite DB file path when `STORE_BACKEND=sqlite` (default `/data/mirror.db`) |
| `CHANNEL_ALLOWLIST` | no | Comma-separated channel IDs to persist; empty = all channels the bot is in (required for `backfill`) |
| `CHANNEL_DENYLIST` | no | Comma-separated channel IDs to always ignore |
| `PERSIST_SUBTYPES` | no | If set, ONLY these message subtypes are stored (normal messages always are) |
| `SKIP_SUBTYPES` | no | Subtypes to drop; defaults to system noise (channel_join/leave/topic/purpose/name/archive/unarchive) |
| `DATABASE_URL` | no | Direct Postgres DSN for local/non-GCP runs; bypasses the Cloud SQL connector |
| `CLOUDSQL_INSTANCE` | no* | Cloud SQL instance connection name `project:region:instance` |
| `DB_NAME` | no* | Database name (with the Cloud SQL connector) |
| `DB_USER` | no* | DB user (IAM user when `DB_IAM_AUTH=true`) |
| `DB_PASSWORD` | no | DB password when not using IAM auth |
| `DB_IAM_AUTH` | no | `true` to use Cloud SQL IAM database authentication |
| `DB_PRIVATE_IP` | no | `true` to dial the instance's private IP (requires VPC egress) |
| `BACKFILL_DAYS` | no | `backfill`: how far back to page (default 90) |
| `LOG_LEVEL` | no | `debug`/`info`/`warn`/`error` (default `info`) |
| `PORT` | no | Health/metrics listener port (default `8080`) |
| `FILE_STORAGE` | no | Enable file-byte mirroring: `none` (default) / `local` / `gcs` |
| `FILE_BUCKET` | no | GCS bucket for file bytes when `FILE_STORAGE=gcs` |
| `FILE_DIR` | no | Local directory for file bytes when `FILE_STORAGE=local` |
| `FILE_MAX_BYTES` | no | Skip files larger than this many bytes (0/unset = no limit) |
| `FILE_MIME_ALLOWLIST` | no | Comma-separated mimetypes to store; empty = all |

\* Provide a database target as EITHER `DATABASE_URL` OR (`CLOUDSQL_INSTANCE` + `DB_NAME` +
`DB_USER`). The Cloud SQL connector path uses the [Cloud SQL Go connector](https://cloud.google.com/sql/docs/postgres/connect-connectors)
with pgx and (optionally) IAM auth.

## Slack app setup

Create an **internal / custom app inside your workspace** — do not distribute it. Internal
apps keep full Tier-3 Web API rate limits, which a large backfill needs.

App manifest (trim the `groups`/private-channel scopes if you only need public channels):

```yaml
display_information:
  name: Channel Mirror
settings:
  socket_mode_enabled: true
  event_subscriptions:
    bot_events:
      - message.channels      # public channels the bot is in
      - message.groups        # private channels the bot is invited to
oauth_config:
  scopes:
    bot:
      - channels:history
      - groups:history
      - channels:read
      - groups:read
      - users:read
      - files:read            # only needed when file mirroring (FILE_STORAGE) is enabled
```

You need two tokens: a **bot token** (`xoxb-…`) and an **app-level token** (`xapp-…`) with
`connections:write`. Membership is the real gate — the bot only receives events for
channels it is a member of, and must be explicitly invited to private channels.

## Backfill

Run once to seed current state:

```bash
export CHANNEL_ALLOWLIST=C0123456789,C0987654321
slack-mirror backfill
```

It pages `conversations.history` per allowlisted channel and `conversations.replies` per
thread, upserting into the same tables as live capture (idempotent, so it's safe to run
alongside or after `serve`).

**History window:** on the **free** Slack plan the Web API only returns the visible ~90
days (`BACKFILL_DAYS` defaults to 90); older messages are hidden and unreachable. To seed
the full prior 12 months, upgrade the workspace to Pro for a single month (this reveals the
previous 12 months), run the backfill, then downgrade. Anything older than 12 months is
already permanently deleted.

## Schema

```sql
CREATE TABLE channels (
    id TEXT PRIMARY KEY, name TEXT, is_private BOOLEAN,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE users (
    id TEXT PRIMARY KEY, username TEXT, real_name TEXT, is_bot BOOLEAN,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE messages (
    channel_id TEXT NOT NULL REFERENCES channels(id),
    ts         TEXT NOT NULL,
    thread_ts  TEXT,
    user_id    TEXT,
    text       TEXT,
    subtype    TEXT,
    raw        JSONB NOT NULL,             -- full latest message payload
    posted_at  TIMESTAMPTZ,
    edited_at  TIMESTAMPTZ,
    search     TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', coalesce(text,''))) STORED,
    PRIMARY KEY (channel_id, ts)
);
```

Migrations are embedded in the binary and applied automatically on `serve`/`backfill`
startup.

## File attachments (optional)

By default `slack-mirror` stores only message metadata; the file objects in a message's
`files[]` are preserved inside the `raw` JSONB but the **bytes** are not downloaded. Set
`FILE_STORAGE` to also mirror the bytes so attachments survive Slack's retention window.

- `FILE_STORAGE=local` writes bytes under `FILE_DIR` (good for local/dev).
- `FILE_STORAGE=gcs` writes bytes to the `FILE_BUCKET` Google Cloud Storage bucket.

File **bytes** go to that object store; file **metadata** (name, mimetype, size, uploader,
a `sha256`, and the storage URI) goes to a `files` table, with a `message_files` edge table
linking each file to the messages that reference it. Bytes never enter Postgres.

Enabling this requires the **`files:read`** Slack scope (see the manifest above). Optional
filters: `FILE_MAX_BYTES` skips files above a size, `FILE_MIME_ALLOWLIST` restricts which
mimetypes are stored. Externally-hosted files (`mode: external`) are skipped — only
Slack-hosted bytes are downloaded.

**Delete semantics:** file deletes propagate, matching messages. When a mirrored message is
deleted in Slack, its `message_files` edges are removed, and any file left with no remaining
references has its row and its stored bytes garbage-collected. (Slack's 90-day free-tier
*hiding* is not a delete event, so hidden content is retained.)

```sql
CREATE TABLE files (
    id TEXT PRIMARY KEY, name TEXT, title TEXT, mimetype TEXT, filetype TEXT,
    size BIGINT, user_id TEXT, mode TEXT, is_external BOOLEAN,
    storage_uri TEXT, sha256 TEXT, download_state TEXT NOT NULL DEFAULT 'pending',
    downloaded_at TIMESTAMPTZ, raw JSONB NOT NULL, created_at TIMESTAMPTZ
);
CREATE TABLE message_files (
    channel_id TEXT NOT NULL, message_ts TEXT NOT NULL,
    file_id TEXT NOT NULL REFERENCES files(id),
    PRIMARY KEY (channel_id, message_ts, file_id),
    FOREIGN KEY (channel_id, message_ts) REFERENCES messages(channel_id, ts) ON DELETE CASCADE
);
```

## Querying

The store is plain Postgres:

```sql
-- Full-text search, newest first
SELECT channel_id, ts, text
FROM messages
WHERE search @@ websearch_to_tsquery('english', 'deploy rollback')
ORDER BY posted_at DESC
LIMIT 50;

-- Reconstruct a thread
SELECT ts, user_id, text
FROM messages
WHERE channel_id = $1 AND thread_ts = $2
ORDER BY ts;
```

## Storage backends

`slack-mirror` supports two storage backends, selected with `STORE_BACKEND`. Capture
semantics, the schema, and file mirroring are identical across both — only where the data
lives differs.

### `postgres` (default)
The existing path: Cloud SQL via the Go connector + IAM, or any Postgres via `DATABASE_URL`.
Full-text search uses a `tsvector`/GIN index. Backfill can run as a separate concurrent job.

### `sqlite`
An embedded SQLite database at `SQLITE_PATH` (default `/data/mirror.db`) — no Cloud SQL, no
separate database server. Full-text search uses SQLite **FTS5** (BM25), kept current by
triggers. This is the cheap, isolated option for storing a large corpus.

Search uses SQLite FTS5 query syntax (so characters like `"`, `*`, `:`, and words like `NEAR`/`OR` are operators) — sanitize or quote untrusted user input before searching.

```bash
export STORE_BACKEND=sqlite SQLITE_PATH=/data/mirror.db
slack-mirror serve
```

**Durability (Litestream → GCS):** the app itself just reads/writes the local SQLite file and
checkpoints the WAL on shutdown. To make that file durable in a bucket, run the binary under
[Litestream](https://litestream.io), which continuously replicates the WAL to GCS and restores
it on cold start. An optional image variant and an example config are provided:
[`Dockerfile.litestream`](Dockerfile.litestream) and
[`deploy/litestream.example.yml`](deploy/litestream.example.yml). The example config is
env-driven — set `LITESTREAM_REPLICA_BUCKET` (target GCS bucket) and `LITESTREAM_REPLICA_PATH`
(object prefix, e.g. `db`) in the runtime environment; Litestream 0.5.x expands them at start,
so the generic image targets any bucket without a rebuild. Keep the actual bucket and
Litestream config in your private deploy repo.

**Single-writer:** SQLite is single-writer, so with this backend `serve` and `backfill` must
not run at the same time against the same database — run `backfill` as a one-off while `serve`
is stopped (or seed before first start). The Postgres backend keeps its concurrent-backfill
behavior.

## Deploying

CI publishes a multi-stage, distroless image to the GitHub Container Registry on each
version tag:

```
ghcr.io/<owner>/slack-mirror:<version>
```

The container is generic — point it at your database and Slack tokens via the environment
variables above. It is designed to run as a single always-on instance (e.g. a Cloud Run
**worker pool** pinned to one instance, which matches a Socket Mode consumer and needs no
inbound port). Run `backfill` once as a separate one-off job using the same image with
`--args=backfill` (Cloud Run) or `docker run … backfill`. Handle `SIGTERM` is built in:
the consumer drains and exits cleanly on revision replacement.

Keep deployment specifics (Terraform/Cloud Run/Cloud SQL/secrets/IAM and your channel
allowlist) in a separate private deployment repo; nothing about a particular organization
belongs in this image.

## Development

```bash
go build ./...
go test ./...          # DB-backed tests skip unless TEST_DATABASE_URL is set
```

To run the database-backed tests:

```bash
export TEST_DATABASE_URL='postgres://postgres:pg@localhost:5432/mirror?sslmode=disable'
go test ./...
```

## License

MIT — see [LICENSE](LICENSE).
