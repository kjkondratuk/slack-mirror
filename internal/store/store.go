package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kjkondratuk/slack-mirror/internal/model"
)

// Store is the write surface consumed by the live consumer and backfill.
type Store interface {
	UpsertMessage(ctx context.Context, m model.MessageRow) error
	DeleteMessage(ctx context.Context, channelID, ts string) error
	UpsertChannel(ctx context.Context, c model.Channel) error
	UpsertUser(ctx context.Context, u model.User) error
	Close()
}

type PgStore struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *PgStore { return &PgStore{pool: pool} }

func (s *PgStore) Close() { s.pool.Close() }

func (s *PgStore) UpsertChannel(ctx context.Context, c model.Channel) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO channels (id, name, is_private, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name,
		                               is_private = EXCLUDED.is_private,
		                               updated_at = now()`,
		c.ID, c.Name, c.IsPrivate)
	if err != nil {
		return fmt.Errorf("upsert channel %s: %w", c.ID, err)
	}
	return nil
}

func (s *PgStore) UpsertUser(ctx context.Context, u model.User) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, username, real_name, is_bot, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (id) DO UPDATE SET username = EXCLUDED.username,
		                               real_name = EXCLUDED.real_name,
		                               is_bot = EXCLUDED.is_bot,
		                               updated_at = now()`,
		u.ID, u.Username, u.RealName, u.IsBot)
	if err != nil {
		return fmt.Errorf("upsert user %s: %w", u.ID, err)
	}
	return nil
}

func (s *PgStore) UpsertMessage(ctx context.Context, m model.MessageRow) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO messages (channel_id, ts, thread_ts, user_id, text, subtype, raw, posted_at, edited_at)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5, NULLIF($6,''), $7, $8, $9)
		ON CONFLICT (channel_id, ts) DO UPDATE SET
			thread_ts = EXCLUDED.thread_ts,
			user_id   = EXCLUDED.user_id,
			text      = EXCLUDED.text,
			subtype   = EXCLUDED.subtype,
			raw       = EXCLUDED.raw,
			posted_at = EXCLUDED.posted_at,
			edited_at = EXCLUDED.edited_at`,
		m.ChannelID, m.TS, m.ThreadTS, m.UserID, m.Text, m.Subtype, m.Raw, m.PostedAt, m.EditedAt)
	if err != nil {
		return fmt.Errorf("upsert message %s/%s: %w", m.ChannelID, m.TS, err)
	}
	return nil
}

func (s *PgStore) DeleteMessage(ctx context.Context, channelID, ts string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM messages WHERE channel_id=$1 AND ts=$2`, channelID, ts)
	if err != nil {
		return fmt.Errorf("delete message %s/%s: %w", channelID, ts, err)
	}
	return nil
}
