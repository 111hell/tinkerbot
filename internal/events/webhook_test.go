package events

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/111hell/tinkerbot/internal/run"
	"github.com/111hell/tinkerbot/internal/siu"
)

type recordingScheduler struct {
	calls int
	key   string
}

func (s *recordingScheduler) Submit(ctx context.Context, key string, event *siu.Event, accept func()) error {
	accept()
	s.calls++
	s.key = key
	return nil
}

func TestWebhookBatchDedupAndIndependentControl(t *testing.T) {
	manager := run.NewManager()
	scheduler := &recordingScheduler{}
	dispatcher := NewDispatcher(manager, scheduler, 64)
	handler := NewWebhookHandler(dispatcher)
	body := `[{"id":"e","event_type":"user_message","data":{"message":{"user_id":83,"message_id":"m","text":"hi"}}}]`
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body)))
		if w.Code != http.StatusNoContent {
			t.Fatalf("webhook status = %d", w.Code)
		}
	}
	if scheduler.calls != 1 || scheduler.key != "user:83" {
		t.Fatalf("duplicate or wrong conversation: %+v", scheduler)
	}
	handle := manager.Begin(context.Background(), "user:83", "m")
	defer manager.Finish(handle)
	duplicate := httptest.NewRecorder()
	handler.ServeHTTP(duplicate, httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body)))
	if handle.Context.Err() != nil || scheduler.calls != 1 {
		t.Fatal("duplicate delivery canceled active reply")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`[null,{"event_type":"future:event"},{"id":"stop","event_type":"bot:stop_responding","data":{"message":{"user_id":83,"message_id":"m"}}}]`)))
	if w.Code != http.StatusBadRequest || handle.Context.Err() == nil {
		t.Fatal("malformed event prevented later stop")
	}
}

type blockedHandler struct{ started chan struct{} }

func (h blockedHandler) Handle(ctx context.Context, _ *siu.Event) error {
	select {
	case h.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestFullQueueKeepsAcceptedIntentAndAllowsStop(t *testing.T) {
	manager := run.NewManager()
	handler := blockedHandler{started: make(chan struct{}, 1)}
	scheduler := NewScheduler(context.Background(), handler, 1, 1, time.Minute)
	defer scheduler.Close()
	dispatcher := NewDispatcher(manager, scheduler, 64)
	first := messageEvent("topic", 0)
	if err := dispatcher.Dispatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	second := messageEvent("topic", 0)
	second.ID, second.Data.Message.MessageID = "second-event", "second-message"
	if err := dispatcher.Dispatch(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	third := messageEvent("topic", 0)
	third.ID, third.Data.Message.MessageID = "third-event", "third-message"
	if err := dispatcher.Dispatch(context.Background(), third); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("queue error = %v", err)
	}
	if !manager.ShouldRun("topic", "second-message") {
		t.Fatal("rejected event superseded an accepted run")
	}
	stop := &siu.Event{ID: "stop", EventType: siu.EventTypeStopResponding, Data: &siu.EventData{Message: &siu.Message{UserID: 83, TopicID: "topic", MessageID: "second-message"}}}
	if err := dispatcher.Dispatch(context.Background(), stop); err != nil {
		t.Fatal(err)
	}
	if manager.ShouldRun("topic", "second-message") {
		t.Fatal("queue saturation blocked stop")
	}
}

func TestWebhookRequiresSingleArray(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `[] []`} {
		scheduler := &recordingScheduler{}
		handler := NewWebhookHandler(NewDispatcher(run.NewManager(), scheduler, 64))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body)))
		if w.Code != http.StatusBadRequest || scheduler.calls != 0 {
			t.Fatalf("accepted invalid webhook: %s", body)
		}
	}
}
