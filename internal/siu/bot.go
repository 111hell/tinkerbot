package siu

import (
	"context"
	"fmt"
	"time"
)

type Bot struct {
	ID        uint64    `json:"id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	AvatarURL string    `json:"avatar_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *Client) WhoAmI(ctx context.Context) (*Bot, error) {
	var bot Bot
	if err := c.get(ctx, "bot.whoami", nil, &bot); err != nil {
		return nil, err
	}
	return &bot, nil
}

func (c *Client) SetWebhook(ctx context.Context, webhookURL string) error {
	return c.post(ctx, "bot.update", map[string]string{"webhook_url": webhookURL}, nil)
}

type SendEventRequest struct {
	UserID    uint64 `json:"user_id"`
	EventType string `json:"event_type"`
	TopicID   string `json:"topic_id,omitempty"`
	Data      string `json:"data,omitempty"`
}

func (c *Client) SendEvent(ctx context.Context, request SendEventRequest) error {
	if request.UserID == 0 || request.EventType == "" {
		return fmt.Errorf("client event requires user_id and event_type")
	}
	return c.post(ctx, "bot.sendEvent", request, nil)
}
