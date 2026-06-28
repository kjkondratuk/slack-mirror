package sqlitestore

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenMigratesAndPragmas(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var fk int
	if err := s.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys=%d want 1", fk)
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type IN ('table','view') AND name IN ('channels','users','messages','files','message_files','messages_fts')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Fatalf("expected 6 tables incl. messages_fts, found %d", n)
	}
	s2, err := Open(filepath.Join(t.TempDir(), "m2.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
}
