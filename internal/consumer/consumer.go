package consumer

import (
	"context"
	"log/slog"

	"github.com/kjkondratuk/slack-mirror/internal/dispatch"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type Consumer struct {
	client   *socketmode.Client
	store    Writer
	resolver Resolver
	filter   dispatch.Filter
	log      *slog.Logger
}

func NewConsumer(sm *socketmode.Client, store Writer, r Resolver, f dispatch.Filter, log *slog.Logger) *Consumer {
	return &Consumer{client: sm, store: store, resolver: r, filter: f, log: log}
}

// handleMessage maps one message event to an action and applies it.
func (c *Consumer) handleMessage(ctx context.Context, ev *slackevents.MessageEvent) error {
	channelID := ev.Channel
	act, err := dispatch.Dispatch(channelID, ev, c.filter)
	if err != nil {
		return err
	}
	if c.log != nil {
		c.log.Info("event", "channel", channelID, "ts", act.TS, "action", act.Kind.String())
	}
	return Apply(ctx, c.store, c.resolver, act)
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
				case socketmode.EventTypeEventsAPI:
					eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
					if !ok {
						continue
					}
					if evt.Request != nil {
						c.client.Ack(*evt.Request) // ack first, then write
					}
					if eventsAPI.Type == slackevents.CallbackEvent {
						if msg, ok := eventsAPI.InnerEvent.Data.(*slackevents.MessageEvent); ok {
							if err := c.handleMessage(ctx, msg); err != nil {
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
