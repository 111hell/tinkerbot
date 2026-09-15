package siu

import (
	"context"
	"net/url"
	"strconv"
)

const (
	EventTypeUserMessage    = "user_message"
	EventTypeGroupMessage   = "group_message"
	EventTypeEditMessage    = "edit_message"
	EventTypeForkTopic      = "bot:fork_topic"
	EventTypeDeleteTopic    = "bot:delete_topic"
	EventTypeSkipThinking   = "bot:skip_thinking"
	EventTypeStopResponding = "bot:stop_responding"
)

type Event struct {
	ID        string     `json:"id"`
	Version   string     `json:"version"`
	Timestamp int64      `json:"timestamp"`
	EventType string     `json:"event_type"`
	Data      *EventData `json:"data"`
}

type EventData struct {
	User     *User      `json:"user,omitempty"`
	Group    *Group     `json:"group,omitempty"`
	Message  *Message   `json:"message,omitempty"`
	Messages []*Message `json:"messages,omitempty"`
	Topic    *Topic     `json:"topic,omitempty"`
	Extra    *string    `json:"extra,omitempty"`
}

type User struct {
	ID         uint64 `json:"id"`
	ExternalID string `json:"external_id"`
	Nickname   string `json:"nickname"`
	AvatarURL  string `json:"avatar_url"`
}

type Group struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type Topic struct {
	TopicID string `json:"topic_id"`
}

type Message struct {
	MessageID    string             `json:"message_id"`
	MessageType  string             `json:"message_type"`
	UserID       uint64             `json:"user_id"`
	ToUserID     uint64             `json:"to_user_id,omitempty"`
	ToGroupID    uint64             `json:"to_group_id,omitempty"`
	ExternalID   string             `json:"external_id,omitempty"`
	ToExternalID string             `json:"to_external_id,omitempty"`
	Text         string             `json:"text"`
	Attachment   *MessageAttachment `json:"attachment"`
	Payload      any                `json:"payload,omitempty"`
	TopicID      string             `json:"topic_id,omitempty"`
}

type GetUpdatesRequest struct {
	Limit   int
	Timeout int // Long-poll timeout in seconds, at most 30.
	StartID string
	NoTrim  bool
}

type GetUpdatesResponse struct {
	Events []*Event `json:"events"`
}

// GetUpdates defaults to start_id=0 (oldest available). StartID is an exclusive
// Redis Stream cursor, not an acknowledgement. Unless NoTrim is true, SIU trims
// through the last returned ID BEFORE sending the response, including older IDs.
// There is no separate ack API: a lost response can lose consumed events.
// Use an HTTP timeout longer than the long poll and only one consumer per Bot.
func (c *Client) GetUpdates(ctx context.Context, request GetUpdatesRequest) (*GetUpdatesResponse, error) {
	query := url.Values{}
	if request.Limit > 0 {
		query.Set("limit", strconv.Itoa(request.Limit))
	}
	if request.Timeout > 0 {
		query.Set("timeout", strconv.Itoa(request.Timeout))
	}
	if request.StartID != "" {
		query.Set("start_id", request.StartID)
	}
	if request.NoTrim {
		query.Set("no_trim", "true")
	}
	var response GetUpdatesResponse
	if err := c.get(ctx, "bot.getUpdates", query, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
