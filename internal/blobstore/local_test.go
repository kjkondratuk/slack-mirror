package blobstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalPutGetDelete(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	bs := NewLocal(dir)

	uri, err := bs.Put(ctx, "files/F1", strings.NewReader("hello bytes"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uri, "file://") {
		t.Fatalf("uri = %q, want file:// prefix", uri)
	}
	got, err := os.ReadFile(filepath.Join(dir, "files/F1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello bytes" {
		t.Fatalf("content = %q", got)
	}
	if err := bs.Delete(ctx, uri); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "files/F1")); !os.IsNotExist(err) {
		t.Fatal("expected file removed after Delete")
	}
}
