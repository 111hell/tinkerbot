package siu

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/111hell/tinker/agent"

	"github.com/111hell/tinkerbot/internal/conversation"
	siuc "github.com/111hell/tinkerbot/internal/siu"
)

type searchContactsArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

type searchContactsResult struct {
	Contacts []*siuc.SearchContactResult `json:"contacts"`
}

func NewSearchContacts(client *siuc.Client) agent.Tool {
	return newTool(SearchContactsName,
		"Find other people in the current private-chat user's SIU contacts by name, alias, remark or username (e.g. 找一下小王). Returns matching contacts; an empty result means no matching contacts. For an explicit request to retrieve the current speaker's SIU profile, use siu_query_user with self=true. Greetings and self-introductions do not need a lookup. For an exact known user ID or username, query_user can retrieve a basic profile directly. Requires a privileged Bot.",
		`{"type":"object","properties":{"query":{"type":"string","description":"Nonempty name, alias, remark or username of another person; at most 255 characters."},"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Maximum contacts, default 20, at most 100."}},"required":["query"],"additionalProperties":false}`,
		func(ctx context.Context, current conversation.Current, args searchContactsArgs) (any, error) {
			if err := requirePrivate(current); err != nil {
				return nil, err
			}
			args.Query = strings.TrimSpace(args.Query)
			if args.Query == "" {
				return nil, errors.New("contact search query is required")
			}
			if utf8.RuneCountInString(args.Query) > 255 {
				return nil, errors.New("contact search query must be at most 255 characters")
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
				return nil, err
			}
			return searchContactsResult{Contacts: response.Results}, nil
		})
}
