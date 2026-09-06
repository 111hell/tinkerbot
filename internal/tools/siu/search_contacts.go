package siu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/111hell/tinker/agent"

	"myagent/internal/conversation"
	siuc "myagent/internal/siu"
)

type searchContactsArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

type searchContactsResult struct {
	Contacts []*siuc.SearchContactResult `json:"contacts"`
}

func NewSearchContacts(client *siuc.Client) agent.Tool {
	return agent.Tool{
		Name:        "siu_search_contacts",
		Description: "Search the current private-chat user's SIU contacts by name, alias, remark, or username and return matching user profiles.",
		InputSchema: `{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"}},"required":["query"],"additionalProperties":false}`,
		Execute: func(ctx context.Context, arguments string) (string, error) {
			var args searchContactsArgs
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return "", err
			}
			current, ok := conversation.FromContext(ctx)
			if !ok {
				return "", errors.New("current SIU conversation is unavailable")
			}
			args.Query = strings.TrimSpace(args.Query)
			if args.Query == "" {
				return "", errors.New("contact search query is required")
			}
			if utf8.RuneCountInString(args.Query) > 255 {
				return "", errors.New("contact search query must be at most 255 characters")
			}
			if args.Limit <= 0 {
				args.Limit = 20
			}
			if args.Limit > 100 {
				args.Limit = 100
			}
			response, err := client.SearchContacts(ctx, siuc.SearchContactsRequest{
				UserID: current.UserID,
				Query:  args.Query,
				Limit:  uint32(args.Limit),
			})
			if err != nil {
				return "", fmt.Errorf("search SIU contacts: %w", err)
			}
			output, err := json.Marshal(searchContactsResult{Contacts: response.Results})
			return string(output), err
		},
	}
}
