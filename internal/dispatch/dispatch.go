// Package dispatch is the pure mapping layer: it turns a parsed Slack message
// event into a model.Action (upsert/delete/skip). No I/O, no DB, no network —
// everything here is unit-testable in isolation.
package dispatch

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kjkondratuk/slack-mirror/internal/model"
	"github.com/slack-go/slack"
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

// messageToRow builds a MessageRow from a slack.Msg — the canonical message type:
// the inner message of a message_changed event, and the element embedded in
// backfilled history. Raw is the JSON re-marshaling (best-effort full payload).
func messageToRow(channelID string, m *slack.Msg) (model.MessageRow, error) {
	posted, err := tsToTime(m.Timestamp)
	if err != nil {
		return model.MessageRow{}, err
	}

	raw, err := json.Marshal(m)
	if err != nil {
		return model.MessageRow{}, fmt.Errorf("marshal raw: %w", err)
	}

	row := model.MessageRow{
		ChannelID: channelID,
		TS:        m.Timestamp,
		ThreadTS:  m.ThreadTimestamp,
		UserID:    m.User,
		Text:      m.Text,
		Subtype:   m.SubType,
		Raw:       raw,
		PostedAt:  posted,
	}
	if row.UserID == "" && m.BotID != "" {
		row.UserID = m.BotID
	}
	if m.Edited != nil && m.Edited.Timestamp != "" {
		if et, err := tsToTime(m.Edited.Timestamp); err == nil {
			row.EditedAt = &et
		}
	}
	return row, nil
}
