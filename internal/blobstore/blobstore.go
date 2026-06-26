// Package blobstore stores file bytes behind a pluggable backend (local FS or
// GCS). URIs are opaque to callers: file://… for local, gs://… for GCS.
package blobstore

import (
	"context"
	"fmt"
	"io"

	"github.com/kjkondratuk/slack-mirror/internal/config"
)

type Blobstore interface {
	Put(ctx context.Context, key string, r io.Reader, contentType string) (uri string, err error)
	Delete(ctx context.Context, uri string) error
}

// New builds the configured backend. Returns nil when files are disabled.
func New(ctx context.Context, c *config.Config) (Blobstore, error) {
	switch c.FileStorage {
	case "", "none":
		return nil, nil
	case "local":
		if c.FileDir == "" {
			return nil, fmt.Errorf("FILE_STORAGE=local requires FILE_DIR")
		}
		return NewLocal(c.FileDir), nil
	case "gcs":
		if c.FileBucket == "" {
			return nil, fmt.Errorf("FILE_STORAGE=gcs requires FILE_BUCKET")
		}
		return NewGCS(ctx, c.FileBucket)
	default:
		return nil, fmt.Errorf("unknown FILE_STORAGE %q", c.FileStorage)
	}
}
