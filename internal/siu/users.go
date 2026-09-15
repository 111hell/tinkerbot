package siu

import "context"

type QueryUserRequest struct {
	UserID   uint64 `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
}

type UserInfo struct {
	ID        uint64 `json:"id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// QueryUser requires a privileged Bot. UserID takes precedence over Username.
func (c *Client) QueryUser(ctx context.Context, request QueryUserRequest) (*UserInfo, error) {
	var response UserInfo
	if err := c.post(ctx, "user.queryUser", request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

type SearchContactsRequest struct {
	UserID uint64 `json:"user_id"`
	Query  string `json:"query"`
	Limit  uint32 `json:"limit"`
}

type SearchContactsResponse struct {
	Results []*SearchContactResult `json:"results"`
}

type SearchContactResult struct {
	User         *ContactUser `json:"user"`
	DisplayName  string       `json:"displayName"`
	MatchedName  string       `json:"matchedName"`
	MatchedField string       `json:"matchedField"`
}

type ContactUser struct {
	ID          string         `json:"id"`
	Username    string         `json:"username"`
	Nickname    string         `json:"nickname"`
	AvatarURL   string         `json:"avatarUrl"`
	Gender      string         `json:"gender"`
	Deleted     bool           `json:"deleted"`
	Birth       string         `json:"birth"`
	Region      *ContactRegion `json:"region"`
	Bio         string         `json:"bio"`
	AccountType string         `json:"accountType"`
	CreatedAt   string         `json:"createdAt"`
}

type ContactRegion struct {
	CountryCodeAlpha2     string `json:"countryCodeAlpha2"`
	SubdivisionLevel1Code string `json:"subdivisionLevel1Code"`
	SubdivisionLevel2Code string `json:"subdivisionLevel2Code"`
}

func (c *Client) SearchContacts(
	ctx context.Context,
	request SearchContactsRequest,
) (*SearchContactsResponse, error) {
	var response SearchContactsResponse
	if err := c.post(ctx, "user.searchContacts", request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
