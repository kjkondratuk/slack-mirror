package blobstore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Local struct{ root string }

func NewLocal(root string) *Local { return &Local{root: root} }

func (l *Local) Put(_ context.Context, key string, r io.Reader, _ string) (string, error) {
	full := filepath.Join(l.root, filepath.Clean("/"+key))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(full)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	return "file://" + abs, nil
}

func (l *Local) Delete(_ context.Context, uri string) error {
	path := strings.TrimPrefix(uri, "file://")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete %s: %w", uri, err)
	}
	return nil
}
