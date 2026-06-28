package sqlitestore

import (
	"strings"
	"time"
)

// rfc renders a time as RFC3339Nano UTC for TEXT storage; zero time → "".
func rfc(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// nullStr returns nil for "" so optional columns land as SQL NULL.
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// rfcPtr renders *time.Time → RFC3339 string or nil.
func rfcPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// inPlaceholders returns "?,?,…" for the ids and the ids as []any.
func inPlaceholders(ids []string) (string, []any) {
	if len(ids) == 0 {
		return "", nil
	}
	ph := strings.Repeat("?,", len(ids))
	ph = ph[:len(ph)-1]
	args := make([]any, len(ids))
	for i, v := range ids {
		args[i] = v
	}
	return ph, args
}
