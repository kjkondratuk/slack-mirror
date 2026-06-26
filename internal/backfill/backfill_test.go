package backfill

import (
	"context"
	"testing"

	"github.com/kjkondratuk/slack-mirror/internal/model"
	"github.com/slack-go/slack"
)

type fakeHistory struct {
	pages   []slack.GetConversationHistoryResponse
	replies map[string][]slack.Message
	pageIdx int
}

func (f *fakeHistory) GetConversationHistoryContext(_ context.Context, _ *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error) {
	resp := f.pages[f.pageIdx]
	f.pageIdx++
	return &resp, nil
}
func (f *fakeHistory) GetConversationRepliesContext(_ context.Context, p *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
	return f.replies[p.Timestamp], false, "", nil
}

type capStore struct{ rows []model.MessageRow }

func (c *capStore) UpsertMessage(_ context.Context, m model.MessageRow) error {
	c.rows = append(c.rows, m)
	return nil
}
func (c *capStore) DeleteMessage(_ context.Context, _, _ string) error    { return nil }
func (c *capStore) UpsertChannel(_ context.Context, _ model.Channel) error { return nil }
func (c *capStore) UpsertUser(_ context.Context, _ model.User) error      { return nil }
func (c *capStore) Close()                                                 {}

type noopResolver struct{}

func (noopResolver) EnsureChannel(context.Context, string) error { return nil }
func (noopResolver) EnsureUser(context.Context, string) error    { return nil }

func msg(ts, text, threadTS string, reply int) slack.Message {
	var m slack.Message
	m.Timestamp = ts
	m.Text = text
	m.ThreadTimestamp = threadTS
	m.ReplyCount = reply
	m.User = "U1"
	return m
}

func TestBackfillPagesHistoryAndReplies(t *testing.T) {
	ctx := context.Background()
	fh := &fakeHistory{
		pages: []slack.GetConversationHistoryResponse{
			{Messages: []slack.Message{
				msg("1700000000.000100", "parent", "", 2),
				msg("1700000001.000100", "standalone", "", 0),
			}, HasMore: false},
		},
		replies: map[string][]slack.Message{
			"1700000000.000100": {
				msg("1700000000.000100", "parent", "1700000000.000100", 2),
				msg("1700000000.000200", "reply", "1700000000.000100", 0),
			},
		},
	}
	st := &capStore{}
	b := New(fh, st, noopResolver{})

	if err := b.Channel(ctx, "C1", 90); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range st.rows {
		seen[r.TS] = true
		if r.ChannelID != "C1" {
			t.Fatalf("wrong channel %q", r.ChannelID)
		}
	}
	for _, ts := range []string{"1700000000.000100", "1700000001.000100", "1700000000.000200"} {
		if !seen[ts] {
			t.Fatalf("missing ts %s in upserts: %v", ts, seen)
		}
	}
}
