package resolver

import (
	"context"
	"testing"

	"github.com/kjkondratuk/slack-mirror/internal/model"
	"github.com/slack-go/slack"
)

type fakeStore struct {
	channels map[string]model.Channel
	users    map[string]model.User
}

func newFakeStore() *fakeStore {
	return &fakeStore{channels: map[string]model.Channel{}, users: map[string]model.User{}}
}
func (f *fakeStore) UpsertChannel(_ context.Context, c model.Channel) error {
	f.channels[c.ID] = c
	return nil
}
func (f *fakeStore) UpsertUser(_ context.Context, u model.User) error {
	f.users[u.ID] = u
	return nil
}

type fakeSlack struct {
	convCalls int
	userCalls int
}

func (f *fakeSlack) GetConversationInfoContext(_ context.Context, in *slack.GetConversationInfoInput) (*slack.Channel, error) {
	f.convCalls++
	ch := &slack.Channel{}
	ch.ID = in.ChannelID
	ch.Name = "general"
	ch.IsPrivate = false
	return ch, nil
}
func (f *fakeSlack) GetUserInfoContext(_ context.Context, user string) (*slack.User, error) {
	f.userCalls++
	return &slack.User{ID: user, Name: "kev", RealName: "Kevin", IsBot: false}, nil
}

func TestResolverCachesChannel(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	sl := &fakeSlack{}
	r := New(sl, st)

	if err := r.EnsureChannel(ctx, "C1"); err != nil {
		t.Fatal(err)
	}
	if err := r.EnsureChannel(ctx, "C1"); err != nil {
		t.Fatal(err)
	}
	if sl.convCalls != 1 {
		t.Fatalf("expected 1 conversations.info call, got %d", sl.convCalls)
	}
	if _, ok := st.channels["C1"]; !ok {
		t.Fatal("channel not persisted")
	}
}

type flakyStore struct {
	failChannel  bool
	channelCalls int
}

func (f *flakyStore) UpsertChannel(_ context.Context, c model.Channel) error {
	f.channelCalls++
	if f.failChannel {
		return errContext("boom")
	}
	return nil
}
func (f *flakyStore) UpsertUser(_ context.Context, u model.User) error { return nil }

type errContext string

func (e errContext) Error() string { return string(e) }

func TestResolverRetriesAfterUpsertFailure(t *testing.T) {
	ctx := context.Background()
	sl := &fakeSlack{}
	st := &flakyStore{failChannel: true}
	r := New(sl, st)

	// First call: upsert fails -> EnsureChannel returns error, id NOT marked seen.
	if err := r.EnsureChannel(ctx, "C1"); err == nil {
		t.Fatal("expected error from failing upsert")
	}
	// Recover: upsert now succeeds. Because the id was not marked seen, the resolver
	// must retry (call conversations.info again + upsert again).
	st.failChannel = false
	if err := r.EnsureChannel(ctx, "C1"); err != nil {
		t.Fatal(err)
	}
	if sl.convCalls != 2 {
		t.Fatalf("expected a retry (2 conversations.info calls), got %d", sl.convCalls)
	}
	if st.channelCalls != 2 {
		t.Fatalf("expected 2 upsert attempts, got %d", st.channelCalls)
	}
	// Now it's cached: a third call makes no further API/store calls.
	if err := r.EnsureChannel(ctx, "C1"); err != nil {
		t.Fatal(err)
	}
	if sl.convCalls != 2 {
		t.Fatalf("expected no further API call after success, got %d", sl.convCalls)
	}
}

func TestResolverCachesUser(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	sl := &fakeSlack{}
	r := New(sl, st)

	if err := r.EnsureUser(ctx, "U1"); err != nil {
		t.Fatal(err)
	}
	if err := r.EnsureUser(ctx, "U1"); err != nil {
		t.Fatal(err)
	}
	if sl.userCalls != 1 {
		t.Fatalf("expected 1 users.info call, got %d", sl.userCalls)
	}
	if err := r.EnsureUser(ctx, ""); err != nil {
		t.Fatalf("empty user id should be a no-op, got %v", err)
	}
	if sl.userCalls != 1 {
		t.Fatalf("empty user id should not call API, got %d", sl.userCalls)
	}
}
