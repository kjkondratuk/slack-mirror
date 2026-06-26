package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/kjkondratuk/slack-mirror/internal/app"
	"github.com/kjkondratuk/slack-mirror/internal/config"
)

type command int

const (
	cmdNone command = iota
	cmdServe
	cmdBackfill
)

var errUsage = errors.New("usage: slack-mirror <serve|backfill>")

func parseCommand(args []string) (command, error) {
	if len(args) == 0 {
		return cmdNone, errUsage
	}
	switch args[0] {
	case "serve":
		return cmdServe, nil
	case "backfill":
		return cmdBackfill, nil
	default:
		return cmdNone, fmt.Errorf("unknown command %q: %w", args[0], errUsage)
	}
}

func main() {
	cmd, err := parseCommand(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: levelFromString(cfg.LogLevel)}))
	ctx := context.Background()

	switch cmd {
	case cmdServe:
		if err := app.Serve(ctx, cfg, log); err != nil {
			log.Error("serve failed", "err", err)
			os.Exit(1)
		}
	case cmdBackfill:
		if err := app.Backfill(ctx, cfg, log); err != nil {
			log.Error("backfill failed", "err", err)
			os.Exit(1)
		}
	}
}

func levelFromString(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
