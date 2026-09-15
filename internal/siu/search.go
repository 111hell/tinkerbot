package siu

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

type RecentUser struct {
	UserID        uint64          `json:"user_id"`
	LatestMessage *HistoryMessage `json:"latest_message"`
}

type ListRecentUsersResponse struct {
	Users   []*RecentUser `json:"users"`
	HasMore bool          `json:"has_more"`
}

func (c *Client) ListRecentUsers(ctx context.Context, beforeMessageID string, limit int) (*ListRecentUsersResponse, error) {
	query := url.Values{}
	if beforeMessageID != "" {
		query.Set("before_message_id", beforeMessageID)
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	var response ListRecentUsersResponse
	if err := c.get(ctx, "message.listRecentUsers", query, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

type SearchChatsRequest struct {
	UserID uint64  `json:"user_id"`
	Query  string  `json:"query"`
	Limit  uint32  `json:"limit,omitempty"`
	BotID  *uint64 `json:"bot_id,omitempty"`
}

type SearchChatsResponse struct {
	Results []*SearchChatResult `json:"results"`
}

// Search endpoints use Protobuf JSON: camelCase keys and string uint64 IDs.
type SearchChatResult struct {
	FromUserID         string                  `json:"fromUserId"`
	ToGroupOrUserID    string                  `json:"toGroupOrUserId"`
	IsGroup            bool                    `json:"isGroup"`
	LatestMatchedMsgAt time.Time               `json:"latestMatchedMsgAt"`
	MatchedMsgs        []*SearchMatchedMessage `json:"matchedMsgs"`
}

type SearchMatchedMessage struct {
	MessageID string    `json:"msgId"`
	Snippet   string    `json:"snippet"`
	CreatedAt time.Time `json:"createdAt"`
}

// SearchChats requires a privileged Bot and searches records visible to UserID.
func (c *Client) SearchChats(ctx context.Context, request SearchChatsRequest) (*SearchChatsResponse, error) {
	var response SearchChatsResponse
	if err := c.post(ctx, "message.searchChats", request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
