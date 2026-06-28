package config

import (
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("SLACK_APP_TOKEN", "xapp-1")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-1")
	t.Setenv("DATABASE_URL", "postgres://localhost/mirror")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.BackfillDays != 90 {
		t.Errorf("BackfillDays = %d, want 90", c.BackfillDays)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", c.LogLevel)
	}
	if !c.SkipSubtypes["channel_join"] {
		t.Errorf("expected channel_join in default SkipSubtypes")
	}
	if len(c.PersistSubtypes) != 0 {
		t.Errorf("PersistSubtypes should be empty by default, got %v", c.PersistSubtypes)
	}
}

func TestLoadParsesLists(t *testing.T) {
	t.Setenv("SLACK_APP_TOKEN", "xapp-1")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-1")
	t.Setenv("DATABASE_URL", "postgres://localhost/mirror")
	t.Setenv("CHANNEL_ALLOWLIST", "C123, C456 ,C789")
	t.Setenv("CHANNEL_DENYLIST", "C999")
	t.Setenv("SKIP_SUBTYPES", "bot_message")
	t.Setenv("BACKFILL_DAYS", "30")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ChannelAllowlist; len(got) != 3 || got[0] != "C123" || got[1] != "C456" || got[2] != "C789" {
		t.Errorf("ChannelAllowlist = %v", got)
	}
	if !c.ChannelDenylist["C999"] {
		t.Errorf("expected C999 in denylist")
	}
	if c.BackfillDays != 30 {
		t.Errorf("BackfillDays = %d, want 30", c.BackfillDays)
	}
	if c.SkipSubtypes["channel_join"] {
		t.Errorf("explicit SKIP_SUBTYPES should replace defaults; channel_join should be absent")
	}
	if !c.SkipSubtypes["bot_message"] {
		t.Errorf("expected bot_message in SkipSubtypes")
	}
}

func TestValidateServeRequiresTokensAndDB(t *testing.T) {
	c := &Config{}
	if err := c.ValidateServe(); err == nil {
		t.Fatal("expected error for empty serve config")
	}
	c = &Config{SlackAppToken: "x", SlackBotToken: "y", DatabaseURL: "postgres://x"}
	if err := c.ValidateServe(); err != nil {
		t.Fatalf("valid serve config rejected: %v", err)
	}
	c = &Config{SlackAppToken: "x", SlackBotToken: "y",
		CloudSQLInstance: "p:r:i", DBName: "mirror", DBUser: "u"}
	if err := c.ValidateServe(); err != nil {
		t.Fatalf("valid cloudsql serve config rejected: %v", err)
	}
}

func TestFileConfig(t *testing.T) {
	t.Setenv("SLACK_APP_TOKEN", "x")
	t.Setenv("SLACK_BOT_TOKEN", "y")
	t.Setenv("DATABASE_URL", "postgres://localhost/mirror")

	c, _ := Load()
	if c.FilesEnabled() {
		t.Fatal("files disabled by default")
	}

	t.Setenv("FILE_STORAGE", "local")
	t.Setenv("FILE_DIR", "/tmp/blobs")
	t.Setenv("FILE_MAX_BYTES", "1048576")
	t.Setenv("FILE_MIME_ALLOWLIST", "image/png, application/pdf")
	c, _ = Load()
	if !c.FilesEnabled() {
		t.Fatal("FILE_STORAGE=local should enable files")
	}
	if c.FileDir != "/tmp/blobs" || c.FileMaxBytes != 1048576 {
		t.Fatalf("bad file cfg: %+v", c)
	}
	if !c.FileMimeAllowlist["image/png"] || !c.FileMimeAllowlist["application/pdf"] {
		t.Fatalf("mime allowlist = %v", c.FileMimeAllowlist)
	}
}

func TestStoreBackend(t *testing.T) {
	t.Setenv("SLACK_APP_TOKEN", "x")
	t.Setenv("SLACK_BOT_TOKEN", "y")

	// default: postgres, and serve requires a DB target
	t.Setenv("DATABASE_URL", "postgres://localhost/mirror")
	c, _ := Load()
	if c.StoreBackend != "postgres" {
		t.Fatalf("default backend = %q, want postgres", c.StoreBackend)
	}
	if c.SqliteEnabled() {
		t.Fatal("sqlite should be off by default")
	}

	// sqlite: no DB target required; SQLITE_PATH defaulted
	t.Setenv("DATABASE_URL", "")
	t.Setenv("STORE_BACKEND", "sqlite")
	c, _ = Load()
	if !c.SqliteEnabled() {
		t.Fatal("STORE_BACKEND=sqlite should enable sqlite")
	}
	if c.SQLitePath != "/data/mirror.db" {
		t.Fatalf("SQLitePath = %q, want /data/mirror.db", c.SQLitePath)
	}
	if err := c.ValidateServe(); err != nil {
		t.Fatalf("sqlite serve should not require a DB target: %v", err)
	}

	t.Setenv("SQLITE_PATH", "/tmp/x.db")
	c, _ = Load()
	if c.SQLitePath != "/tmp/x.db" {
		t.Fatalf("SQLitePath override = %q", c.SQLitePath)
	}
}
