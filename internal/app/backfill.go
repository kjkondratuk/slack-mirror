package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kjkondratuk/slack-mirror/internal/backfill"
	"github.com/kjkondratuk/slack-mirror/internal/config"
	"github.com/kjkondratuk/slack-mirror/internal/dbconn"
	"github.com/kjkondratuk/slack-mirror/internal/resolver"
	"github.com/kjkondratuk/slack-mirror/internal/store"
	"github.com/slack-go/slack"
)

// Backfill seeds the allowlisted channels' current history into the mirror, then
// returns. Idempotent against whatever serve has already captured.
func Backfill(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	if err := cfg.ValidateBackfill(); err != nil {
		return err
	}
	if len(cfg.ChannelAllowlist) == 0 {
		return fmt.Errorf("backfill: CHANNEL_ALLOWLIST must list the channels to seed")
	}

	pool, err := dbconn.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := store.Migrate(ctx, pool); err != nil {
		return err
	}
	st := store.New(pool)
	defer st.Close()

	api := slack.New(cfg.SlackBotToken)
	res := resolver.New(api, st)
	b := backfill.New(api, st, res)

	for _, ch := range cfg.ChannelAllowlist {
		log.Info("backfill channel", "channel", ch, "days", cfg.BackfillDays)
		if err := b.Channel(ctx, ch, cfg.BackfillDays); err != nil {
			return err
		}
	}
	log.Info("backfill complete", "channels", len(cfg.ChannelAllowlist))
	return nil
}
