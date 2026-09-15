package events

import (
	"context"
	"errors"
	"io"
	"iter"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/111hell/tinker/agent"
	"github.com/111hell/tinkerbot/internal/chat"
	"github.com/111hell/tinkerbot/internal/conversation"
	"github.com/111hell/tinkerbot/internal/run"
	"github.com/111hell/tinkerbot/internal/siu"
)

type testRunner struct {
	deltas  []agent.Delta
	input   string
	history []agent.Message
}

func (r *testRunner) Run(ctx context.Context, history []agent.Message, input string) iter.Seq2[agent.Event, error] {
	return func(yield func(agent.Event, error) bool) {
		r.input, r.history = input, history
		var text strings.Builder
		for _, delta := range r.deltas {
			text.WriteString(delta.Text)
			if !yield(agent.Event{Type: agent.MessageDelta, Delta: delta}, nil) {
				return
			}
		}
		messages := append(append([]agent.Message(nil), history...), agent.UserMessage(input), agent.Message{Role: agent.AssistantRole, Text: text.String()})
		yield(agent.Event{Type: agent.RunCompleted, Messages: messages}, nil)
	}
}

type testReplies struct {
	plain       []siu.SendMessageRequest
	stream      []siu.SendStreamMessageRequest
	finish      []siu.FinishStreamMessageRequest
	onStream    func(siu.SendStreamMessageRequest) error
	finishError error
}

func (r *testReplies) SendMessage(ctx context.Context, req siu.SendMessageRequest) (*siu.SendMessageResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.plain = append(r.plain, req)
	return &siu.SendMessageResponse{MessageID: "plain-reply"}, nil
}
func (r *testReplies) SendStreamMessage(ctx context.Context, req siu.SendStreamMessageRequest) (*siu.SendStreamMessageResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.stream = append(r.stream, req)
	if r.onStream != nil {
		if err := r.onStream(req); err != nil {
			return nil, err
		}
	}
	return &siu.SendStreamMessageResponse{MessageID: "bot-reply"}, nil
}
func (r *testReplies) FinishStreamMessage(ctx context.Context, req siu.FinishStreamMessageRequest) (*siu.FinishStreamMessageResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.finish = append(r.finish, req)
	return &siu.FinishStreamMessageResponse{MessageID: "bot-reply"}, r.finishError
}

type testHistory struct{ calls int }

func (h *testHistory) ListMessages(ctx context.Context, userID uint64, before string, limit int) (*siu.ListMessagesResponse, error) {
	h.calls++
	return &siu.ListMessagesResponse{}, nil
}

func setupHandler(runner chat.Runner, replies *testReplies) (*EventHandler, *testHistory) {
	manager := run.NewManager()
	history := &testHistory{}
	sessions := chat.NewService(runner, time.Minute, chat.NewMemoryStore())
	handler := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), conversation.NewService(chat.NewMemoryStore(), history), manager, sessions, replies, nil, 64)
	return handler, history
}

func messageEvent(topic string, groupID uint64) *siu.Event {
	kind := siu.EventTypeUserMessage
	if groupID != 0 {
		kind = siu.EventTypeGroupMessage
	}
	return &siu.Event{ID: "event-1", EventType: kind, Data: &siu.EventData{Message: &siu.Message{MessageID: "user-message", UserID: 83, ToUserID: 64, ToGroupID: groupID, TopicID: topic, Text: "hello"}}}
}

func TestPrivateAndGroupReplyRouting(t *testing.T) {
	for _, test := range []struct {
		name, topic string
		group       uint64
		wantSession string
	}{
		{"topic", "topic-1", 0, "siu:topic-1"},
		{"no topic", "", 0, "siu:user:83"},
		{"group", "", 42, "siu:group:42"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &testRunner{deltas: []agent.Delta{{Reasoning: "thinking\nnext"}, {Text: "hello "}, {Text: "world"}}}
			replies := &testReplies{}
			handler, history := setupHandler(runner, replies)
			event := messageEvent(test.topic, test.group)
			event.Data.Message.Attachment = &siu.MessageAttachment{File: &siu.AttachmentFile{Filename: "notes.txt", URL: "https://files.example/notes"}}
			if err := handler.Handle(context.Background(), event); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(runner.input, "notes.txt") {
				t.Fatal("attachment-only metadata was dropped")
			}
			if _, err := handler.sessions.History(context.Background(), test.wantSession); err != nil {
				t.Fatal(err)
			}
			if test.topic == "" {
				if len(replies.stream) != 0 || len(replies.finish) != 0 || len(replies.plain) != 1 {
					t.Fatalf("expected only plain reply: %+v", replies)
				}
				plain := replies.plain[0]
				if plain.Text != "hello world" || plain.TopicID != "" || plain.ToGroupID != test.group {
					t.Fatalf("bad reply: %+v", plain)
				}
				if test.group != 0 && (plain.ToUserID != 0 || history.calls != 0) {
					t.Fatal("group reply used private conversation")
				}
			} else {
				if len(replies.plain) != 0 || len(replies.stream) != 2 || len(replies.finish) != 1 {
					t.Fatalf("expected stream then finish: %+v", replies)
				}
				for i, req := range replies.stream {
					if req.TopicID != test.topic || req.UserMessageID != "user-message" || req.Index != uint32(i) {
						t.Fatalf("mixed stream IDs: %+v", req)
					}
					if i == 0 && req.MessageID != "" || i > 0 && req.MessageID != "bot-reply" {
						t.Fatal("reply ID not propagated")
					}
				}
				if got := replies.stream[0].RichReasoning[0]; got != `{"type":"text","id":1,"payload":"thinking\nnext"}` {
					t.Fatalf("wrong rich reasoning protocol: %s", got)
				}
				if *replies.finish[0].Text != "hello world" {
					t.Fatal("final text is incomplete")
				}
			}
		})
	}
}

func TestStopAndSkipDuringStreaming(t *testing.T) {
	for _, stop := range []bool{false, true} {
		runner := &testRunner{deltas: []agent.Delta{{Reasoning: strings.Repeat("思", streamChunkRunes)}, {Reasoning: "second"}, {Text: "answer"}}}
		replies := &testReplies{}
		handler, _ := setupHandler(runner, replies)
		dispatcher := NewDispatcher(handler.runs, nil, 64)
		replies.onStream = func(req siu.SendStreamMessageRequest) error {
			if len(replies.stream) != 1 {
				return nil
			}
			kind, messageID := siu.EventTypeSkipThinking, "bot-reply"
			if stop {
				kind, messageID = siu.EventTypeStopResponding, "user-message"
			}
			return dispatcher.Dispatch(context.Background(), &siu.Event{ID: "control", EventType: kind, Data: &siu.EventData{Message: &siu.Message{UserID: 83, TopicID: "topic", MessageID: messageID}}})
		}
		err := handler.Handle(context.Background(), messageEvent("topic", 0))
		if stop {
			if !errors.Is(err, context.Canceled) || len(replies.stream) != 1 || len(replies.finish) != 0 || len(replies.plain) != 0 {
				t.Fatalf("stop still produced output: %+v, %v", replies, err)
			}
		} else if err != nil || len(replies.stream) != 2 || replies.stream[1].Text != "answer" || len(replies.finish) != 1 {
			t.Fatalf("skip must keep body and finish: %+v, %v", replies, err)
		}
	}
}

func TestPartialStreamFailureFinishesExistingReply(t *testing.T) {
	runner := &testRunner{deltas: []agent.Delta{{Text: strings.Repeat("一", streamChunkRunes)}, {Text: strings.Repeat("二", streamChunkRunes)}, {Text: "three"}}}
	replies := &testReplies{}
	replies.onStream = func(req siu.SendStreamMessageRequest) error {
		if req.Index == 1 {
			return errors.New("temporary network failure")
		}
		return nil
	}
	handler, _ := setupHandler(runner, replies)
	if err := handler.Handle(context.Background(), messageEvent("topic", 0)); err != nil {
		t.Fatal(err)
	}
	if len(replies.stream) != 2 || len(replies.plain) != 0 || len(replies.finish) != 1 || *replies.finish[0].Text != strings.Repeat("一", streamChunkRunes)+strings.Repeat("二", streamChunkRunes)+"three" {
		t.Fatalf("partial reply duplicated or lost: %+v", replies)
	}
}

func TestClosedStreamDoesNotFinishOrFallback(t *testing.T) {
	runner := &testRunner{deltas: []agent.Delta{{Text: "one"}, {Text: "two"}}}
	replies := &testReplies{onStream: func(siu.SendStreamMessageRequest) error { return &siu.APIError{Code: "CHAT_MESSAGE_STREAM_CLOSED"} }}
	handler, _ := setupHandler(runner, replies)
	if err := handler.Handle(context.Background(), messageEvent("topic", 0)); err != nil {
		t.Fatal(err)
	}
	if len(replies.stream) != 1 || len(replies.finish) != 0 || len(replies.plain) != 0 {
		t.Fatalf("closed stream was followed by output: %+v", replies)
	}
}

func TestEditInvalidatesHistoryWithoutCancelingOwnToolRun(t *testing.T) {
	runner := &testRunner{deltas: []agent.Delta{{Text: "answer"}}}
	replies := &testReplies{}
	handler, _ := setupHandler(runner, replies)
	ctx := context.Background()
	if err := handler.Handle(ctx, messageEvent("", 0)); err != nil {
		t.Fatal(err)
	}
	handler.runs.Prepare("user:83", "new-message")
	handle := handler.runs.Begin(ctx, "user:83", "new-message")
	defer handler.runs.Finish(handle)
	edit := &siu.Event{ID: "edit", EventType: siu.EventTypeEditMessage, Data: &siu.EventData{Message: &siu.Message{MessageID: "plain-reply", UserID: 64, ToUserID: 83, Text: "edited"}}}
	scheduler := &recordingScheduler{}
	dispatcher := NewDispatcher(handler.runs, scheduler, 64)
	if err := dispatcher.Dispatch(ctx, edit); err != nil {
		t.Fatal(err)
	}
	if handle.Context.Err() != nil {
		t.Fatal("own edit webhook canceled the tool's run")
	}
	if err := handler.Handle(ctx, edit); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.sessions.History(ctx, "siu:user:83"); !errors.Is(err, chat.ErrNotFound) {
		t.Fatal("edited history cache survived")
	}
	if len(replies.plain) != 1 {
		t.Fatal("edit generated another reply")
	}
}

func TestForkAndDeleteContext(t *testing.T) {
	runner := &testRunner{deltas: []agent.Delta{{Text: "answer"}}}
	handler, history := setupHandler(runner, &testReplies{})
	ctx := context.Background()
	fork := &siu.Event{ID: "fork", EventType: siu.EventTypeForkTopic, Data: &siu.EventData{User: &siu.User{ID: 83}, Topic: &siu.Topic{TopicID: "fork-topic"}, Messages: []*siu.Message{
		{MessageID: "copied-user", UserID: 83, Text: "earlier question", TopicID: "fork-topic"},
		{MessageID: "copied-bot", UserID: 64, Text: "earlier answer", TopicID: "fork-topic"},
	}}}
	if err := handler.Handle(ctx, fork); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(ctx, messageEvent("fork-topic", 0)); err != nil {
		t.Fatal(err)
	}
	if len(runner.history) != 2 || runner.history[1].Role != agent.AssistantRole || history.calls != 0 {
		t.Fatalf("fork not used: %+v", runner.history)
	}
	fork.EventType = siu.EventTypeDeleteTopic
	if err := handler.Handle(ctx, fork); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.sessions.History(ctx, "siu:fork-topic"); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("deleted topic history survived: %v", err)
	}
}
