package siu

import (
	"context"
	"fmt"
	"strings"

	"github.com/111hell/tinker/agent"
	"github.com/111hell/tinkerbot/internal/conversation"
	siuc "github.com/111hell/tinkerbot/internal/siu"
)

type searchChatsArgs struct {
	Query string  `json:"query"`
	Limit uint32  `json:"limit,omitempty"`
	BotID *uint64 `json:"bot_id,omitempty"`
}

func NewSearchChats(client *siuc.Client) agent.Tool {
	return newTool(SearchChatsName,
		"Find chats by message keywords across records visible to the current private-chat user (e.g. 找聊过报销的对话). Returns matching chats and message snippets, not full transcripts or user profiles. For recent messages in this Topic use siu_list_messages; for people by name use siu_search_contacts. Requires a privileged Bot.",
		`{"type":"object","properties":{"query":{"type":"string","description":"Message-content keywords, not a person lookup."},"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Maximum matching chats, 1-50; omit for server default."},"bot_id":{"type":"integer","minimum":0,"description":"Optional known Bot participant ID; omit for no Bot filter."}},"required":["query"],"additionalProperties":false}`,
		func(ctx context.Context, current conversation.Current, args searchChatsArgs) (any, error) {
			if err := requirePrivate(current); err != nil {
				return nil, err
			}
			args.Query = strings.TrimSpace(args.Query)
			if args.Query == "" {
				return nil, fmt.Errorf("chat search query is required")
			}
			return client.SearchChats(ctx, siuc.SearchChatsRequest{UserID: current.UserID, Query: args.Query, Limit: args.Limit, BotID: args.BotID})
		})
}

type queryUserArgs struct {
	Self     bool   `json:"self,omitempty"`
	UserID   uint64 `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
}

func NewQueryUser(client *siuc.Client) agent.Tool {
	return newTool(QueryUserName,
		"Get a SIU user profile (ID, username, nickname, avatar URL). For 我是谁 / 我的资料 / who am I, call with self=true. The current speaker is resolved automatically, including in groups. Do not supply user_id or username for self, guess an ID, or use a schema minimum as a default. For another person ONLY, pass a user_id or exact username already known from the conversation or tool results. If only their nickname or remark is known, search contacts first when available. user_id takes precedence. Use self, user_id and username only. Empty arguments also default to self. Report returned fields only. Requires a privileged Bot.",
		`{"type":"object","properties":{"self":{"type":"boolean","description":"Set true for 我是谁 / 我的资料: query the current speaker. Omit user_id and username in this case."},"user_id":{"type":"integer","minimum":1,"description":"Known target user ID. Omit both selectors to query the current speaker. Takes precedence over username."},"username":{"type":"string","description":"Exact SIU username, not a nickname, remark, or the words 我 / 自己. Omit for self."}},"additionalProperties":false}`,
		func(ctx context.Context, current conversation.Current, args queryUserArgs) (any, error) {
			args.Username = strings.TrimSpace(args.Username)
			if args.Self && (args.UserID != 0 || args.Username != "") {
				return nil, fmt.Errorf("self=true cannot be combined with user_id or username")
			}
			if args.UserID == 0 && args.Username == "" {
				args.UserID = current.UserID
			}
			return client.QueryUser(ctx, siuc.QueryUserRequest{UserID: args.UserID, Username: args.Username})
		})
}
