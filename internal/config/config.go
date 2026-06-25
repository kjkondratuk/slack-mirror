// Package config loads the entire runtime configuration from environment
// variables (design §6). No file or flag config — the image is generic and
// driven purely by env + secrets.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	SlackAppToken string
	SlackBotToken string

	ChannelAllowlist []string
	ChannelDenylist  map[string]bool
	PersistSubtypes  map[string]bool
	SkipSubtypes     map[string]bool

	CloudSQLInstance string
	DBName           string
	DBUser           string
	DBPassword       string
	DBIAMAuth        bool
	DBPrivateIP      bool
	DatabaseURL      string

	BackfillDays int
	LogLevel     string
	Port         string
}

var defaultSkipSubtypes = []string{
	"channel_join", "channel_leave", "channel_topic", "channel_purpose",
	"channel_name", "channel_archive", "channel_unarchive",
}

func Load() (*Config, error) {
	c := &Config{
		SlackAppToken:    os.Getenv("SLACK_APP_TOKEN"),
		SlackBotToken:    os.Getenv("SLACK_BOT_TOKEN"),
		ChannelAllowlist: splitList(os.Getenv("CHANNEL_ALLOWLIST")),
		ChannelDenylist:  toSet(splitList(os.Getenv("CHANNEL_DENYLIST"))),
		PersistSubtypes:  toSet(splitList(os.Getenv("PERSIST_SUBTYPES"))),
		CloudSQLInstance: os.Getenv("CLOUDSQL_INSTANCE"),
		DBName:           os.Getenv("DB_NAME"),
		DBUser:           os.Getenv("DB_USER"),
		DBPassword:       os.Getenv("DB_PASSWORD"),
		DBIAMAuth:        parseBool(os.Getenv("DB_IAM_AUTH")),
		DBPrivateIP:      parseBool(os.Getenv("DB_PRIVATE_IP")),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		LogLevel:         orDefault(os.Getenv("LOG_LEVEL"), "info"),
		Port:             orDefault(os.Getenv("PORT"), "8080"),
	}

	if raw := os.Getenv("SKIP_SUBTYPES"); raw != "" {
		c.SkipSubtypes = toSet(splitList(raw))
	} else {
		c.SkipSubtypes = toSet(defaultSkipSubtypes)
	}

	c.BackfillDays = 90
	if raw := os.Getenv("BACKFILL_DAYS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("BACKFILL_DAYS: %w", err)
		}
		c.BackfillDays = n
	}
	return c, nil
}

func (c *Config) hasDB() bool {
	return c.DatabaseURL != "" || (c.CloudSQLInstance != "" && c.DBName != "" && c.DBUser != "")
}

func (c *Config) ValidateServe() error {
	var missing []string
	if c.SlackAppToken == "" {
		missing = append(missing, "SLACK_APP_TOKEN")
	}
	if c.SlackBotToken == "" {
		missing = append(missing, "SLACK_BOT_TOKEN")
	}
	if !c.hasDB() {
		missing = append(missing, "DATABASE_URL or (CLOUDSQL_INSTANCE+DB_NAME+DB_USER)")
	}
	if len(missing) > 0 {
		return fmt.Errorf("serve: missing required config: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (c *Config) ValidateBackfill() error {
	var missing []string
	if c.SlackBotToken == "" {
		missing = append(missing, "SLACK_BOT_TOKEN")
	}
	if !c.hasDB() {
		missing = append(missing, "DATABASE_URL or (CLOUDSQL_INSTANCE+DB_NAME+DB_USER)")
	}
	if len(missing) > 0 {
		return fmt.Errorf("backfill: missing required config: %s", strings.Join(missing, ", "))
	}
	return nil
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, i := range items {
		m[i] = true
	}
	return m
}

func parseBool(s string) bool {
	b, _ := strconv.ParseBool(strings.TrimSpace(s))
	return b
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
