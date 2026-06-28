package sqlitestore

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/kjkondratuk/slack-mirror/internal/model"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestSqliteUpsertAndDelete(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if err := s.UpsertChannel(ctx, model.Channel{ID: "C1", Name: "general"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUser(ctx, model.User{ID: "U1", Username: "kev"}); err != nil {
		t.Fatal(err)
	}
	row := model.MessageRow{ChannelID: "C1", TS: "100.1", UserID: "U1", Text: "hello",
		Raw: json.RawMessage(`{"text":"hello"}`), PostedAt: time.Unix(1700000000, 0)}
	if err := s.UpsertMessage(ctx, row); err != nil {
		t.Fatal(err)
	}
	row.Text = "edited"
	if err := s.UpsertMessage(ctx, row); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM messages`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("re-upsert rows=%d want 1", n)
	}
	var txt string
	if err := s.db.QueryRowContext(ctx, `SELECT text FROM messages WHERE channel_id='C1' AND ts='100.1'`).Scan(&txt); err != nil {
		t.Fatal(err)
	}
	if txt != "edited" {
		t.Fatalf("text=%q", txt)
	}
	if err := s.DeleteMessage(ctx, "C1", "100.1"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMessage(ctx, "C1", "100.1"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM messages`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("after delete rows=%d want 0", n)
	}
}
