package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/111hell/tinker/agent"
	"github.com/111hell/tinker/model/openaicompat"
	"github.com/111hell/tinkerbot/internal/siu"
)

type modelTransport func(*http.Request) (*http.Response, error)

func (f modelTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestModelReasoningReachesSIUInBatches(t *testing.T) {
	for _, field := range []string{"reasoning_content", "reasoning"} {
		t.Run(field, func(t *testing.T) {
			client := &http.Client{Transport: modelTransport(func(r *http.Request) (*http.Response, error) {
				w := httptest.NewRecorder()
				w.Header().Set("Content-Type", "text/event-stream")
				for i := 0; i < 100; i++ {
					fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"%s\":\"思\"}}]}\n\n", field)
				}
				for i := 0; i < 100; i++ {
					fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"答\"}}]}\n\n")
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				return w.Result(), nil
			})}
			model, err := openaicompat.New(openaicompat.Config{APIKey: "test", BaseURL: "https://model.test/v1", Model: "test", HTTPClient: client})
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := agent.New(agent.Config{Model: model})
			if err != nil {
				t.Fatal(err)
			}
			replies := &testReplies{}
			handler, _ := setupHandler(runtime, replies)
			if err := handler.Handle(context.Background(), messageEvent("topic", 0)); err != nil {
				t.Fatal(err)
			}
			if len(replies.stream) != 6 {
				t.Fatalf("got %d chunks, want 6", len(replies.stream))
			}
			for i, req := range replies.stream {
				if req.Index != uint32(i) {
					t.Fatalf("bad index: %+v", req)
				}
				if i < 3 {
					var row siu.RichReasoning
					if len(req.RichReasoning) != 1 {
						t.Fatalf("missing row: %+v", req)
					}
					if err := json.Unmarshal([]byte(req.RichReasoning[0]), &row); err != nil {
						t.Fatal(err)
					}
					n := min((i+1)*48, 100)
					if req.Text != "" || row.ID != 1 || row.Replace != (i > 0) || row.Payload != strings.Repeat("思", n) {
						t.Fatalf("invalid cumulative row: %+v", req)
					}
				} else if len(req.RichReasoning) != 0 || req.Text == "" {
					t.Fatalf("body out of order: %+v", req)
				}
			}
			if len(replies.finish) != 1 || *replies.finish[0].Text != strings.Repeat("答", 100) {
				t.Fatal("incomplete final answer")
			}
			want := (siu.RichReasoning{Type: "text", ID: 1, Payload: strings.Repeat("思", 100)}).JSONLine()
			if got := replies.finish[0].RichReasoning; len(got) != 1 || got[0] != want {
				t.Fatalf("final rows not compacted: %v", got)
			}
		})
	}
}

func TestStreamBufferFlushesAgedTextAndMixedDelta(t *testing.T) {
	var events []agent.Event
	b := streamBuffer{emit: func(e agent.Event) error { events = append(events, e); return nil }}
	if err := b.add("一", false); err != nil {
		t.Fatal(err)
	}
	b.since = time.Now().Add(-streamChunkInterval)
	if err := b.add("二", false); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Delta.Text != "一二" {
		t.Fatal("aged buffer not flushed")
	}
	if err := b.accept(agent.Event{Type: agent.MessageDelta, Delta: agent.Delta{Reasoning: "思考", Text: "正文"}}); err != nil {
		t.Fatal(err)
	}
	if err := b.accept(agent.Event{Type: agent.RunCompleted}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[1].Delta.Reasoning != "思考" || events[2].Delta.Text != "正文" || events[3].Type != agent.RunCompleted {
		t.Fatalf("order changed: %+v", events)
	}
}
