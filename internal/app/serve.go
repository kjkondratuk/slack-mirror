package app

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/kjkondratuk/slack-mirror/internal/config"
	"github.com/kjkondratuk/slack-mirror/internal/consumer"
	"github.com/kjkondratuk/slack-mirror/internal/dbconn"
	"github.com/kjkondratuk/slack-mirror/internal/dispatch"
	"github.com/kjkondratuk/slack-mirror/internal/resolver"
	"github.com/kjkondratuk/slack-mirror/internal/store"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

// Serve connects the database, applies migrations, and runs the Socket Mode
// consumer until SIGTERM/SIGINT, then drains and exits.
func Serve(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	if err := cfg.ValidateServe(); err != nil {
		return err
	}

	pool, err := dbconn.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close() // always close the pool, even if Migrate fails (double-close is safe)
	if err := store.Migrate(ctx, pool); err != nil {
		return err
	}
	st := store.New(pool)
	defer st.Close()

	api := slack.New(cfg.SlackBotToken, slack.OptionAppLevelToken(cfg.SlackAppToken))
	sm := socketmode.New(api)
	res := resolver.New(api, st)

	filter := dispatch.Filter{
		Allow:   toSet(cfg.ChannelAllowlist),
		Deny:    cfg.ChannelDenylist,
		Persist: cfg.PersistSubtypes,
		Skip:    cfg.SkipSubtypes,
	}

	c := consumer.NewConsumer(sm, st, res, filter, log)

	// Graceful shutdown on SIGTERM/SIGINT (Cloud Run sends SIGTERM on replace).
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	log.Info("serve starting")
	if err := c.Run(ctx); err != nil && ctx.Err() == nil {
		return fmt.Errorf("consumer: %w", err)
	}
	log.Info("serve stopped")
	return nil
}

func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, i := range items {
		m[i] = true
	}
	return m
}
