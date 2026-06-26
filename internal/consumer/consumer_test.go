package consumer

import (
	"context"
	"testing"

	"github.com/kjkondratuk/slack-mirror/internal/dispatch"
	"github.com/slack-go/slack/slackevents"
)

func TestHandleMessageEventUpserts(t *testing.T) {
	ctx := context.Background()
	st := &recordingStore{}
	rs := &recordingResolver{}
	c := &Consumer{store: st, resolver: rs, filter: dispatch.Filter{}}

	ev := &slackevents.MessageEvent{Channel: "C1", User: "U1", Text: "hi", TimeStamp: "1700000000.000100"}
	if err := c.handleMessage(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if len(st.upserts) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(st.upserts))
	}
}
