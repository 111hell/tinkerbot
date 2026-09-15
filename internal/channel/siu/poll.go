package siu

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/111hell/tinkerbot/internal/conversation"
	"github.com/111hell/tinkerbot/internal/events"
	siuapi "github.com/111hell/tinkerbot/internal/siu"
)

const (
	pollTimeoutSeconds = 30 // The application HTTP client allows 40 seconds.
	pollLimit          = 30
	pollMaxPending     = 1000
	pollRetryDelay     = 3 * time.Second
)

type updatesClient interface {
	GetUpdates(context.Context, siuapi.GetUpdatesRequest) (*siuapi.GetUpdatesResponse, error)
}

// poll uses SIU's consume-on-fetch mode. There is no separate acknowledgement:
// the server trims through the last returned ID before sending the response.
// Always fetch from the oldest remaining event, without a synthetic cursor.
func poll(ctx context.Context, client updatesClient, dispatcher *events.Dispatcher, logger *slog.Logger) error {
	// Events are live notifications, not durable jobs. A new polling lifetime
	// never resumes offline work. SIU event timestamps are Unix milliseconds;
	// this eligibility boundary assumes synchronized clocks, not exact dedup.
	startedAt := time.Now().UnixMilli()
	logger.InfoContext(ctx, "SIU polling online boundary", "started_at_ms", startedAt)
	var pending []*siuapi.Event
	defer func() {
		if len(pending) > 0 {
			logger.Error("SIU polling stopped with consumed events still pending", "pending_events", len(pending))
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		pending, err = dispatchPending(ctx, dispatcher, pending, logger)
		if err != nil {
			return err
		}
		if len(pending) >= pollMaxPending {
			return fmt.Errorf("SIU polling backlog reached %d events; stopping consumption", pollMaxPending)
		}
		timeout := pollTimeoutSeconds
		if len(pending) > 0 {
			// Retry admission promptly, while still fetching later controls.
			timeout = 1
		}
		response, err := client.GetUpdates(ctx, siuapi.GetUpdatesRequest{
			Limit: min(pollLimit, pollMaxPending-len(pending)), Timeout: timeout,
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.WarnContext(ctx, "SIU polling failed; retrying", "error", err, "retry_ms", pollRetryDelay.Milliseconds())
			if err := waitPoll(ctx, pollRetryDelay); err != nil {
				return err
			}
			continue
		}
		if len(response.Events) > 0 {
			logger.InfoContext(ctx, "SIU polling received", "event_count", len(response.Events), "pending_events", len(pending))
		}
		pending = append(pending, onlineEvents(response.Events, startedAt, logger)...)
		// Empty responses normally spent their time in the server's long poll.
		// Pace immediate empty responses too, e.g. from a misconfigured proxy.
		if len(response.Events) == 0 {
			if err := waitPoll(ctx, 100*time.Millisecond); err != nil {
				return err
			}
		}
	}
}

// Apply to every batch, including delayed delivery of pre-start events. Do not
// advance the boundary between requests: that would drop live inter-poll events.
func onlineEvents(incoming []*siuapi.Event, startedAt int64, logger *slog.Logger) []*siuapi.Event {
	kept := incoming[:0]
	for _, event := range incoming {
		if event == nil {
			continue
		}
		if event.Timestamp <= startedAt {
			logger.Info("SIU polling offline event discarded", "event_id", event.ID, "event_type", event.EventType, "event_timestamp_ms", event.Timestamp, "started_at_ms", startedAt)
			continue
		}
		kept = append(kept, event)
	}
	clear(incoming[len(kept):])
	return kept
}

// Keep normal events in order when admission is full. Controls may bypass them.
// A valid deletion invalidates older not-yet-admitted work for that Topic.
func dispatchPending(ctx context.Context, dispatcher *events.Dispatcher, incoming []*siuapi.Event, logger *slog.Logger) ([]*siuapi.Event, error) {
	pending := incoming[:0]
	blocked := false
	seen := make(map[string]bool, len(incoming))
	for i, event := range incoming {
		if event != nil && dispatcher.Seen(event.ID) {
			continue
		}
		if event != nil && event.ID != "" {
			if seen[event.ID] {
				continue
			}
			seen[event.ID] = true
		}
		control := event != nil && (event.EventType == siuapi.EventTypeStopResponding || event.EventType == siuapi.EventTypeSkipThinking || event.EventType == siuapi.EventTypeDeleteTopic)
		if blocked && !control {
			pending = append(pending, event)
			continue
		}
		err := dispatcher.Dispatch(ctx, event)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, events.ErrSchedulerClosed) {
			return append(pending, incoming[i:]...), err
		}
		if event != nil && event.EventType == siuapi.EventTypeDeleteTopic && (err == nil || errors.Is(err, events.ErrQueueFull)) {
			topicID := conversation.TopicID(event)
			kept := pending[:0]
			for _, older := range pending {
				if conversation.TopicID(older) == topicID {
					logger.InfoContext(ctx, "SIU pending event superseded by topic deletion", "event_id", older.ID, "topic_id", topicID)
					continue
				}
				kept = append(kept, older)
			}
			pending = kept
		}
		if errors.Is(err, events.ErrQueueFull) {
			pending = append(pending, event)
			blocked = true
			continue
		}
		if err != nil {
			// A malformed event is not retryable, and must not hide later controls.
			attrs := []any{"error", err}
			if event != nil {
				attrs = append(attrs, "event_id", event.ID, "event_type", event.EventType)
			}
			logger.WarnContext(ctx, "SIU polling event rejected", attrs...)
		}
	}
	clear(incoming[len(pending):])
	return pending, nil
}

func waitPoll(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
