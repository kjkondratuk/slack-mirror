// Package resolver lazily resolves Slack channel/user metadata and caches it in
// process, persisting each newly-seen entity via the store. It guarantees the
// channels row exists before a message referencing it is written (FK safety).
package resolver

import (
	"context"
	"sync"

	"github.com/kjkondratuk/slack-mirror/internal/model"
	"github.com/slack-go/slack"
)

// InfoClient is the narrow slice of the Slack Web API the resolver needs.
// *slack.Client satisfies it.
type InfoClient interface {
	GetConversationInfoContext(ctx context.Context, in *slack.GetConversationInfoInput) (*slack.Channel, error)
	GetUserInfoContext(ctx context.Context, user string) (*slack.User, error)
}

// MetaStore is the subset of store.Store the resolver writes to.
type MetaStore interface {
	UpsertChannel(ctx context.Context, c model.Channel) error
	UpsertUser(ctx context.Context, u model.User) error
}

type Resolver struct {
	slack InfoClient
	store MetaStore

	mu       sync.Mutex
	channels map[string]bool
	users    map[string]bool
}

func New(s InfoClient, store MetaStore) *Resolver {
	return &Resolver{
		slack:    s,
		store:    store,
		channels: map[string]bool{},
		users:    map[string]bool{},
	}
}

func (r *Resolver) isSeen(set map[string]bool, id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return set[id]
}

func (r *Resolver) markSeen(set map[string]bool, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set[id] = true
}

// EnsureChannel guarantees the channels row exists. On cache miss it calls
// conversations.info and upserts. On API error it still upserts a stub row so
// the FK constraint holds. The channel is only marked seen after a successful
// upsert so that transient DB failures allow the next call to retry.
func (r *Resolver) EnsureChannel(ctx context.Context, id string) error {
	if id == "" || r.isSeen(r.channels, id) {
		return nil
	}
	c := model.Channel{ID: id}
	if info, err := r.slack.GetConversationInfoContext(ctx, &slack.GetConversationInfoInput{ChannelID: id}); err == nil && info != nil {
		c.Name = info.Name
		c.IsPrivate = info.IsPrivate
	}
	if err := r.store.UpsertChannel(ctx, c); err != nil {
		return err // not marked seen -> retried on the next message
	}
	r.markSeen(r.channels, id)
	return nil
}

// EnsureUser guarantees a users row exists. Empty id is a no-op (system msgs).
// The user is only marked seen after a successful upsert so that transient DB
// failures allow the next call to retry.
func (r *Resolver) EnsureUser(ctx context.Context, id string) error {
	if id == "" || r.isSeen(r.users, id) {
		return nil
	}
	u := model.User{ID: id}
	if info, err := r.slack.GetUserInfoContext(ctx, id); err == nil && info != nil {
		u.Username = info.Name
		u.RealName = info.RealName
		u.IsBot = info.IsBot
	}
	if err := r.store.UpsertUser(ctx, u); err != nil {
		return err
	}
	r.markSeen(r.users, id)
	return nil
}
