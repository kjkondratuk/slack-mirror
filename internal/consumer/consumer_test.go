package consumer

import (
	"context"
	"testing"

	"github.com/kjkondratuk/slack-mirror/internal/dispatch"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

func TestHandleMessageEventUpserts(t *testing.T) {
	ctx := context.Background()
	st := &recordingStore{}
	rs := &recordingResolver{}
	c := &Consumer{store: st, resolver: rs, filter: dispatch.Filter{}}

	ev := &slackevents.MessageEvent{Channel: "C1", User: "U1", Text: "hi", TimeStamp: "1700000000.000100"}
	if err := c.handleMessage(ctx, ev, nil); err != nil {
		t.Fatal(err)
	}
	if len(st.upserts) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(st.upserts))
	}
}

func TestFileMsgParsesRawPayloadForNewMessage(t *testing.T) {
	payload := []byte(`{"event":{"type":"message","ts":"100.1","files":[{"id":"F1","url_private_download":"https://x/a"}]}}`)
	ev := &slackevents.MessageEvent{TimeStamp: "100.1"} // new message (no subtype)
	msg := fileMsg(ev, payload)
	if msg == nil {
		t.Fatal("expected a parsed slack.Msg, got nil")
	}
	if len(msg.Files) != 1 || msg.Files[0].ID != "F1" {
		t.Fatalf("expected F1 in parsed files, got %+v", msg.Files)
	}
}

func TestFileMsgChangedUsesInnerMessage(t *testing.T) {
	inner := &slack.Msg{Timestamp: "100.1", Files: []slack.File{{ID: "F2"}}}
	ev := &slackevents.MessageEvent{SubType: "message_changed", Message: inner}
	if got := fileMsg(ev, nil); got != inner {
		t.Fatalf("message_changed should return the inner ev.Message, got %p want %p", got, inner)
	}
}

func TestFileMsgNilOnBadPayload(t *testing.T) {
	ev := &slackevents.MessageEvent{TimeStamp: "100.1"}
	if msg := fileMsg(ev, []byte("not json")); msg != nil {
		t.Fatalf("expected nil on unparseable payload, got %+v", msg)
	}
}
