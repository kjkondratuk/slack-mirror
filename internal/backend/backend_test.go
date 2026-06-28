package backend

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kjkondratuk/slack-mirror/internal/config"
	"github.com/kjkondratuk/slack-mirror/internal/model"
)

func TestSelectSqlite(t *testing.T) {
	cfg := &config.Config{StoreBackend: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "b.db")}
	b, cleanup, err := Select(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	// proves it's a live backend satisfying the union (UpsertChannel is from MetaStore)
	if err := b.UpsertChannel(context.Background(), model.Channel{ID: "C1"}); err != nil {
		t.Fatal(err)
	}
}

func TestSelectDefaultsToPostgres(t *testing.T) {
	// No StoreBackend set, no DB target → postgres path is taken and fails to
	// connect (we only assert it did NOT take the sqlite path / did not panic).
	cfg := &config.Config{}
	_, _, err := Select(context.Background(), cfg, nil)
	if err == nil {
		t.Fatal("expected an error building postgres backend with no DB target")
	}
}
