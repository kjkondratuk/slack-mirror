// Package sqlitestore is an embedded SQLite + FTS5 storage backend implementing
// the same write interfaces as internal/store (the Postgres backend).
package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kjkondratuk/slack-mirror/internal/blobstore"
	"github.com/kjkondratuk/slack-mirror/internal/model"
	_ "modernc.org/sqlite"
)

type Store struct {
	db    *sql.DB
	blobs blobstore.Blobstore // nil when file mirroring disabled
}

// Open opens (creating if needed) the SQLite DB at path, applies migrations, and
// returns a ready Store. blobs may be nil. Caller owns Close().
func Open(path string, blobs blobstore.Blobstore) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create sqlite dir: %w", err)
		}
	}
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Single writer: serialize all access to avoid SQLITE_BUSY (matches the
	// single-instance deployment model).
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, blobs: blobs}, nil
}

// New / NewWithBlobs mirror the Postgres store constructors for symmetry.
func New(path string) (*Store, error)                                     { return Open(path, nil) }
func NewWithBlobs(path string, blobs blobstore.Blobstore) (*Store, error) { return Open(path, blobs) }

// Close checkpoints the WAL into the main DB file (so the file is self-contained
// for litestream/backup), then closes.
func (s *Store) Close() {
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	_ = s.db.Close()
}

func (s *Store) UpsertChannel(ctx context.Context, c model.Channel) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO channels (id, name, is_private, updated_at)
		VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, is_private=excluded.is_private,
			updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		c.ID, c.Name, c.IsPrivate)
	if err != nil {
		return fmt.Errorf("upsert channel %s: %w", c.ID, err)
	}
	return nil
}

func (s *Store) UpsertUser(ctx context.Context, u model.User) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, username, real_name, is_bot, updated_at)
		VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		ON CONFLICT(id) DO UPDATE SET username=excluded.username, real_name=excluded.real_name,
			is_bot=excluded.is_bot, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		u.ID, u.Username, u.RealName, u.IsBot)
	if err != nil {
		return fmt.Errorf("upsert user %s: %w", u.ID, err)
	}
	return nil
}

func (s *Store) UpsertMessage(ctx context.Context, m model.MessageRow) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messages (channel_id, ts, thread_ts, user_id, text, subtype, raw, posted_at, edited_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(channel_id, ts) DO UPDATE SET
			thread_ts=excluded.thread_ts, user_id=excluded.user_id, text=excluded.text,
			subtype=excluded.subtype, raw=excluded.raw, posted_at=excluded.posted_at, edited_at=excluded.edited_at`,
		m.ChannelID, m.TS, nullStr(m.ThreadTS), nullStr(m.UserID), m.Text, nullStr(m.Subtype),
		string(m.Raw), nullStr(rfc(m.PostedAt)), rfcPtr(m.EditedAt))
	if err != nil {
		return fmt.Errorf("upsert message %s/%s: %w", m.ChannelID, m.TS, err)
	}
	return nil
}

// DeleteMessage removes the message and ref-count-GCs any file left with zero
// edges (deleting the row + blob). SQLite dialect of the Postgres store's GC.
func (s *Store) DeleteMessage(ctx context.Context, channelID, ts string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT file_id FROM message_files WHERE channel_id=? AND message_ts=?`, channelID, ts)
	if err != nil {
		return err
	}
	var candidates []string
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

	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE channel_id=? AND ts=?`, channelID, ts); err != nil {
		return fmt.Errorf("delete message %s/%s: %w", channelID, ts, err)
	}

	orphanURIs, err := gcOrphans(ctx, tx, candidates)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.deleteBlobs(ctx, orphanURIs)
}

// gcOrphans deletes file rows among `candidates` that now have zero edges and
// returns their storage URIs. No-op for an empty candidate set.
func gcOrphans(ctx context.Context, tx *sql.Tx, candidates []string) ([]string, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	ph, args := inPlaceholders(candidates)
	q := `SELECT id, coalesce(storage_uri,'') FROM files
	      WHERE id IN (` + ph + `)
	        AND NOT EXISTS (SELECT 1 FROM message_files mf WHERE mf.file_id = files.id)`
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var orphanIDs, uris []string
	for rows.Next() {
		var id, uri string
		if err := rows.Scan(&id, &uri); err != nil {
			rows.Close()
			return nil, err
		}
		orphanIDs = append(orphanIDs, id)
		if uri != "" {
			uris = append(uris, uri)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(orphanIDs) > 0 {
		ph2, args2 := inPlaceholders(orphanIDs)
		if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE id IN (`+ph2+`)`, args2...); err != nil {
			return nil, err
		}
	}
	return uris, nil
}

func (s *Store) deleteBlobs(ctx context.Context, uris []string) error {
	if s.blobs == nil {
		return nil
	}
	for _, uri := range uris {
		if err := s.blobs.Delete(ctx, uri); err != nil {
			return fmt.Errorf("gc blob %s: %w", uri, err)
		}
	}
	return nil
}
