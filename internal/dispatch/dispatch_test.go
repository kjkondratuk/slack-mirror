package dispatch

import (
	"testing"
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
