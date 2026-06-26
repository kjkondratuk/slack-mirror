// Package backfill seeds current state by paging conversations.history and
// conversations.replies, upserting into the same messages table as live capture.
// Idempotent against whatever serve has already captured.
package backfill

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/kjkondratuk/slack-mirror/internal/consumer"
	"github.com/kjkondratuk/slack-mirror/internal/model"
	"github.com/slack-go/slack"
)

// HistoryClient is the narrow Web API surface backfill needs. *slack.Client fits.
type HistoryClient interface {
	GetConversationHistoryContext(ctx context.Context, p *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error)
	GetConversationRepliesContext(ctx context.Context, p *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error)
}

type Backfiller struct {
	slack    HistoryClient
	store    consumer.Writer
	resolver consumer.Resolver
}

func New(s HistoryClient, store consumer.Writer, r consumer.Resolver) *Backfiller {
	return &Backfiller{slack: s, store: store, resolver: r}
}

// Channel pages all history for one channel within the last `days` days and
// upserts every message (and thread reply) via the shared Apply path.
func (b *Backfiller) Channel(ctx context.Context, channelID string, days int) error {
	oldest := ""
	if days > 0 {
		oldest = strconv.FormatInt(time.Now().AddDate(0, 0, -days).Unix(), 10) + ".000000"
	}

	cursor := ""
	for {
		resp, err := b.slack.GetConversationHistoryContext(ctx, &slack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Oldest:    oldest,
			Limit:     1000,
			Cursor:    cursor,
		})
		if err != nil {
			return fmt.Errorf("history %s: %w", channelID, err)
		}
		for i := range resp.Messages {
			m := &resp.Messages[i]
			if err := b.upsert(ctx, channelID, m); err != nil {
				return err
			}
			if (m.ReplyCount > 0 && m.ThreadTimestamp == "") || (m.ThreadTimestamp != "" && m.ThreadTimestamp == m.Timestamp) {
				if err := b.replies(ctx, channelID, m.Timestamp); err != nil {
					return err
				}
			}
		}
		if !resp.HasMore || resp.ResponseMetaData.NextCursor == "" {
			break
		}
		cursor = resp.ResponseMetaData.NextCursor
	}
	return nil
}

func (b *Backfiller) replies(ctx context.Context, channelID, threadTS string) error {
	cursor := ""
	for {
		msgs, hasMore, next, err := b.slack.GetConversationRepliesContext(ctx, &slack.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: threadTS,
			Limit:     1000,
			Cursor:    cursor,
		})
		if err != nil {
			return fmt.Errorf("replies %s/%s: %w", channelID, threadTS, err)
		}
		for i := range msgs {
			if err := b.upsert(ctx, channelID, &msgs[i]); err != nil {
				return err
			}
		}
		if !hasMore || next == "" {
			break
		}
		cursor = next
	}
	return nil
}

// upsert converts a slack.Message into a MessageRow and applies it via the
// shared consumer.Apply path (ensures channel/user metadata first).
func (b *Backfiller) upsert(ctx context.Context, channelID string, m *slack.Message) error {
	posted, err := parseTS(m.Timestamp)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(m)
	row := model.MessageRow{
		ChannelID: channelID,
		TS:        m.Timestamp,
		ThreadTS:  m.ThreadTimestamp,
		UserID:    firstNonEmpty(m.User, m.BotID),
		Text:      m.Text,
		Subtype:   m.SubType,
		Raw:       raw,
		PostedAt:  posted,
	}
	if m.Edited != nil && m.Edited.Timestamp != "" {
		if et, err := parseTS(m.Edited.Timestamp); err == nil {
			row.EditedAt = &et
		}
	}
	act := model.Action{Kind: model.ActionUpsert, ChannelID: channelID, TS: row.TS, Message: &row}
	return consumer.Apply(ctx, b.store, b.resolver, act)
}

func parseTS(ts string) (time.Time, error) {
	sec, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("ts %q: %w", ts, err)
	}
	return time.Unix(int64(sec), int64((sec-float64(int64(sec)))*1e9)).UTC(), nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
