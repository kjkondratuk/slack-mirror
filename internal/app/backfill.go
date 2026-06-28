package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/kjkondratuk/slack-mirror/internal/backend"
	"github.com/kjkondratuk/slack-mirror/internal/backfill"
	"github.com/kjkondratuk/slack-mirror/internal/blobstore"
	"github.com/kjkondratuk/slack-mirror/internal/config"
	"github.com/kjkondratuk/slack-mirror/internal/dispatch"
	"github.com/kjkondratuk/slack-mirror/internal/files"
	"github.com/kjkondratuk/slack-mirror/internal/resolver"
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

	api := slack.New(cfg.SlackBotToken)
	res := resolver.New(api, st)

	filter := dispatch.Filter{
		Allow:   toSet(cfg.ChannelAllowlist),
		Deny:    cfg.ChannelDenylist,
		Persist: cfg.PersistSubtypes,
		Skip:    cfg.SkipSubtypes,
	}

	var b *backfill.Backfiller
	if cfg.FilesEnabled() {
		dl := &files.Downloader{HTTP: http.DefaultClient, Token: cfg.SlackBotToken,
			Blobs: blobs, Store: st, MaxBytes: cfg.FileMaxBytes, MimeAllow: cfg.FileMimeAllowlist}
		b = backfill.NewWithFiles(api, st, res, filter, dl)
	} else {
		b = backfill.New(api, st, res, filter)
	}

	for _, ch := range cfg.ChannelAllowlist {
		log.Info("backfill channel", "channel", ch, "days", cfg.BackfillDays)
		if err := b.Channel(ctx, ch, cfg.BackfillDays); err != nil {
			return err
		}
	}
	log.Info("backfill complete", "channels", len(cfg.ChannelAllowlist))
	return nil
}
