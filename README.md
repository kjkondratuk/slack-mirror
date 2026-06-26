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
