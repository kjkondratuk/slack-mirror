// Package backend selects and builds the configured storage backend (postgres or
// sqlite) behind a single interface the wiring uses everywhere.
package backend

import (
	"context"
	"fmt"

	"github.com/kjkondratuk/slack-mirror/internal/blobstore"
	"github.com/kjkondratuk/slack-mirror/internal/config"
	"github.com/kjkondratuk/slack-mirror/internal/consumer"
	"github.com/kjkondratuk/slack-mirror/internal/dbconn"
	"github.com/kjkondratuk/slack-mirror/internal/files"
	"github.com/kjkondratuk/slack-mirror/internal/resolver"
	"github.com/kjkondratuk/slack-mirror/internal/sqlitestore"
	"github.com/kjkondratuk/slack-mirror/internal/store"
)

// Backend is the union of write surfaces the app needs; both *store.PgStore and
// *sqlitestore.Store satisfy it.
type Backend interface {
	consumer.Writer    // UpsertMessage, DeleteMessage
	resolver.MetaStore // UpsertChannel, UpsertUser
	files.FileStore    // UpsertFile, LinkFile, SetFileStored, SetFileState, ReconcileMessageFiles
	Close()
}

// compile-time guards
var (
	_ Backend = (*store.PgStore)(nil)
	_ Backend = (*sqlitestore.Store)(nil)
)

// Select builds the configured backend. blobs may be nil (file mirroring off).
// The returned cleanup closes the backend and any connection resources.
func Select(ctx context.Context, cfg *config.Config, blobs blobstore.Blobstore) (Backend, func(), error) {
	if cfg.StoreBackend == "sqlite" {
		s, err := sqlitestore.NewWithBlobs(cfg.SQLitePath, blobs)
		if err != nil {
			return nil, nil, fmt.Errorf("sqlite backend: %w", err)
		}
		return s, s.Close, nil
	}
	// postgres (default)
	pool, dialerCleanup, err := dbconn.New(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	if err := store.Migrate(ctx, pool); err != nil {
		dialerCleanup()
		pool.Close()
		return nil, nil, err
	}
	pg := store.NewWithBlobs(pool, blobs)
	return pg, func() { pg.Close(); dialerCleanup() }, nil
}
