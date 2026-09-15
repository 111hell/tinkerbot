package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/111hell/tinker/agent"

	"github.com/111hell/tinkerbot/internal/chat"
	"github.com/111hell/tinkerbot/internal/conversation"
	"github.com/111hell/tinkerbot/internal/logging"
	"github.com/111hell/tinkerbot/internal/run"
	"github.com/111hell/tinkerbot/internal/siu"
	"github.com/111hell/tinkerbot/internal/tools"
	"github.com/111hell/tinkerbot/internal/toolscope"
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
	botID         uint64
}

func NewHandler(logger *slog.Logger, conversations *conversation.Service, runs *run.Manager, sessions *chat.Service, replies ReplySender, tools []string, botID uint64) *EventHandler {
	return &EventHandler{
		logger: logger, conversations: conversations, runs: runs,
		sessions: sessions, replies: replies, tools: append([]string(nil), tools...), botID: botID,
	}
}

func (h *EventHandler) Handle(ctx context.Context, event *siu.Event) (err error) {
	logger := h.logger.With("event_id", event.ID, "event_type", event.EventType, "topic_id", conversation.TopicID(event))
	if key, keyErr := conversation.KeyFromEvent(event, h.botID); keyErr == nil {
		logger = logger.With("conversation_id", key.ID())
	}
	if event.Data.Message != nil {
		logger = logger.With("message_id", event.Data.Message.MessageID, "user_id", event.Data.Message.UserID)
	}
	ctx = logging.WithLogger(ctx, logger)
	started := time.Now()
	logger.InfoContext(ctx, "SIU event handling started")
	defer func() {
		attrs := []any{"duration_ms", time.Since(started).Milliseconds()}
		switch {
		case err == nil:
			logger.InfoContext(ctx, "SIU event handling completed", attrs...)
		case errors.Is(err, context.Canceled):
			logger.InfoContext(ctx, "SIU event run canceled", attrs...)
		default:
			logger.ErrorContext(ctx, "handle SIU event failed", append(attrs, "error", err)...)
		}
	}()
	switch event.EventType {
	case siu.EventTypeUserMessage, siu.EventTypeGroupMessage:
		return h.handleMessage(ctx, event)
	case siu.EventTypeForkTopic:
		key, err := conversation.KeyFromEvent(event, h.botID)
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
		return h.sessions.Initialize(ctx, sessionID(key.ID()), history)
	case siu.EventTypeDeleteTopic, siu.EventTypeEditMessage:
		key, err := conversation.KeyFromEvent(event, h.botID)
		if err != nil {
			return err
		}
		// An edit invalidates cached text. The next private turn reloads SIU's
		// current history; it does not generate a second reply to our own edit.
		if err := h.conversations.DeleteTopic(ctx, key.ID()); err != nil {
			return err
		}
		if err := h.sessions.Reset(ctx, sessionID(key.ID())); err != nil {
			return err
		}
		h.runs.Forget(key.ID())
		return nil
	default:
		return fmt.Errorf("unsupported SIU event type %q", event.EventType)
	}
}

func (h *EventHandler) handleMessage(ctx context.Context, event *siu.Event) error {
	key, err := conversation.KeyFromEvent(event, h.botID)
	if err != nil {
		return err
	}
	message := event.Data.Message
	if !h.runs.ShouldRun(key.ID(), message.MessageID) {
		return context.Canceled
	}
	// Seed new sessions from existing SIU history, including legacy SQLite rows.
	if _, err := h.sessions.History(ctx, sessionID(key.ID())); errors.Is(err, chat.ErrNotFound) {
		logging.FromContext(ctx).InfoContext(ctx, "SIU history loading started")
		history, err := h.conversations.LoadHistory(ctx, key, message.MessageID)
		if err != nil {
			return err
		}
		logging.FromContext(ctx).InfoContext(ctx, "SIU history loaded", "history_messages", len(history))
		if err := h.sessions.Initialize(ctx, sessionID(key.ID()), history); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	handle := h.runs.Begin(ctx, key.ID(), message.MessageID)
	defer h.runs.Finish(handle)
	return h.runAgent(handle, key, message)
}

func (h *EventHandler) runAgent(handle run.Handle, key conversation.Key, message *siu.Message) error {
	current := conversation.Current{UserID: message.UserID, TopicID: key.TopicID, GroupID: key.GroupID}
	names := h.tools
	if key.GroupID != 0 {
		names = tools.ForSIUGroup(names)
	}
	ctx := toolscope.WithTools(conversation.WithCurrent(handle.Context, current), names)

	logger := logging.FromContext(ctx).With("generation", handle.Generation)
	ctx = logging.WithLogger(ctx, logger)
	var full strings.Builder
	var streamID string
	var index uint32
	var streamFailed bool
	var reasoning richReasoning
	canStream := key.TopicID != "" && key.GroupID == 0
	input := conversation.MessageText(message.Text, message.Attachment)
	if key.GroupID != 0 {
		input = fmt.Sprintf("[SIU group sender user_id=%d]\n%s", message.UserID, input)
	}
	buffer := streamBuffer{emit: func(event agent.Event) error {
		if canStream && handle.ThinkingContext.Err() == nil {
			if richChunks := reasoning.update(event); len(richChunks) > 0 && !streamFailed {
				response, sendErr := h.replies.SendStreamMessage(handle.ThinkingContext, siu.SendStreamMessageRequest{
					ToUserID: message.UserID, TopicID: key.TopicID, Index: index,
					RichReasoning: richChunks, MessageID: streamID,
					UserMessageID: message.MessageID,
				})
				if sendErr != nil {
					if siu.IsCode(sendErr, "CHAT_MESSAGE_STREAM_CLOSED") {
						return errStreamClosed
					}
					if !errors.Is(sendErr, context.Canceled) && handle.ThinkingContext.Err() == nil {
						streamFailed = true
						logger.WarnContext(ctx, "stream SIU reasoning failed; will deliver complete text", "reply_message_id", streamID, "error", sendErr)
					}
				} else {
					if streamID == "" {
						logger.InfoContext(ctx, "SIU reply stream started", "reply_message_id", response.MessageID)
					}
					streamID = response.MessageID
					h.runs.BindReply(handle, streamID)
					index++
				}
			}
		}

		text := event.Delta.Text
		if text == "" {
			return nil
		}
		full.WriteString(text)
		if !canStream || streamFailed {
			return nil
		}
		response, err := h.replies.SendStreamMessage(handle.Context, siu.SendStreamMessageRequest{
			ToUserID: message.UserID, TopicID: key.TopicID, Index: index, Text: text,
			MessageID: streamID, UserMessageID: message.MessageID,
		})
		if err != nil {
			if siu.IsCode(err, "CHAT_MESSAGE_STREAM_CLOSED") {
				return errStreamClosed
			}
			streamFailed = true
			logger.WarnContext(ctx, "stream SIU reply failed; will deliver complete text", "reply_message_id", streamID, "error", err)
			return nil
		}
		if streamID == "" {
			logger.InfoContext(ctx, "SIU reply stream started", "reply_message_id", response.MessageID)
		}
		streamID = response.MessageID
		h.runs.BindReply(handle, streamID)
		index++
		return nil
	}}
	err := h.sessions.Run(ctx, sessionID(key.ID()), input, buffer.accept)
	if errors.Is(err, errStreamClosed) {
		logger.InfoContext(ctx, "SIU reply stream closed", "reply_message_id", streamID)
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
	if !canStream || streamID == "" {
		return h.sendPlain(ctx, key, answer)
	}
	// If a later chunk failed, finish the existing reply with the full text.
	// This avoids leaving a partial reply beside a duplicate plain message.
	logger.InfoContext(ctx, "SIU reply finalizing", "reply_message_id", streamID, "chunks", index, "output_bytes", len(answer), "stream_fallback", streamFailed)
	_, err = h.replies.FinishStreamMessage(handle.Context, siu.FinishStreamMessageRequest{
		ToUserID: message.UserID, MessageID: streamID, UserMessageID: message.MessageID, Text: &answer, RichReasoning: reasoning.snapshot(),
	})
	if err != nil {
		if siu.IsCode(err, "CHAT_MESSAGE_STREAM_CLOSED") {
			logger.InfoContext(ctx, "SIU reply stream closed", "reply_message_id", streamID)
			return nil
		}
		return fmt.Errorf("finish SIU reply %s: %w", streamID, err)
	}
	logger.InfoContext(ctx, "SIU reply delivered", "reply_message_id", streamID, "delivery", "stream")
	return nil
}

func (h *EventHandler) sendPlain(ctx context.Context, key conversation.Key, answer string) error {
	request := siu.SendMessageRequest{Text: answer, TopicID: key.TopicID}
	if key.GroupID != 0 {
		request.ToGroupID = key.GroupID
	} else {
		request.ToUserID = key.UserID
	}
	logging.FromContext(ctx).InfoContext(ctx, "SIU plain reply sending", "output_bytes", len(answer))
	response, err := h.replies.SendMessage(ctx, request)
	if err != nil {
		return fmt.Errorf("send plain SIU reply: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "SIU reply delivered", "reply_message_id", response.MessageID, "delivery", "plain")
	return nil
}

// The prefix is an ingress concern; the shared runtime treats it as opaque.
func sessionID(topicID string) string { return "siu:" + topicID }
