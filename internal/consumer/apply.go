// Package consumer runs the live Socket Mode loop and shares the Apply glue with
// backfill: ensure channel/user metadata exists, then execute the store write.
package consumer

import (
	"context"
	"fmt"

	"github.com/kjkondratuk/slack-mirror/internal/model"
)

// Writer is the store surface Apply needs.
type Writer interface {
	UpsertMessage(ctx context.Context, m model.MessageRow) error
	DeleteMessage(ctx context.Context, channelID, ts string) error
}

// Resolver ensures channel/user rows exist before a message references them.
type Resolver interface {
	EnsureChannel(ctx context.Context, id string) error
	EnsureUser(ctx context.Context, id string) error
}

// Apply executes one Action. For upserts it ensures the channel (FK) and user
// rows first. Idempotent: safe to call repeatedly with the same Action.
func Apply(ctx context.Context, w Writer, r Resolver, act model.Action) error {
	switch act.Kind {
	case model.ActionUpsert:
		if act.Message == nil {
			return fmt.Errorf("upsert action with nil message for %s/%s", act.ChannelID, act.TS)
		}
		if err := r.EnsureChannel(ctx, act.ChannelID); err != nil {
			return err
		}
		if err := r.EnsureUser(ctx, act.Message.UserID); err != nil {
			return err
		}
		return w.UpsertMessage(ctx, *act.Message)
	case model.ActionDelete:
		return w.DeleteMessage(ctx, act.ChannelID, act.TS)
	default:
		return nil // skip
	}
}
