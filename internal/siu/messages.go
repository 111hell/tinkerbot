package siu

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

type HistoryMessage struct {
	MessageID   string     `json:"message_id"`
	MessageType string     `json:"message_type"`
	FromUserID  uint64     `json:"from_user_id"`
	ToUserID    uint64     `json:"to_user_id"`
	Text        string     `json:"text"`
	Attachment  any        `json:"attachment"`
	Payload     any        `json:"payload,omitempty"`
	TopicID     string     `json:"topic_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

type ListMessagesResponse struct {
	Messages []*HistoryMessage `json:"messages"`
	HasMore  bool              `json:"has_more"`
}

func (c *Client) ListMessages(ctx context.Context, userID uint64, beforeMessageID string, limit int) (*ListMessagesResponse, error) {
	query := url.Values{"user_id": {strconv.FormatUint(userID, 10)}}
	if beforeMessageID != "" {
		query.Set("before_message_id", beforeMessageID)
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	var response ListMessagesResponse
	if err := c.get(ctx, "message.listMessages", query, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

type SendMessageRequest struct {
	ToUserID  uint64 `json:"to_user_id"`
	ToGroupID uint64 `json:"to_group_id"`
	Text      string `json:"text"`
	TopicID   string `json:"topic_id"`
}

type SendMessageResponse struct {
	MessageID string `json:"message_id"`
}

func (c *Client) SendMessage(ctx context.Context, request SendMessageRequest) (*SendMessageResponse, error) {
	var response SendMessageResponse
	if err := c.post(ctx, "message.sendMessage", request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

type SendStreamMessageRequest struct {
	ToUserID      uint64   `json:"to_user_id"`
	ToGroupID     uint64   `json:"to_group_id"`
	TopicID       string   `json:"topic_id"`
	Index         uint32   `json:"index"`
	Text          string   `json:"text"`
	RichReasoning []string `json:"rich_reasoning"`
	ClearText     bool     `json:"clear_text"`
	MessageID     string   `json:"message_id"`
	UserMessageID string   `json:"user_message_id"`
	SessionID     string   `json:"session_id"`
}

type SendStreamMessageResponse struct {
	MessageID string `json:"message_id"`
}

func (c *Client) SendStreamMessage(ctx context.Context, request SendStreamMessageRequest) (*SendStreamMessageResponse, error) {
	var response SendStreamMessageResponse
	if err := c.post(ctx, "message.sendStreamMessage", request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

type FinishStreamMessageRequest struct {
	ToUserID      uint64   `json:"to_user_id"`
	ToGroupID     uint64   `json:"to_group_id"`
	MessageID     string   `json:"message_id"`
	UserMessageID string   `json:"user_message_id"`
	Text          *string  `json:"text"`
	RichReasoning []string `json:"rich_reasoning"`
}

type FinishStreamMessageResponse struct {
	MessageID string `json:"message_id"`
}

func (c *Client) FinishStreamMessage(ctx context.Context, request FinishStreamMessageRequest) (*FinishStreamMessageResponse, error) {
	var response FinishStreamMessageResponse
	if err := c.post(ctx, "message.finishStreamMessage", request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
