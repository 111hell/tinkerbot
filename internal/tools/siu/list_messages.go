package siu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/111hell/tinker/agent"

	"github.com/111hell/tinkerbot/internal/conversation"
	siuc "github.com/111hell/tinkerbot/internal/siu"
)

type listMessagesArgs struct {
	Limit int `json:"limit,omitempty"`
}

type listMessagesResult struct {
	Messages []*siuc.HistoryMessage `json:"messages"`
}

func NewListMessages(client *siuc.Client) agent.Tool {
	return agent.Tool{
		Name:        ListMessagesName,
		Description: "List recent messages from the current private SIU conversation.",
		InputSchema: `{"type":"object","properties":{"limit":{"type":"integer"}},"additionalProperties":false}`,
		Execute: func(ctx context.Context, arguments string) (string, error) {
			var args listMessagesArgs
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return "", err
			}
			current, ok := conversation.FromContext(ctx)
			if !ok {
				return "", errors.New("current SIU conversation is unavailable")
			}
			if args.Limit <= 0 || args.Limit > 100 {
				args.Limit = 50
			}
			response, err := client.ListMessages(ctx, current.UserID, "", args.Limit)
			if err != nil {
				return "", fmt.Errorf("list SIU messages: %w", err)
			}
			messages := make([]*siuc.HistoryMessage, 0, len(response.Messages))
			for _, message := range response.Messages {
				if message != nil && message.TopicID == current.TopicID {
					messages = append(messages, message)
				}
			}
			output, err := json.Marshal(listMessagesResult{Messages: messages})
			return string(output), err
		},
	}
}
