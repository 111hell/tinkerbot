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
	"time"
)

type Client struct {
	baseURL *url.URL
	token   string
	http    *http.Client
}

type APIError struct {
	StatusCode int
	Code       string
	Message    string
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
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("SIU base URL must be an absolute HTTP(S) URL without credentials, query or fragment")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("SIU bot token is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 40 * time.Second}
	}
	transport := *httpClient
	redirect := transport.CheckRedirect
	transport.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// X-Api-Key must never follow a redirect to another origin.
		if req.URL.Scheme != via[0].URL.Scheme || req.URL.Host != via[0].URL.Host {
			return http.ErrUseLastResponse
		}
		if redirect != nil {
			return redirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("too many SIU redirects")
		}
		return nil
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, "/api/v1") {
		parsed.Path = path.Join(parsed.Path, "api/v1")
	}
	return &Client{baseURL: parsed, token: token, http: &transport}, nil
}

type envelope[T any] struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    *T     `json:"data"`
}

func (c *Client) endpoint(method string) *url.URL {
	u := *c.baseURL
	u.Path = path.Join(u.Path, "bot", method)
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
	req.Header.Set("X-Api-Key", c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call SIU API: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return fmt.Errorf("read SIU response: %w", err)
	}
	if len(body) > 8<<20 {
		return fmt.Errorf("SIU response exceeds 8 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure envelope[json.RawMessage]
		if json.Unmarshal(body, &failure) == nil && failure.Code != "" && failure.Code != "SUCCESS" {
			return &APIError{StatusCode: resp.StatusCode, Code: failure.Code, Message: failure.Message}
		}
		return &APIError{StatusCode: resp.StatusCode, Message: resp.Status}
	}
	wrapper := envelope[json.RawMessage]{}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return fmt.Errorf("decode SIU response: %w", err)
	}
	if wrapper.Code != "SUCCESS" {
		return &APIError{StatusCode: resp.StatusCode, Code: wrapper.Code, Message: wrapper.Message}
	}
	if out == nil {
		return nil
	}
	if wrapper.Data == nil {
		return fmt.Errorf("SIU API returned SUCCESS without response data")
	}
	if err := json.Unmarshal(*wrapper.Data, out); err != nil {
		return fmt.Errorf("decode SIU response data: %w", err)
	}
	return nil
}
