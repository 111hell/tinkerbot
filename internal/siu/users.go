package siu

import "context"

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
