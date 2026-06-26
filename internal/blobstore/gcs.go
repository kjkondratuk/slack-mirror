package blobstore

import (
	"context"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/storage"
)

type GCS struct {
	client *storage.Client
	bucket string
}

func NewGCS(ctx context.Context, bucket string) (*GCS, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("gcs client: %w", err)
	}
	return &GCS{client: client, bucket: bucket}, nil
}

func (g *GCS) Put(ctx context.Context, key string, r io.Reader, contentType string) (string, error) {
	w := g.client.Bucket(g.bucket).Object(key).NewWriter(ctx)
	w.ContentType = contentType
	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close()
		return "", fmt.Errorf("gcs put %s: %w", key, err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("gcs close %s: %w", key, err)
	}
	return fmt.Sprintf("gs://%s/%s", g.bucket, key), nil
}

func (g *GCS) Delete(ctx context.Context, uri string) error {
	rest := strings.TrimPrefix(uri, "gs://")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("bad gs uri %q", uri)
	}
	if err := g.client.Bucket(parts[0]).Object(parts[1]).Delete(ctx); err != nil && err != storage.ErrObjectNotExist {
		return fmt.Errorf("gcs delete %s: %w", uri, err)
	}
	return nil
}
