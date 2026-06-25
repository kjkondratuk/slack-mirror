// Package dispatch is the pure mapping layer: it turns a parsed Slack message
// event into a model.Action (upsert/delete/skip). No I/O, no DB, no network —
// everything here is unit-testable in isolation.
package dispatch

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Filter decides which channels and subtypes are persisted.
type Filter struct {
	Allow   map[string]bool // channel allowlist; empty => all channels allowed
	Deny    map[string]bool // channel denylist; always wins
	Persist map[string]bool // subtype allowlist; if non-empty, only these persist
	Skip    map[string]bool // subtype denylist
}

func (f Filter) ChannelAllowed(id string) bool {
	if f.Deny[id] {
		return false
	}
	if len(f.Allow) == 0 {
		return true
	}
	return f.Allow[id]
}

// SubtypePersisted reports whether a message with the given subtype should be
// stored. A normal message (empty subtype) always persists. Skip is checked
// first; then, if a Persist allowlist is configured, the subtype must be in it.
func (f Filter) SubtypePersisted(subtype string) bool {
	if subtype == "" {
		return true
	}
	if f.Skip[subtype] {
		return false
	}
	if len(f.Persist) > 0 {
		return f.Persist[subtype]
	}
	return true
}

// tsToTime converts a Slack ts ("seconds.micros") into a time.Time.
func tsToTime(ts string) (time.Time, error) {
	dot := strings.IndexByte(ts, '.')
	secPart := ts
	fracPart := ""
	if dot >= 0 {
		secPart = ts[:dot]
		fracPart = ts[dot+1:]
	}
	sec, err := strconv.ParseInt(secPart, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("ts %q: %w", ts, err)
	}
	var nsec int64
	if fracPart != "" {
		// Slack uses 6 fractional digits (microseconds). Pad/truncate to 9 for ns.
		if len(fracPart) > 9 {
			fracPart = fracPart[:9]
		}
		for len(fracPart) < 9 {
			fracPart += "0"
		}
		nsec, err = strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("ts frac %q: %w", ts, err)
		}
	}
	return time.Unix(sec, nsec).UTC(), nil
}
