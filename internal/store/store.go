package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kjkondratuk/slack-mirror/internal/blobstore"
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
	pool  *pgxpool.Pool
	blobs blobstore.Blobstore // nil when file mirroring disabled
}

func New(pool *pgxpool.Pool) *PgStore { return &PgStore{pool: pool} }

// NewWithBlobs builds a store that GCs file blobs on message delete.
func NewWithBlobs(pool *pgxpool.Pool, blobs blobstore.Blobstore) *PgStore {
	return &PgStore{pool: pool, blobs: blobs}
}

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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Capture files linked to this message before the cascade removes the edges.
	var candidates []string
	rows, err := tx.Query(ctx, `SELECT file_id FROM message_files WHERE channel_id=$1 AND message_ts=$2`, channelID, ts)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM messages WHERE channel_id=$1 AND ts=$2`, channelID, ts); err != nil {
		return fmt.Errorf("delete message %s/%s: %w", channelID, ts, err)
	}

	var orphanURIs []string
	if len(candidates) > 0 {
		var orphanIDs []string
		orows, err := tx.Query(ctx, `
			SELECT id, coalesce(storage_uri,'') FROM files
			WHERE id = ANY($1)
			  AND NOT EXISTS (SELECT 1 FROM message_files mf WHERE mf.file_id = files.id)`, candidates)
		if err != nil {
			return err
		}
		for orows.Next() {
			var id, uri string
			if err := orows.Scan(&id, &uri); err != nil {
				orows.Close()
				return err
			}
			orphanIDs = append(orphanIDs, id)
			if uri != "" {
				orphanURIs = append(orphanURIs, uri)
			}
		}
		orows.Close()
		if err := orows.Err(); err != nil {
			return err
		}
		if len(orphanIDs) > 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM files WHERE id = ANY($1)`, orphanIDs); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Blobs live outside the DB: delete after the row commit succeeds.
	if s.blobs != nil {
		for _, uri := range orphanURIs {
			if err := s.blobs.Delete(ctx, uri); err != nil {
				return fmt.Errorf("gc blob %s: %w", uri, err)
			}
		}
	}
	return nil
}

func (s *PgStore) UpsertFile(ctx context.Context, r model.FileRow) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO files (id, name, title, mimetype, filetype, size, user_id, mode, is_external,
		                   storage_uri, sha256, download_state, raw, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,NULLIF($10,''),NULLIF($11,''),$12,$13, now())
		ON CONFLICT (id) DO UPDATE SET
			name=EXCLUDED.name, title=EXCLUDED.title, mimetype=EXCLUDED.mimetype,
			filetype=EXCLUDED.filetype, size=EXCLUDED.size, user_id=EXCLUDED.user_id,
			mode=EXCLUDED.mode, is_external=EXCLUDED.is_external, raw=EXCLUDED.raw`,
		r.ID, r.Name, r.Title, r.Mimetype, r.Filetype, r.Size, r.UserID, r.Mode, r.IsExternal,
		r.StorageURI, r.SHA256, r.DownloadState, r.Raw)
	if err != nil {
		return fmt.Errorf("upsert file %s: %w", r.ID, err)
	}
	return nil
}

func (s *PgStore) LinkFile(ctx context.Context, channelID, messageTS, fileID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO message_files (channel_id, message_ts, file_id)
		VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, channelID, messageTS, fileID)
	if err != nil {
		return fmt.Errorf("link file %s: %w", fileID, err)
	}
	return nil
}

func (s *PgStore) SetFileStored(ctx context.Context, id, storageURI, sha string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE files SET storage_uri=$2, sha256=$3, download_state='stored', downloaded_at=now() WHERE id=$1`,
		id, storageURI, sha)
	if err != nil {
		return fmt.Errorf("set file stored %s: %w", id, err)
	}
	return nil
}

func (s *PgStore) SetFileState(ctx context.Context, id, state string) error {
	_, err := s.pool.Exec(ctx, `UPDATE files SET download_state=$2 WHERE id=$1`, id, state)
	if err != nil {
		return fmt.Errorf("set file state %s: %w", id, err)
	}
	return nil
}

// ReconcileMessageFiles makes the message's file edges match keepFileIDs: it
// removes edges for files no longer attached and ref-count-GCs any file left
// with zero edges (deleting the row + blob). Used on message edits, where an
// edit may drop a previously-attached file.
func (s *PgStore) ReconcileMessageFiles(ctx context.Context, channelID, messageTS string, keepFileIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Edges to remove: files linked to this message but not in the keep set.
	var removed []string
	rows, err := tx.Query(ctx,
		`SELECT file_id FROM message_files
		 WHERE channel_id=$1 AND message_ts=$2 AND NOT (file_id = ANY($3))`,
		channelID, messageTS, keepFileIDs)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		removed = append(removed, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(removed) == 0 {
		return tx.Commit(ctx) // nothing to do
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM message_files
		 WHERE channel_id=$1 AND message_ts=$2 AND NOT (file_id = ANY($3))`,
		channelID, messageTS, keepFileIDs); err != nil {
		return err
	}

	var orphanURIs, orphanIDs []string
	orows, err := tx.Query(ctx, `
		SELECT id, coalesce(storage_uri,'') FROM files
		WHERE id = ANY($1)
		  AND NOT EXISTS (SELECT 1 FROM message_files mf WHERE mf.file_id = files.id)`, removed)
	if err != nil {
		return err
	}
	for orows.Next() {
		var id, uri string
		if err := orows.Scan(&id, &uri); err != nil {
			orows.Close()
			return err
		}
		orphanIDs = append(orphanIDs, id)
		if uri != "" {
			orphanURIs = append(orphanURIs, uri)
		}
	}
	orows.Close()
	if err := orows.Err(); err != nil {
		return err
	}
	if len(orphanIDs) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM files WHERE id = ANY($1)`, orphanIDs); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if s.blobs != nil {
		for _, uri := range orphanURIs {
			if err := s.blobs.Delete(ctx, uri); err != nil {
				return fmt.Errorf("gc blob %s: %w", uri, err)
			}
		}
	}
	return nil
}
