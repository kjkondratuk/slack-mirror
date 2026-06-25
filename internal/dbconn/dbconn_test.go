package dbconn

import (
	"strings"
	"testing"

	"github.com/kjkondratuk/slack-mirror/internal/config"
)

func TestConnectorDSN_IAM(t *testing.T) {
	c := &config.Config{DBUser: "svc@proj.iam", DBName: "mirror", DBIAMAuth: true, DBPassword: "ignored"}
	got := connectorDSN(c)
	if !strings.Contains(got, "user=svc@proj.iam") || !strings.Contains(got, "dbname=mirror") {
		t.Fatalf("dsn = %q", got)
	}
	if strings.Contains(got, "password=") {
		t.Fatalf("IAM auth DSN must not contain a password: %q", got)
	}
}

func TestConnectorDSN_Password(t *testing.T) {
	c := &config.Config{DBUser: "mirror", DBName: "mirror", DBIAMAuth: false, DBPassword: "s3cr3t"}
	got := connectorDSN(c)
	if !strings.Contains(got, "password=s3cr3t") {
		t.Fatalf("expected password in DSN: %q", got)
	}
}

func TestUseDirectURL(t *testing.T) {
	if !useDirectURL(&config.Config{DatabaseURL: "postgres://x"}) {
		t.Fatal("DATABASE_URL set should use direct URL")
	}
	if useDirectURL(&config.Config{CloudSQLInstance: "p:r:i"}) {
		t.Fatal("no DATABASE_URL should use connector")
	}
}
