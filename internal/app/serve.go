package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/kjkondratuk/slack-mirror/internal/backend"
	"github.com/kjkondratuk/slack-mirror/internal/blobstore"
	"github.com/kjkondratuk/slack-mirror/internal/config"
	"github.com/kjkondratuk/slack-mirror/internal/consumer"
	"github.com/kjkondratuk/slack-mirror/internal/dispatch"
	"github.com/kjkondratuk/slack-mirror/internal/files"
	"github.com/kjkondratuk/slack-mirror/internal/health"
	"github.com/kjkondratuk/slack-mirror/internal/resolver"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

// Serve connects the database, applies migrations, and runs the Socket Mode
// consumer until SIGTERM/SIGINT, then drains and exits.
func Serve(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	if err := cfg.ValidateServe(); err != nil {
		return err
	}

	var blobs blobstore.Blobstore
	if cfg.FilesEnabled() {
		b, err := blobstore.New(ctx, cfg)
		if err != nil {
			return err
		}
		blobs = b
	}

	st, cleanup, err := backend.Select(ctx, cfg, blobs)
	if err != nil {
		return err
	}
	defer cleanup()

	api := slack.New(cfg.SlackBotToken, slack.OptionAppLevelToken(cfg.SlackAppToken))
	sm := socketmode.New(api)
	res := resolver.New(api, st)

	var downloader consumer.FileHandler
	if cfg.FilesEnabled() {
		downloader = &files.Downloader{
			HTTP: http.DefaultClient, Token: cfg.SlackBotToken,
			Blobs: blobs, Store: st, MaxBytes: cfg.FileMaxBytes, MimeAllow: cfg.FileMimeAllowlist,
		}
	}

	filter := dispatch.Filter{
		Allow:   toSet(cfg.ChannelAllowlist),
		Deny:    cfg.ChannelDenylist,
		Persist: cfg.PersistSubtypes,
		Skip:    cfg.SkipSubtypes,
	}
	healthState := &health.State{}
	c := consumer.NewConsumer(sm, st, res, filter, log, healthState, downloader)

	// Health/metrics listener — meaningful in the Cloud Run service fallback,
	// harmless in the worker-pool deployment.
	go func() {
		if err := http.ListenAndServe(":"+cfg.Port, health.Handler(healthState, 10*time.Minute)); err != nil {
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
