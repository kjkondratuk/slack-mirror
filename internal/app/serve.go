package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/kjkondratuk/slack-mirror/internal/blobstore"
	"github.com/kjkondratuk/slack-mirror/internal/config"
	"github.com/kjkondratuk/slack-mirror/internal/consumer"
	"github.com/kjkondratuk/slack-mirror/internal/dbconn"
	"github.com/kjkondratuk/slack-mirror/internal/dispatch"
	"github.com/kjkondratuk/slack-mirror/internal/files"
	"github.com/kjkondratuk/slack-mirror/internal/health"
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
	var st *store.PgStore
	var downloader consumer.FileHandler
	if cfg.FilesEnabled() {
		blobs, err := blobstore.New(ctx, cfg)
		if err != nil {
			return err
		}
		pg := store.NewWithBlobs(pool, blobs)
		st = pg
		downloader = &files.Downloader{
			HTTP: http.DefaultClient, Token: cfg.SlackBotToken,
			Blobs: blobs, Store: pg, MaxBytes: cfg.FileMaxBytes, MimeAllow: cfg.FileMimeAllowlist,
		}
	} else {
		st = store.New(pool)
	}
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

	st2 := &health.State{}
	c := consumer.NewConsumer(sm, st, res, filter, log, st2, downloader)

	// Health/metrics listener — meaningful in the Cloud Run service fallback,
	// harmless in the worker-pool deployment.
	go func() {
		if err := http.ListenAndServe(":"+cfg.Port, health.Handler(st2, 10*time.Minute)); err != nil {
			log.Warn("health listener stopped", "err", err)
		}
	}()

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
