package events

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/111hell/tinkerbot/internal/siu"
)

func NewWebhookHandler(dispatcher *Dispatcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var incoming []*siu.Event
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
		if err := decoder.Decode(&incoming); err != nil {
			slog.WarnContext(r.Context(), "SIU webhook rejected", "reason", "invalid_json", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if incoming == nil {
			slog.WarnContext(r.Context(), "SIU webhook rejected", "reason", "null_batch")
			http.Error(w, "webhook body must be an Event array", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			slog.WarnContext(r.Context(), "SIU webhook rejected", "reason", "trailing_data")
			http.Error(w, "webhook body must contain one Event array", http.StatusBadRequest)
			return
		}
		slog.DebugContext(r.Context(), "SIU webhook received", "event_count", len(incoming))
		status := http.StatusNoContent
		var firstError error
		for _, event := range incoming {
			if err := dispatcher.Dispatch(r.Context(), event); err != nil {
				if firstError == nil {
					firstError = err
				}
				// One malformed or unqueueable event must not discard later
				// control events in a batch that SIU will not actively retry.
				if errors.Is(err, ErrQueueFull) || errors.Is(err, ErrSchedulerClosed) {
					status = http.StatusServiceUnavailable
				} else if status == http.StatusNoContent {
					status = http.StatusBadRequest
				}
			}
		}
		if firstError != nil {
			slog.WarnContext(r.Context(), "SIU webhook dispatch failed", "status", status, "event_count", len(incoming), "error", firstError)
			http.Error(w, firstError.Error(), status)
			return
		}
		w.WriteHeader(status)
	})
}
