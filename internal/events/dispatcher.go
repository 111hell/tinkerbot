package events

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/111hell/tinkerbot/internal/conversation"
	"github.com/111hell/tinkerbot/internal/run"
	"github.com/111hell/tinkerbot/internal/siu"
)

type EventScheduler interface {
	// accept runs only after capacity is reserved, before the handler can run.
	Submit(context.Context, string, *siu.Event, func()) error
}

type Dispatcher struct {
	runs      *run.Manager
	scheduler EventScheduler
	botID     uint64
	mu        sync.Mutex
	seen      map[string]struct{}
	order     []string
	next      int
}

func NewDispatcher(runs *run.Manager, scheduler EventScheduler, botID uint64) *Dispatcher {
	return &Dispatcher{runs: runs, scheduler: scheduler, botID: botID, seen: make(map[string]struct{})}
}

// Seen lets the polling ingress avoid reapplying local backlog deletion for an
// already accepted event. Queue-full events have not been marked as seen.
func (d *Dispatcher) Seen(eventID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.seen[eventID]
	return ok
}

func (d *Dispatcher) Dispatch(ctx context.Context, event *siu.Event) error {
	if event == nil {
		return fmt.Errorf("event is required")
	}
	switch event.EventType {
	case siu.EventTypeUserMessage, siu.EventTypeGroupMessage, siu.EventTypeEditMessage,
		siu.EventTypeForkTopic, siu.EventTypeDeleteTopic, siu.EventTypeStopResponding, siu.EventTypeSkipThinking:
	default:
		slog.DebugContext(ctx, "SIU event ignored", "event_id", event.ID, "event_type", event.EventType, "reason", "unsupported_type")
		// New protocol events must not reject the rest of a webhook batch.
		return nil
	}
	key, err := conversation.KeyFromEvent(event, d.botID)
	if err != nil {
		return err
	}
	if event.EventType == siu.EventTypeUserMessage || event.EventType == siu.EventTypeGroupMessage ||
		event.EventType == siu.EventTypeStopResponding || event.EventType == siu.EventTypeSkipThinking {
		if event.Data.Message == nil || event.Data.Message.MessageID == "" {
			return fmt.Errorf("message_id is required for event %q", event.EventType)
		}
	}
	logger := slog.Default().With("event_id", event.ID, "event_type", event.EventType, "conversation_id", key.ID(), "topic_id", key.TopicID)
	if event.Data.Message != nil {
		logger = logger.With("message_id", event.Data.Message.MessageID, "user_id", event.Data.Message.UserID)
	}
	// Serialize acceptance so duplicate deliveries cannot cancel an active reply.
	// Keep the most recent 4096 event IDs; this is process-local deduplication.
	d.mu.Lock()
	defer d.mu.Unlock()
	if event.ID != "" {
		if _, ok := d.seen[event.ID]; ok {
			logger.DebugContext(ctx, "SIU event ignored", "reason", "duplicate")
			return nil
		}
	}
	switch event.EventType {
	case siu.EventTypeStopResponding:
		matched := d.runs.CancelMessage(key.ID(), event.Data.Message.MessageID)
		logger.InfoContext(ctx, "SIU stop requested", "active_run_matched", matched)
	case siu.EventTypeSkipThinking:
		matched := d.runs.CancelThinking(key.ID(), event.Data.Message.MessageID)
		logger.InfoContext(ctx, "SIU skip thinking requested", "active_run_matched", matched)
	default:
		// Deletion must stop generation even when cleanup cannot yet be queued.
		// Cancel is idempotent; a queue-full retry still schedules cleanup.
		if event.EventType == siu.EventTypeDeleteTopic {
			d.runs.Cancel(key.ID())
		}
		if err := d.scheduler.Submit(ctx, key.ID(), event, func() {
			logger.InfoContext(ctx, "SIU event accepted")
			switch event.EventType {
			case siu.EventTypeUserMessage, siu.EventTypeGroupMessage:
				d.runs.Prepare(key.ID(), event.Data.Message.MessageID)
			case siu.EventTypeDeleteTopic:
				d.runs.Cancel(key.ID())
			}
		}); err != nil {
			logger.WarnContext(ctx, "SIU event enqueue failed", "error", err)
			return err
		}
	}
	if event.ID != "" {
		if len(d.order) < 4096 {
			d.order = append(d.order, event.ID)
		} else {
			delete(d.seen, d.order[d.next])
			d.order[d.next] = event.ID
			d.next = (d.next + 1) % len(d.order)
		}
		d.seen[event.ID] = struct{}{}
	}
	return nil
}
