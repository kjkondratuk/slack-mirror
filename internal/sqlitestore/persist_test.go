package sqlitestore

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/kjkondratuk/slack-mirror/internal/model"
)

func TestSqlitePersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p.db")

	s, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertChannel(ctx, model.Channel{ID: "C1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMessage(ctx, model.MessageRow{ChannelID: "C1", TS: "1.0",
		Text: "persisted rollback", Raw: json.RawMessage(`{}`), PostedAt: time.Unix(1, 0)}); err != nil {
		t.Fatal(err)
	}
	s.Close() // checkpoints the WAL into the main file

	// Re-open the SAME file: migration must be idempotent, data + FTS index intact.
	s2, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	var n int
	if err := s2.db.QueryRowContext(ctx, `SELECT count(*) FROM messages`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 persisted message after reopen, got %d", n)
	}
	hits, err := s2.Search(ctx, "rollback", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].TS != "1.0" {
		t.Fatalf("FTS index lost across reopen: %+v", hits)
	}
}
