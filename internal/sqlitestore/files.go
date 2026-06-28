package sqlitestore

import (
	"context"
	"fmt"

	"github.com/kjkondratuk/slack-mirror/internal/model"
)

func (s *Store) UpsertFile(ctx context.Context, r model.FileRow) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO files (id, name, title, mimetype, filetype, size, user_id, mode, is_external,
		                   storage_uri, sha256, download_state, raw, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, title=excluded.title, mimetype=excluded.mimetype,
			filetype=excluded.filetype, size=excluded.size, user_id=excluded.user_id, mode=excluded.mode,
			is_external=excluded.is_external, raw=excluded.raw`,
		r.ID, r.Name, r.Title, r.Mimetype, r.Filetype, r.Size, nullStr(r.UserID), r.Mode, r.IsExternal,
		nullStr(r.StorageURI), nullStr(r.SHA256), r.DownloadState, string(r.Raw))
	if err != nil {
		return fmt.Errorf("upsert file %s: %w", r.ID, err)
	}
	return nil
}

func (s *Store) LinkFile(ctx context.Context, channelID, messageTS, fileID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO message_files (channel_id, message_ts, file_id) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
		channelID, messageTS, fileID)
	if err != nil {
		return fmt.Errorf("link file %s: %w", fileID, err)
	}
	return nil
}

func (s *Store) SetFileStored(ctx context.Context, id, storageURI, sha string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE files SET storage_uri=?, sha256=?, download_state='stored',
		 downloaded_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, storageURI, sha, id)
	if err != nil {
		return fmt.Errorf("set file stored %s: %w", id, err)
	}
	return nil
}

func (s *Store) SetFileState(ctx context.Context, id, state string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE files SET download_state=? WHERE id=?`, state, id)
	if err != nil {
		return fmt.Errorf("set file state %s: %w", id, err)
	}
	return nil
}

// ReconcileMessageFiles makes the message's edges match keepFileIDs and GCs any
// orphaned files. Empty keep set removes all the message's edges. SQLite dialect:
// builds NOT IN(…) with a guard for the empty case (empty IN() is a syntax error).
func (s *Store) ReconcileMessageFiles(ctx context.Context, channelID, messageTS string, keepFileIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	where := `channel_id=? AND message_ts=?`
	args := []any{channelID, messageTS}
	if len(keepFileIDs) > 0 {
		ph, kargs := inPlaceholders(keepFileIDs)
		where += ` AND file_id NOT IN (` + ph + `)`
		args = append(args, kargs...)
	}

	rows, err := tx.QueryContext(ctx, `SELECT file_id FROM message_files WHERE `+where, args...)
	if err != nil {
		return err
	}
	var removed []string
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
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM message_files WHERE `+where, args...); err != nil {
		return err
	}
	orphanURIs, err := gcOrphans(ctx, tx, removed)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.deleteBlobs(ctx, orphanURIs)
}
