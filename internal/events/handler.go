package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/111hell/tinker/agent"

	"myagent/internal/chat"
	"myagent/internal/conversation"
	"myagent/internal/run"
	"myagent/internal/siu"
	"myagent/internal/toolscope"
)

var errStreamClosed = errors.New("SIU reply stream closed")

type ReplySender interface {
	SendMessage(context.Context, siu.SendMessageRequest) (*siu.SendMessageResponse, error)
	SendStreamMessage(context.Context, siu.SendStreamMessageRequest) (*siu.SendStreamMessageResponse, error)
	FinishStreamMessage(context.Context, siu.FinishStreamMessageRequest) (*siu.FinishStreamMessageResponse, error)
}

type EventHandler struct {
	logger        *slog.Logger
	conversations *conversation.Service
	runs          *run.Manager
	sessions      *chat.Service
	tools         []string
	replies       ReplySender
}

func NewHandler(logger *slog.Logger, conversations *conversation.Service, runs *run.Manager, sessions *chat.Service, replies ReplySender, tools []string) *EventHandler {
	return &EventHandler{
		logger: logger, conversations: conversations, runs: runs,
		sessions: sessions, replies: replies, tools: append([]string(nil), tools...),
	}
}

func (h *EventHandler) Handle(ctx context.Context, event *siu.Event) (err error) {
	defer func() {
		if err == nil {
			return
		}
		attrs := []any{"event_id", event.ID, "event_type", event.EventType, "topic_id", conversation.TopicID(event), "error", err}
		if errors.Is(err, context.Canceled) {
			h.logger.DebugContext(ctx, "SIU event run canceled", attrs...)
			return
		}
		h.logger.ErrorContext(ctx, "handle SIU event failed", attrs...)
	}()
	switch event.EventType {
	case siu.EventTypeUserMessage:
		return h.handleMessage(ctx, event)
	case siu.EventTypeForkTopic:
		key, err := conversation.KeyFromEvent(event)
		if err != nil {
			return err
		}
		if err := h.conversations.ImportFork(ctx, key, event.Data.Messages); err != nil {
			return err
		}
		history, err := h.conversations.LoadHistory(ctx, key, "")
		if err != nil {
			return err
		}
		return h.sessions.Initialize(ctx, sessionID(key.TopicID), history)
	case siu.EventTypeDeleteTopic:
		topicID := conversation.TopicID(event)
		if err := h.conversations.DeleteTopic(ctx, topicID); err != nil {
			return err
		}
		if err := h.sessions.Reset(ctx, sessionID(topicID)); err != nil {
			return err
		}
		h.runs.Forget(topicID)
		return nil
	default:
		return fmt.Errorf("unsupported SIU event type %q", event.EventType)
	}
}

func (h *EventHandler) handleMessage(ctx context.Context, event *siu.Event) error {
	key, err := conversation.KeyFromEvent(event)
	if err != nil {
		return err
	}
	message := event.Data.Message
	if !h.runs.ShouldRun(key.TopicID, message.MessageID) {
		return context.Canceled
	}
	// Seed new sessions from existing SIU history, including legacy SQLite rows.
	if _, err := h.sessions.History(ctx, sessionID(key.TopicID)); errors.Is(err, chat.ErrNotFound) {
		history, err := h.conversations.LoadHistory(ctx, key, message.MessageID)
		if err != nil {
			return err
		}
		if err := h.sessions.Initialize(ctx, sessionID(key.TopicID), history); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	handle := h.runs.Begin(ctx, key.TopicID, message.MessageID)
	defer h.runs.Finish(handle)
	return h.runAgent(handle, key, message)
}

func (h *EventHandler) runAgent(handle run.Handle, key conversation.Key, message *siu.Message) error {
	current := conversation.Current{UserID: message.UserID, TopicID: key.TopicID}
	ctx := toolscope.WithTools(conversation.WithCurrent(handle.Context, current), h.tools)

	var full strings.Builder
	var streamID string
	var index uint32
	var streamFailed bool
	err := h.sessions.Run(ctx, sessionID(key.TopicID), message.Text, func(event agent.Event) error {
		if event.Type != agent.MessageDelta {
			return nil
		}
		if text := event.Delta.Reasoning; text != "" {
			if !streamFailed && handle.ThinkingContext.Err() == nil {
				richChunk, _ := json.Marshal(map[string]string{"type": "reasoning", "text": text})
				response, sendErr := h.replies.SendStreamMessage(handle.ThinkingContext, siu.SendStreamMessageRequest{
					ToUserID: message.UserID, TopicID: key.TopicID, Index: index,
					RichReasoning: []string{string(richChunk)}, MessageID: streamID,
					UserMessageID: message.MessageID, SessionID: key.TopicID,
				})
				if sendErr != nil {
					if siu.IsCode(sendErr, "CHAT_MESSAGE_STREAM_CLOSED") {
						return errStreamClosed
					}
					if !errors.Is(sendErr, context.Canceled) && handle.ThinkingContext.Err() == nil {
						streamFailed = true
						h.logger.Warn("stream SIU reasoning failed; will use plain reply", "topic_id", key.TopicID, "error", sendErr)
					}
				} else {
					streamID = response.MessageID
					index++
				}
			}
		}

		text := event.Delta.Text
		if text == "" {
			return nil
		}
		full.WriteString(text)
		if streamFailed {
			return nil
		}
		response, err := h.replies.SendStreamMessage(handle.Context, siu.SendStreamMessageRequest{
			ToUserID: message.UserID, TopicID: key.TopicID, Index: index, Text: text,
			MessageID: streamID, UserMessageID: message.MessageID, SessionID: key.TopicID,
		})
		if err != nil {
			if siu.IsCode(err, "CHAT_MESSAGE_STREAM_CLOSED") {
				return errStreamClosed
			}
			streamFailed = true
			h.logger.Warn("stream SIU reply failed; will use plain reply", "topic_id", key.TopicID, "error", err)
			return nil
		}
		streamID = response.MessageID
		index++
		return nil
	})
	if errors.Is(err, errStreamClosed) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("run session: %w", err)
	}
	if err := handle.Context.Err(); err != nil {
		return err
	}

	answer := full.String()
	if answer == "" {
		return errors.New("agent returned no text")
	}
	if streamFailed {
		_, err := h.replies.SendMessage(handle.Context, siu.SendMessageRequest{
			ToUserID: message.UserID, Text: answer, TopicID: key.TopicID,
		})
		if err != nil {
			return fmt.Errorf("send plain SIU reply: %w", err)
		}
		return nil
	}
	_, err = h.replies.FinishStreamMessage(handle.Context, siu.FinishStreamMessageRequest{
		ToUserID: message.UserID, MessageID: streamID, UserMessageID: message.MessageID, Text: &answer,
	})
	if err != nil {
		if siu.IsCode(err, "CHAT_MESSAGE_STREAM_CLOSED") {
			return nil
		}
		_, fallbackErr := h.replies.SendMessage(handle.Context, siu.SendMessageRequest{
			ToUserID: message.UserID, Text: answer, TopicID: key.TopicID,
		})
		if fallbackErr != nil {
			return fmt.Errorf("finish SIU reply: %v; plain fallback: %w", err, fallbackErr)
		}
	}
	return nil
}

// The prefix is an ingress concern; the shared runtime treats it as opaque.
func sessionID(topicID string) string { return "siu:" + topicID }
