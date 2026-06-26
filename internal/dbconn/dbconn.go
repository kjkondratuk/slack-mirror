// Package dbconn builds a *pgxpool.Pool from config. Two paths share one return
// type: a direct DATABASE_URL (local/dev) or the Cloud SQL Go connector with IAM
// or password auth (GCP). Callers don't care which path was taken.
package dbconn

import (
	"context"
	"fmt"
	"net"
	"strings"

	"cloud.google.com/go/cloudsqlconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kjkondratuk/slack-mirror/internal/config"
)

func useDirectURL(c *config.Config) bool { return c.DatabaseURL != "" }

// connectorDSN builds a key/value DSN for the Cloud SQL connector path. The
// connector supplies the network dial, so no host/port appears here. Under IAM
// auth the password is omitted entirely.
func connectorDSN(c *config.Config) string {
	parts := []string{"user=" + c.DBUser, "dbname=" + c.DBName}
	if !c.DBIAMAuth && c.DBPassword != "" {
		parts = append(parts, "password="+c.DBPassword)
	}
	return strings.Join(parts, " ")
}

// New returns a ready connection pool and a cleanup function that closes the
// Cloud SQL dialer (if used). Caller owns both pool.Close() and cleanup().
func New(ctx context.Context, c *config.Config) (*pgxpool.Pool, func(), error) {
	if useDirectURL(c) {
		pool, err := pgxpool.New(ctx, c.DatabaseURL)
		if err != nil {
			return nil, func() {}, fmt.Errorf("pgxpool (direct url): %w", err)
		}
		return pool, func() {}, nil
	}

	var opts []cloudsqlconn.Option
	if c.DBIAMAuth {
		opts = append(opts, cloudsqlconn.WithIAMAuthN())
	}
	dialer, err := cloudsqlconn.NewDialer(ctx, opts...)
	if err != nil {
		return nil, func() {}, fmt.Errorf("cloudsqlconn dialer: %w", err)
	}
	cleanup := func() { _ = dialer.Close() }

	var dialOpts []cloudsqlconn.DialOption
	if c.DBPrivateIP {
		dialOpts = append(dialOpts, cloudsqlconn.WithPrivateIP())
	}

	cfg, err := pgxpool.ParseConfig(connectorDSN(c))
	if err != nil {
		cleanup()
		return nil, func() {}, fmt.Errorf("parse connector dsn: %w", err)
	}
	cfg.ConnConfig.DialFunc = func(ctx context.Context, _ string, _ string) (net.Conn, error) {
		return dialer.Dial(ctx, c.CloudSQLInstance, dialOpts...)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		cleanup()
		return nil, func() {}, fmt.Errorf("pgxpool (connector): %w", err)
	}
	return pool, cleanup, nil
}
