package siu

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

type HistoryMessage struct {
	MessageID   string             `json:"message_id"`
	MessageType string             `json:"message_type"`
	FromUserID  uint64             `json:"from_user_id"`
	ToUserID    uint64             `json:"to_user_id"`
	Text        string             `json:"text"`
	Attachment  *MessageAttachment `json:"attachment"`
	Payload     any                `json:"payload,omitempty"`
	TopicID     string             `json:"topic_id,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	DeletedAt   *time.Time         `json:"deleted_at,omitempty"`
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
	ToUserID  uint64 `json:"to_user_id,omitempty"`
	ToGroupID uint64 `json:"to_group_id,omitempty"`
	Text      string `json:"text"`
	TopicID   string `json:"topic_id,omitempty"`
}

type SendMessageResponse struct {
	MessageID string `json:"message_id"`
}

func (c *Client) SendMessage(ctx context.Context, request SendMessageRequest) (*SendMessageResponse, error) {
	if err := validateTarget(request.ToUserID, request.ToGroupID, request.TopicID); err != nil {
		return nil, err
	}
	var response SendMessageResponse
	if err := c.post(ctx, "message.sendMessage", request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

type SendStreamMessageRequest struct {
	ToUserID      uint64   `json:"to_user_id"`
	TopicID       string   `json:"topic_id"`
	Index         uint32   `json:"index"`
	Text          string   `json:"text"`
	RichReasoning []string `json:"rich_reasoning,omitempty"`
	ClearText     bool     `json:"clear_text,omitempty"`
	MessageID     string   `json:"message_id,omitempty"`
	UserMessageID string   `json:"user_message_id"`
}

type SendStreamMessageResponse struct {
	MessageID string `json:"message_id"`
}

func (c *Client) SendStreamMessage(ctx context.Context, request SendStreamMessageRequest) (*SendStreamMessageResponse, error) {
	if request.ToUserID == 0 || request.TopicID == "" || request.UserMessageID == "" {
		return nil, fmt.Errorf("stream requires to_user_id, topic_id and user_message_id")
	}
	var response SendStreamMessageResponse
	if err := c.post(ctx, "message.sendStreamMessage", request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

type FinishStreamMessageRequest struct {
	ToUserID      uint64  `json:"to_user_id"`
	MessageID     string  `json:"message_id"`
	UserMessageID string  `json:"user_message_id"`
	Text          *string `json:"text,omitempty"`
	// Nil merges uploaded reasoning; an empty, non-nil slice clears it.
	RichReasoning []string `json:"rich_reasoning"`
}

type FinishStreamMessageResponse struct {
	MessageID string `json:"message_id"`
}

func (c *Client) FinishStreamMessage(ctx context.Context, request FinishStreamMessageRequest) (*FinishStreamMessageResponse, error) {
	if request.ToUserID == 0 || request.MessageID == "" || request.UserMessageID == "" {
		return nil, fmt.Errorf("finish stream requires to_user_id, message_id and user_message_id")
	}
	var response FinishStreamMessageResponse
	if err := c.post(ctx, "message.finishStreamMessage", request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func validateTarget(userID, groupID uint64, topicID string) error {
	if (userID == 0) == (groupID == 0) {
		return fmt.Errorf("exactly one of to_user_id and to_group_id is required")
	}
	if groupID != 0 && topicID != "" {
		return fmt.Errorf("group messages cannot have a topic_id")
	}
	return nil
}

type SendPhotoRequest struct {
	ToUserID     uint64 `json:"to_user_id,omitempty"`
	ToGroupID    uint64 `json:"to_group_id,omitempty"`
	TopicID      string `json:"topic_id,omitempty"`
	ImageURL     string `json:"image_url,omitempty"`
	FileID       string `json:"file_id,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	// SIU currently uses the main FileID even when ThumbnailFileID is supplied.
	ThumbnailFileID string `json:"thumbnail_file_id,omitempty"`
	Text            string `json:"text,omitempty"`
	Width           uint32 `json:"width"`
	Height          uint32 `json:"height"`
	PreviewARGB     string `json:"preview_argb,omitempty"`
}

func (c *Client) SendPhoto(ctx context.Context, request SendPhotoRequest) (*SendMessageResponse, error) {
	if err := validateTarget(request.ToUserID, request.ToGroupID, request.TopicID); err != nil {
		return nil, err
	}
	if request.FileID == "" && request.ImageURL == "" {
		return nil, fmt.Errorf("photo requires file_id or image_url")
	}
	if request.FileID != "" && (request.Width == 0 || request.Height == 0) {
		return nil, fmt.Errorf("file_id photos require positive width and height")
	}
	var response SendMessageResponse
	if err := c.post(ctx, "message.sendPhoto", request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

type EditMessageRequest struct {
	ToUserID  uint64 `json:"to_user_id,omitempty"`
	ToGroupID uint64 `json:"to_group_id,omitempty"`
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
}

type EditMessageResponse struct {
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
}

func (c *Client) EditMessage(ctx context.Context, request EditMessageRequest) (*EditMessageResponse, error) {
	if err := validateTarget(request.ToUserID, request.ToGroupID, ""); err != nil {
		return nil, err
	}
	if request.MessageID == "" || request.Text == "" {
		return nil, fmt.Errorf("edit requires message_id and nonempty text")
	}
	var response EditMessageResponse
	if err := c.post(ctx, "message.editMessage", request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

type RevokeMessageRequest struct {
	ToUserID  uint64 `json:"to_user_id,omitempty"`
	ToGroupID uint64 `json:"to_group_id,omitempty"`
	MessageID string `json:"message_id"`
}

func (c *Client) RevokeMessage(ctx context.Context, request RevokeMessageRequest) error {
	if err := validateTarget(request.ToUserID, request.ToGroupID, ""); err != nil {
		return err
	}
	if request.MessageID == "" {
		return fmt.Errorf("message_id is required")
	}
	return c.post(ctx, "message.revokeMessage", request, nil)
}

type EmptyStreamMessageRequest struct {
	ToUserID  uint64 `json:"to_user_id,omitempty"`
	ToGroupID uint64 `json:"to_group_id,omitempty"`
	MessageID string `json:"message_id"`
}

// EmptyStreamMessage clears an existing reply. To regenerate, reuse its reply,
// user-message and Topic IDs starting at index 0, then finish the stream.
// Clearing does not remove SIU's five-minute stop state.
func (c *Client) EmptyStreamMessage(ctx context.Context, request EmptyStreamMessageRequest) error {
	if err := validateTarget(request.ToUserID, request.ToGroupID, ""); err != nil {
		return err
	}
	if request.MessageID == "" {
		return fmt.Errorf("message_id is required")
	}
	return c.post(ctx, "message.emptyStreamMessage", request, nil)
}
