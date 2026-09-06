package siu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

type Client struct {
	baseURL *url.URL
	token   string
	http    *http.Client
}

type APIError struct {
	Code    string
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("SIU API failed: %s: %s", e.Code, e.Message) }

func IsCode(err error, code string) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == code
}

func NewClient(baseURL, token string, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse SIU base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("SIU base URL must be absolute")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, "/api/v1") {
		parsed.Path = path.Join(parsed.Path, "api/v1")
	}
	return &Client{baseURL: parsed, token: token, http: httpClient}, nil
}

type envelope[T any] struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    *T     `json:"data"`
}

func (c *Client) endpoint(method string) *url.URL {
	u := *c.baseURL
	u.Path = path.Join(u.Path, "bot", c.token, method)
	return &u
}

func (c *Client) get(ctx context.Context, method string, query url.Values, out any) error {
	u := c.endpoint(method)
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("create SIU request: %w", err)
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, method string, input, out any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode SIU request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(method).String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create SIU request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call SIU API: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read SIU response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("SIU API returned %s: %s", resp.Status, string(body))
	}
	wrapper := envelope[json.RawMessage]{}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return fmt.Errorf("decode SIU response: %w", err)
	}
	if wrapper.Code != "SUCCESS" || wrapper.Data == nil {
		return &APIError{Code: wrapper.Code, Message: wrapper.Message}
	}
	if err := json.Unmarshal(*wrapper.Data, out); err != nil {
		return fmt.Errorf("decode SIU response data: %w", err)
	}
	return nil
}
