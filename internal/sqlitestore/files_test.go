package sqlitestore

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/kjkondratuk/slack-mirror/internal/model"
)

type fakeBlobs struct{ deleted []string }

func (b *fakeBlobs) Put(context.Context, string, io.Reader, string) (string, error) { return "", nil }
func (b *fakeBlobs) Delete(_ context.Context, uri string) error {
	b.deleted = append(b.deleted, uri)
	return nil
}

func storeWithBlobs(t *testing.T, b *fakeBlobs) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "f.db"), b)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func seedMsg(t *testing.T, s *Store, ts string) {
	t.Helper()
	ctx := context.Background()
	if err := s.UpsertChannel(ctx, model.Channel{ID: "C1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMessage(ctx, model.MessageRow{ChannelID: "C1", TS: ts, Raw: json.RawMessage(`{}`), PostedAt: time.Unix(1, 0)}); err != nil {
		t.Fatal(err)
	}
}

func mustCount(t *testing.T, s *Store, q string, want int) {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(), q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("%q = %d, want %d", q, n, want)
	}
}

func TestSqliteFileGCOnDelete(t *testing.T) {
	ctx := context.Background()
	b := &fakeBlobs{}
	s := storeWithBlobs(t, b)
	seedMsg(t, s, "100.1")
	if err := s.UpsertFile(ctx, model.FileRow{FileRef: model.FileRef{ID: "F1", Raw: json.RawMessage(`{}`)}, DownloadState: "stored"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFileStored(ctx, "F1", "mem://files/F1", "h"); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkFile(ctx, "C1", "100.1", "F1"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMessage(ctx, "C1", "100.1"); err != nil {
		t.Fatal(err)
	}
	mustCount(t, s, `SELECT count(*) FROM files`, 0)
	if len(b.deleted) != 1 || b.deleted[0] != "mem://files/F1" {
		t.Fatalf("blob not deleted: %v", b.deleted)
	}
}

func TestSqliteReconcileRemovesAndKeepsShared(t *testing.T) {
	ctx := context.Background()
	b := &fakeBlobs{}
	s := storeWithBlobs(t, b)
	seedMsg(t, s, "100.1")
	seedMsg(t, s, "200.1")
	for _, id := range []string{"F1", "F2"} {
		if err := s.UpsertFile(ctx, model.FileRow{FileRef: model.FileRef{ID: id, Raw: json.RawMessage(`{}`)}, DownloadState: "stored"}); err != nil {
			t.Fatal(err)
		}
		if err := s.SetFileStored(ctx, id, "mem://files/"+id, "h"); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"F1", "F2"} {
		if err := s.LinkFile(ctx, "C1", "100.1", id); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.LinkFile(ctx, "C1", "200.1", "F1"); err != nil {
		t.Fatal(err)
	}
	// edit 100.1 to keep only F1 → F2 GC'd; F1 stays (on 100.1 and 200.1)
	if err := s.ReconcileMessageFiles(ctx, "C1", "100.1", []string{"F1"}); err != nil {
		t.Fatal(err)
	}
	mustCount(t, s, "SELECT count(*) FROM files WHERE id='F2'", 0)
	mustCount(t, s, "SELECT count(*) FROM files WHERE id='F1'", 1)
	// edit 100.1 to keep NOTHING → F1 still referenced by 200.1, survives
	if err := s.ReconcileMessageFiles(ctx, "C1", "100.1", nil); err != nil {
		t.Fatal(err)
	}
	mustCount(t, s, "SELECT count(*) FROM files WHERE id='F1'", 1)
}
