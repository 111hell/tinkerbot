package siu

import (
	"context"

	"github.com/111hell/tinker/agent"

	"github.com/111hell/tinkerbot/internal/conversation"
	siuc "github.com/111hell/tinkerbot/internal/siu"
)

type listMessagesArgs struct {
	Limit           int    `json:"limit,omitempty"`
	BeforeMessageID string `json:"before_message_id,omitempty"`
}

type listMessagesResult struct {
	Messages            []*siuc.HistoryMessage `json:"messages"`
	HasMore             bool                   `json:"has_more"`
	NextBeforeMessageID string                 `json:"next_before_message_id,omitempty"`
}

func NewListMessages(client *siuc.Client) agent.Tool {
	return newTool(ListMessagesName,
		"Read recent history in the current private SIU Topic (e.g. 我刚才说了什么 / 总结这段对话), newest first, within the last 30 days. User and Topic are supplied automatically. For keyword search across chats use siu_search_chats. Start with {}; continue with next_before_message_id when more history is needed. A filtered page can be empty with has_more=true; this does not mean there is no history. Stop paging when has_more=false; do not repeat an exhausted query with a larger limit. Deleted messages are excluded.",
		`{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Page size before Topic filtering, default 50, at most 100."},"before_message_id":{"type":"string","description":"Use next_before_message_id from the previous page; omit on the first call."}},"additionalProperties":false}`,
		func(ctx context.Context, current conversation.Current, args listMessagesArgs) (any, error) {
			if err := requirePrivate(current); err != nil {
				return nil, err
			}
			if args.Limit <= 0 || args.Limit > 100 {
				args.Limit = 50
			}
			response, err := client.ListMessages(ctx, current.UserID, args.BeforeMessageID, args.Limit)
			if err != nil {
				return nil, err
			}
			messages := make([]*siuc.HistoryMessage, 0, len(response.Messages))
			for _, message := range response.Messages {
				if message != nil && message.TopicID == current.TopicID && message.DeletedAt == nil {
					messages = append(messages, message)
				}
			}
			result := listMessagesResult{Messages: messages, HasMore: response.HasMore}
			if response.HasMore {
				for i := len(response.Messages) - 1; i >= 0; i-- {
					if response.Messages[i] != nil {
						result.NextBeforeMessageID = response.Messages[i].MessageID
						break
					}
				}
			}
			return result, nil
		})
}
