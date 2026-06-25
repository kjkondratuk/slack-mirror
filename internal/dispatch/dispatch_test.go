package dispatch

import (
	"testing"

	"github.com/kjkondratuk/slack-mirror/internal/model"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

func TestTSToTime(t *testing.T) {
	got, err := tsToTime("1700000000.000200")
	if err != nil {
		t.Fatal(err)
	}
	if got.Unix() != 1700000000 {
		t.Fatalf("Unix = %d, want 1700000000", got.Unix())
	}
	if got.Nanosecond() != 200000 { // .000200 seconds = 200 microseconds
		t.Fatalf("Nanosecond = %d, want 200000", got.Nanosecond())
	}
	if _, err := tsToTime("not-a-ts"); err == nil {
		t.Fatal("expected error for malformed ts")
	}
}

func TestFilterChannelAllowed(t *testing.T) {
	f := Filter{Deny: map[string]bool{"C9": true}}
	if !f.ChannelAllowed("C1") {
		t.Error("empty allowlist should allow C1")
	}
	if f.ChannelAllowed("C9") {
		t.Error("denylist should block C9")
	}
	f = Filter{Allow: map[string]bool{"C1": true}}
	if !f.ChannelAllowed("C1") || f.ChannelAllowed("C2") {
		t.Error("allowlist gate wrong")
	}
}

func TestFilterSubtypePersisted(t *testing.T) {
	f := Filter{Skip: map[string]bool{"channel_join": true}}
	if f.SubtypePersisted("channel_join") {
		t.Error("channel_join should be skipped")
	}
	if !f.SubtypePersisted("") {
		t.Error("normal message (empty subtype) should persist")
	}
	if !f.SubtypePersisted("bot_message") {
		t.Error("bot_message should persist by default")
	}
	f = Filter{Persist: map[string]bool{"bot_message": true}}
	if !f.SubtypePersisted("bot_message") || f.SubtypePersisted("file_share") {
		t.Error("persist-allowlist gate wrong")
	}
	if !f.SubtypePersisted("") {
		t.Error("normal message should persist even in persist-allowlist mode")
	}
}

func TestMessageToRow(t *testing.T) {
	m := &slack.Msg{
		Channel:         "C1",
		User:            "U1",
		Text:            "hi there",
		Timestamp:       "1700000000.000100",
		ThreadTimestamp: "1699999999.000000",
	}
	row, err := messageToRow("C1", m)
	if err != nil {
		t.Fatal(err)
	}
	if row.ChannelID != "C1" || row.TS != "1700000000.000100" || row.UserID != "U1" {
		t.Fatalf("bad row identity: %+v", row)
	}
	if row.ThreadTS != "1699999999.000000" {
		t.Fatalf("thread_ts = %q", row.ThreadTS)
	}
	if row.Text != "hi there" {
		t.Fatalf("text = %q", row.Text)
	}
	if row.PostedAt.Unix() != 1700000000 {
		t.Fatalf("posted_at = %v", row.PostedAt)
	}
	if len(row.Raw) == 0 || row.Raw[0] != '{' {
		t.Fatalf("raw should be JSON object, got %q", row.Raw)
	}
}

func TestMessageToRowEdited(t *testing.T) {
	m := &slack.Msg{
		User:      "U1",
		Text:      "edited",
		Timestamp: "1700000000.000100",
		Edited:    &slack.Edited{User: "U1", Timestamp: "1700000060.000000"},
	}
	row, err := messageToRow("C1", m)
	if err != nil {
		t.Fatal(err)
	}
	if row.EditedAt == nil || row.EditedAt.Unix() != 1700000060 {
		t.Fatalf("edited_at = %v", row.EditedAt)
	}
}

func TestMessageToRowBotFallback(t *testing.T) {
	m := &slack.Msg{SubType: "bot_message", BotID: "B1", Text: "beep", Timestamp: "1700000000.000100"}
	row, err := messageToRow("C1", m)
	if err != nil {
		t.Fatal(err)
	}
	if row.UserID != "B1" {
		t.Fatalf("expected bot id fallback, got %q", row.UserID)
	}
}

func defFilter() Filter {
	return Filter{Skip: map[string]bool{"channel_join": true}}
}

func TestDispatchNormalMessage(t *testing.T) {
	ev := &slackevents.MessageEvent{Channel: "C1", User: "U1", Text: "hello", TimeStamp: "1700000000.000100"}
	act, err := Dispatch("C1", ev, defFilter())
	if err != nil {
		t.Fatal(err)
	}
	if act.Kind != model.ActionUpsert || act.TS != "1700000000.000100" || act.Message == nil {
		t.Fatalf("got %+v", act)
	}
}

func TestDispatchSkippedSubtype(t *testing.T) {
	ev := &slackevents.MessageEvent{Channel: "C1", SubType: "channel_join", TimeStamp: "1700000000.000100"}
	act, err := Dispatch("C1", ev, defFilter())
	if err != nil {
		t.Fatal(err)
	}
	if act.Kind != model.ActionSkip {
		t.Fatalf("channel_join should skip, got %v", act.Kind)
	}
}

func TestDispatchChannelNotAllowed(t *testing.T) {
	ev := &slackevents.MessageEvent{Channel: "C2", Text: "x", TimeStamp: "1700000000.000100"}
	f := Filter{Allow: map[string]bool{"C1": true}}
	act, err := Dispatch("C2", ev, f)
	if err != nil {
		t.Fatal(err)
	}
	if act.Kind != model.ActionSkip {
		t.Fatalf("non-allowlisted channel should skip, got %v", act.Kind)
	}
}

func TestDispatchMessageChanged(t *testing.T) {
	// message_changed carries the original message nested under .Message (*slack.Msg);
	// the identifying ts is the INNER message ts, not the event ts.
	ev := &slackevents.MessageEvent{
		Channel:        "C1",
		SubType:        "message_changed",
		TimeStamp:      "1700000999.000999", // event ts — must NOT be used as key
		EventTimeStamp: "1700000999.000999",
		Message: &slack.Msg{
			User:      "U1",
			Text:      "after edit",
			Timestamp: "1700000000.000100", // original ts — the key
			Edited:    &slack.Edited{Timestamp: "1700000999.000999"},
		},
	}
	act, err := Dispatch("C1", ev, defFilter())
	if err != nil {
		t.Fatal(err)
	}
	if act.Kind != model.ActionUpsert {
		t.Fatalf("changed should upsert, got %v", act.Kind)
	}
	if act.TS != "1700000000.000100" {
		t.Fatalf("changed must key on inner ts, got %q", act.TS)
	}
	if act.Message == nil || act.Message.Text != "after edit" {
		t.Fatalf("changed row wrong: %+v", act.Message)
	}
	if act.Message.EditedAt == nil {
		t.Fatalf("changed row should have edited_at set")
	}
}

func TestDispatchMessageDeleted(t *testing.T) {
	ev := &slackevents.MessageEvent{
		Channel:          "C1",
		SubType:          "message_deleted",
		TimeStamp:        "1700000999.000999",
		DeletedTimeStamp: "1700000000.000100",
	}
	act, err := Dispatch("C1", ev, defFilter())
	if err != nil {
		t.Fatal(err)
	}
	if act.Kind != model.ActionDelete {
		t.Fatalf("deleted should delete, got %v", act.Kind)
	}
	if act.TS != "1700000000.000100" {
		t.Fatalf("deleted must use deleted_ts, got %q", act.TS)
	}
}
