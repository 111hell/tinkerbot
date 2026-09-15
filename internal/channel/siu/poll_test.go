package siu

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/111hell/tinkerbot/internal/events"
	"github.com/111hell/tinkerbot/internal/run"
	siuapi "github.com/111hell/tinkerbot/internal/siu"
)

func TestOnlineEvents(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const start int64 = 1_800_000_000_000
	for _, kind := range []string{
		siuapi.EventTypeUserMessage, siuapi.EventTypeGroupMessage, siuapi.EventTypeEditMessage,
		siuapi.EventTypeForkTopic, siuapi.EventTypeDeleteTopic, siuapi.EventTypeStopResponding, siuapi.EventTypeSkipThinking,
	} {
		t.Run(kind, func(t *testing.T) {
			incoming := []*siuapi.Event{
				nil,
				{ID: "undated", EventType: kind},
				{ID: "old", EventType: kind, Timestamp: start - 1},
				{ID: "same-millisecond", EventType: kind, Timestamp: start},
				{ID: "new", EventType: kind, Timestamp: start + 1},
			}
			got := onlineEvents(incoming, start, logger)
			if len(got) != 1 || got[0].ID != "new" {
				t.Fatalf("unexpected events: %+v", got)
			}
		})
	}
	// All starts follow the same policy; a message from the intervening downtime
	// is never recovered, including short outages within Agentiu's two-minute TTL.
	for _, name := range []string{"first switch", "repeated switch", "normal polling restart"} {
		t.Run(name, func(t *testing.T) {
			incoming := []*siuapi.Event{
				{ID: "previous-online", Timestamp: start + 1},
				{ID: "during-downtime", Timestamp: start + 10},
				{ID: "after-restart", Timestamp: start + 21},
			}
			got := onlineEvents(incoming, start+20, logger)
			if len(got) != 1 || got[0].ID != "after-restart" {
				t.Fatalf("replayed offline work: %+v", got)
			}
		})
	}
}

type scriptedUpdates func(context.Context, siuapi.GetUpdatesRequest) (*siuapi.GetUpdatesResponse, error)

func (f scriptedUpdates) GetUpdates(ctx context.Context, request siuapi.GetUpdatesRequest) (*siuapi.GetUpdatesResponse, error) {
	return f(ctx, request)
}

type admittedEvents struct{ ids []string }

func (s *admittedEvents) Submit(_ context.Context, _ string, event *siuapi.Event, accept func()) error {
	accept()
	s.ids = append(s.ids, event.ID)
	return nil
}

func TestPollDrainsOfflineBatchesAndKeepsInterPollEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	scheduler := &admittedEvents{}
	dispatcher := events.NewDispatcher(run.NewManager(), scheduler, 169461)
	calls := 0
	message := func(id string, timestamp int64) *siuapi.Event {
		return &siuapi.Event{ID: id, Timestamp: timestamp, EventType: siuapi.EventTypeUserMessage, Data: &siuapi.EventData{Message: &siuapi.Message{UserID: 134, MessageID: id}}}
	}
	client := scriptedUpdates(func(_ context.Context, request siuapi.GetUpdatesRequest) (*siuapi.GetUpdatesResponse, error) {
		calls++
		if request.StartID != "" || request.NoTrim || request.Limit != pollLimit {
			t.Fatalf("unexpected consume contract: %+v", request)
		}
		old := time.Now().Add(-time.Hour).UnixMilli()
		// One millisecond ahead avoids accidentally sharing the startup millisecond.
		live := time.Now().UnixMilli() + 1
		switch calls {
		case 1:
			return &siuapi.GetUpdatesResponse{Events: []*siuapi.Event{message("old-page-1", old)}}, nil
		case 2:
			return &siuapi.GetUpdatesResponse{Events: []*siuapi.Event{message("live-1", live), message("delayed-old", old)}}, nil
		case 3:
			return &siuapi.GetUpdatesResponse{Events: []*siuapi.Event{message("between-polls", live)}}, nil
		default:
			cancel()
			return nil, context.Canceled
		}
	})
	if err := poll(ctx, client, dispatcher, logger); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(scheduler.ids, []string{"live-1", "between-polls"}) {
		t.Fatalf("admitted: %v", scheduler.ids)
	}
	if dispatcher.Seen("old-page-1") || dispatcher.Seen("delayed-old") {
		t.Fatal("offline events reached dispatcher")
	}
}
