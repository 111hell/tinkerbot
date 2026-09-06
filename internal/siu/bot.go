package siu

import (
	"context"
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
	var bot Bot
	return c.post(ctx, "bot.update", map[string]string{"webhook_url": webhookURL}, &bot)
}
