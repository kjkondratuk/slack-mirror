// Package sqlitestore is an embedded SQLite + FTS5 storage backend implementing
// the same write interfaces as internal/store (the Postgres backend).
package sqlitestore

import (
	"database/sql"
	"fmt"

	"github.com/kjkondratuk/slack-mirror/internal/blobstore"
	_ "modernc.org/sqlite"
)

type Store struct {
	db    *sql.DB
	blobs blobstore.Blobstore // nil when file mirroring disabled
}

// Open opens (creating if needed) the SQLite DB at path, applies migrations, and
// returns a ready Store. blobs may be nil. Caller owns Close().
func Open(path string, blobs blobstore.Blobstore) (*Store, error) {
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
