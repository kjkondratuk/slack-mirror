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

func TestReconcileMessageFilesGCsRemovedFile(t *testing.T) {
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
	row := model.MessageRow{ChannelID: "C1", TS: "100.1", Raw: json.RawMessage(`{}`), PostedAt: time.Unix(1, 0)}
	if err := s.UpsertMessage(ctx, row); err != nil {
		t.Fatal(err)
	}
	// Two files attached, both stored.
	for _, id := range []string{"F1", "F2"} {
		if err := s.UpsertFile(ctx, model.FileRow{FileRef: model.FileRef{ID: id, Raw: json.RawMessage(`{}`)}, DownloadState: "stored"}); err != nil {
			t.Fatal(err)
		}
		if err := s.SetFileStored(ctx, id, "mem://files/"+id, "h"); err != nil {
			t.Fatal(err)
		}
		if err := s.LinkFile(ctx, "C1", "100.1", id); err != nil {
			t.Fatal(err)
		}
	}
	// Edit drops F2 (keep only F1). F2 must be GC'd (row + blob); F1 stays.
	if err := s.ReconcileMessageFiles(ctx, "C1", "100.1", []string{"F1"}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM files WHERE id='F2'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("F2 should be GC'd, found %d", n)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM files WHERE id='F1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("F1 should remain, found %d", n)
	}
	if len(blobs.deleted) != 1 || blobs.deleted[0] != "mem://files/F2" {
		t.Fatalf("expected F2 blob deleted, got %v", blobs.deleted)
	}
}

func TestReconcileKeepsSharedFile(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	s := NewWithBlobs(pool, &fakeBlobs{})
	if err := s.UpsertChannel(ctx, model.Channel{ID: "C1"}); err != nil {
		t.Fatal(err)
	}
	for _, ts := range []string{"100.1", "200.1"} {
		if err := s.UpsertMessage(ctx, model.MessageRow{ChannelID: "C1", TS: ts, Raw: json.RawMessage(`{}`), PostedAt: time.Unix(1, 0)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.UpsertFile(ctx, model.FileRow{FileRef: model.FileRef{ID: "F1", Raw: json.RawMessage(`{}`)}, DownloadState: "stored"}); err != nil {
		t.Fatal(err)
	}
	// F1 shared by both messages.
	if err := s.LinkFile(ctx, "C1", "100.1", "F1"); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkFile(ctx, "C1", "200.1", "F1"); err != nil {
		t.Fatal(err)
	}
	// Edit msg 100.1 to drop F1 — but F1 still referenced by 200.1, so it must survive.
	if err := s.ReconcileMessageFiles(ctx, "C1", "100.1", nil); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM files WHERE id='F1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("shared F1 must survive, found %d", n)
	}
}
