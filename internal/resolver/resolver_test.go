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
