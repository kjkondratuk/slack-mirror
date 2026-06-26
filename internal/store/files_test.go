package store

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/kjkondratuk/slack-mirror/internal/model"
)

type fakeBlobs struct{ deleted []string }

func (b *fakeBlobs) Put(_ context.Context, _ string, _ io.Reader, _ string) (string, error) {
	return "", nil
}
func (b *fakeBlobs) Delete(_ context.Context, uri string) error {
	b.deleted = append(b.deleted, uri)
	return nil
}

func TestDeleteMessageGCsOrphanFile(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	blobs := &fakeBlobs{}
	s := NewWithBlobs(pool, blobs)

	if err := s.UpsertChannel(ctx, model.Channel{ID: "C1"}); err != nil {
		t.Fatal(err)
	}
	row := model.MessageRow{ChannelID: "C1", TS: "1700000000.000100", Raw: json.RawMessage(`{}`), PostedAt: time.Unix(1, 0)}
	if err := s.UpsertMessage(ctx, row); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertFile(ctx, model.FileRow{FileRef: model.FileRef{ID: "F1", Raw: json.RawMessage(`{}`)}, DownloadState: "stored"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFileStored(ctx, "F1", "mem://files/F1", "abc"); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkFile(ctx, "C1", "1700000000.000100", "F1"); err != nil {
		t.Fatal(err)
	}

	// Deleting the only referencing message must GC the orphaned file + blob.
	if err := s.DeleteMessage(ctx, "C1", "1700000000.000100"); err != nil {
		t.Fatal(err)
	}
	var nFiles int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM files`).Scan(&nFiles); err != nil {
		t.Fatal(err)
	}
	if nFiles != 0 {
		t.Fatalf("expected orphan file GC'd, found %d", nFiles)
	}
	if len(blobs.deleted) != 1 || blobs.deleted[0] != "mem://files/F1" {
		t.Fatalf("blob not deleted: %v", blobs.deleted)
	}
}
