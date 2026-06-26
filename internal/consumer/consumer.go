package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/kjkondratuk/slack-mirror/internal/dispatch"
	"github.com/kjkondratuk/slack-mirror/internal/files"
	"github.com/kjkondratuk/slack-mirror/internal/health"
	"github.com/kjkondratuk/slack-mirror/internal/model"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// FileHandler downloads + records a message's file attachments. *files.Downloader implements it.
type FileHandler interface {
	Handle(ctx context.Context, channelID, messageTS string, refs []model.FileRef) error
}

type Consumer struct {
	client     *socketmode.Client
	store      Writer
	resolver   Resolver
	filter     dispatch.Filter
	log        *slog.Logger
	state      *health.State
	downloader FileHandler
}

func NewConsumer(sm *socketmode.Client, store Writer, r Resolver, f dispatch.Filter, log *slog.Logger, state *health.State, downloader FileHandler) *Consumer {
	return &Consumer{client: sm, store: store, resolver: r, filter: f, log: log, state: state, downloader: downloader}
}

// handleMessage maps one message event to an action and applies it.
func (c *Consumer) handleMessage(ctx context.Context, ev *slackevents.MessageEvent, rawPayload []byte) error {
	channelID := ev.Channel
	act, err := dispatch.Dispatch(channelID, ev, c.filter)
	if err != nil {
		return err
	}
	if c.log != nil {
		c.log.Info("event", "channel", channelID, "ts", act.TS, "action", act.Kind.String())
	}
	if err := Apply(ctx, c.store, c.resolver, act); err != nil {
		if c.state != nil {
			c.state.WriteErrors.Add(1)
		}
		return err
	}
	if c.state != nil {
		c.state.MarkEvent(time.Now())
	}
	// File attachments: only for upserted messages (FK requires the message row to exist).
	if c.downloader != nil && act.Kind == model.ActionUpsert {
		if msg := fileMsg(ev, rawPayload); msg != nil {
			if refs := files.FromMsg(msg); len(refs) > 0 {
				if err := c.downloader.Handle(ctx, act.ChannelID, act.TS, refs); err != nil {
					c.log.Error("file handle", "channel", act.ChannelID, "ts", act.TS, "err", err)
				}
			}
		}
	}
	return nil
}

// fileMsg returns the slack.Msg carrying file attachments for a message event.
// For message_changed the inner ev.Message holds the files; for a new message we
// parse the raw payload's event object into a slack.Msg (slackevents.MessageEvent
// does not surface files at the top level).
func fileMsg(ev *slackevents.MessageEvent, rawPayload []byte) *slack.Msg {
	if ev.SubType == "message_changed" {
		return ev.Message
	}
	if len(rawPayload) == 0 {
		return nil
	}
	var p struct {
		Event json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal(rawPayload, &p); err != nil || len(p.Event) == 0 {
		return nil
	}
	var m slack.Msg
	if err := json.Unmarshal(p.Event, &m); err != nil {
		return nil
	}
	return &m
}

// Run consumes Socket Mode events until ctx is cancelled. It acks each envelope
// immediately (Slack retries within ~3s) and then performs the idempotent write.
func (c *Consumer) Run(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-c.client.Events:
				if !ok {
					return
				}
				switch evt.Type {
				case socketmode.EventTypeConnecting:
					c.log.Info("socketmode connecting")
				case socketmode.EventTypeConnected:
					c.log.Info("socketmode connected")
					if c.state != nil {
						c.state.SetConnected(true)
					}
				case socketmode.EventTypeDisconnect:
					c.log.Info("socketmode disconnected")
					if c.state != nil {
						c.state.SetConnected(false)
					}
				case socketmode.EventTypeEventsAPI:
					eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
					if !ok {
						continue
					}
					var payload []byte
					if evt.Request != nil {
						c.client.Ack(*evt.Request) // ack first, then write
						payload = evt.Request.Payload
					}
					if eventsAPI.Type == slackevents.CallbackEvent {
						if msg, ok := eventsAPI.InnerEvent.Data.(*slackevents.MessageEvent); ok {
							if err := c.handleMessage(ctx, msg, payload); err != nil {
								c.log.Error("handle message", "err", err)
							}
						}
					}
				}
			}
		}
	}()
	return c.client.RunContext(ctx)
}
