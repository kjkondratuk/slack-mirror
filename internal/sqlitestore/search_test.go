package sqlitestore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kjkondratuk/slack-mirror/internal/model"
)

func TestSqliteSearch(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if err := s.UpsertChannel(ctx, model.Channel{ID: "C1"}); err != nil {
		t.Fatal(err)
	}
	put := func(ts, text string) {
		if err := s.UpsertMessage(ctx, model.MessageRow{ChannelID: "C1", TS: ts, Text: text,
			Raw: json.RawMessage(`{}`), PostedAt: time.Unix(1, 0)}); err != nil {
			t.Fatal(err)
		}
	}
	put("1.0", "the deploy rollback failed")
	put("2.0", "lunch plans for friday")

	hits, err := s.Search(ctx, "rollback", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].TS != "1.0" {
		t.Fatalf("search hits = %+v", hits)
	}
	// edit removes the term → no hit (triggers keep FTS in sync)
	put("1.0", "all clear now")
	hits, _ = s.Search(ctx, "rollback", 10)
	if len(hits) != 0 {
		t.Fatalf("after edit, expected 0 hits, got %d", len(hits))
	}
}
