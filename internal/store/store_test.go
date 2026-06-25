package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kjkondratuk/slack-mirror/internal/model"
)

func newStore(t *testing.T) *PgStore {
	t.Helper()
	ctx := context.Background()
	pool := testPool(t) // skips when TEST_DATABASE_URL unset
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return New(pool)
}

func TestUpsertAndDeleteMessage(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	if err := s.UpsertChannel(ctx, model.Channel{ID: "C1", Name: "general", IsPrivate: false}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUser(ctx, model.User{ID: "U1", Username: "kev", RealName: "Kevin"}); err != nil {
		t.Fatal(err)
	}

	row := model.MessageRow{
		ChannelID: "C1", TS: "1700000000.000100", UserID: "U1",
		Text: "hello world", Raw: json.RawMessage(`{"text":"hello world"}`),
		PostedAt: time.Unix(1700000000, 0),
	}
	if err := s.UpsertMessage(ctx, row); err != nil {
		t.Fatal(err)
	}

	edited := time.Unix(1700000050, 0)
	row.Text = "hello edited"
	row.Raw = json.RawMessage(`{"text":"hello edited"}`)
	row.EditedAt = &edited
	if err := s.UpsertMessage(ctx, row); err != nil {
		t.Fatal(err)
	}

	var text string
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM messages`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row after re-upsert, got %d", count)
	}
	if err := s.pool.QueryRow(ctx, `SELECT text FROM messages WHERE channel_id=$1 AND ts=$2`,
		"C1", "1700000000.000100").Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != "hello edited" {
		t.Fatalf("text = %q, want edited", text)
	}

	if err := s.DeleteMessage(ctx, "C1", "1700000000.000100"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMessage(ctx, "C1", "1700000000.000100"); err != nil {
		t.Fatalf("second delete errored: %v", err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM messages`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", count)
	}
}
