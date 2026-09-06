package events

import (
	"encoding/json"
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
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&incoming); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, event := range incoming {
			if err := dispatcher.Dispatch(r.Context(), event); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
