package siu

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"
)

type UploadMediaRequest struct {
	ContentType   string `json:"content_type,omitempty"`
	ContentLength uint64 `json:"content_length,omitempty"`
	ContentMD5    string `json:"content_md5,omitempty"`
}

type UploadMediaResponse struct {
	FileID    string    `json:"file_id"`
	UploadURL string    `json:"upload_url"`
	ExpiredAt time.Time `json:"expired_at"`
}

// DownloadMedia reads an SIU attachment's presigned URL without Bot credentials.
// Like Agentiu's downloadLimit, both time and bytes are bounded and signed URLs
// are excluded from errors. The caller supplies the remaining per-message budget.
func (c *Client) DownloadMedia(ctx context.Context, fileURL string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("media download budget exhausted")
	}
	u, err := url.Parse(fileURL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("invalid media download URL")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("invalid media download request")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("download SIU media: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download SIU media: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("download SIU media exceeds %d bytes", limit)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read SIU media: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download SIU media exceeds %d bytes", limit)
	}
	return data, nil
}

// UploadMedia allocates a file and its presigned PUT URL; it does not upload bytes.
func (c *Client) UploadMedia(ctx context.Context, request UploadMediaRequest) (*UploadMediaResponse, error) {
	var response UploadMediaResponse
	if err := c.post(ctx, "file.uploadMedia", request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// PutMedia sends bytes using the same metadata used to sign the upload URL.
// It deliberately bypasses the Bot API envelope and authentication header.
func (c *Client) PutMedia(ctx context.Context, uploadURL string, content io.Reader, metadata UploadMediaRequest) error {
	if metadata.ContentLength > math.MaxInt64 {
		return fmt.Errorf("media is too large")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, content)
	if err != nil {
		return fmt.Errorf("invalid media upload URL")
	}
	if metadata.ContentLength > 0 {
		req.ContentLength = int64(metadata.ContentLength)
	}
	if metadata.ContentType != "" {
		req.Header.Set("Content-Type", metadata.ContentType)
	}
	if metadata.ContentMD5 != "" {
		req.Header.Set("Content-MD5", metadata.ContentMD5)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		} // Do not log presigned credentials.
		return fmt.Errorf("upload SIU media: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upload SIU media: HTTP %d", resp.StatusCode)
	}
	_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<20))
	return err
}

// UploadImage performs both upload steps, calculating the size and Base64 MD5.
// Pass the returned FileID to SendPhoto with the image's width and height.
func (c *Client) UploadImage(ctx context.Context, contentType string, data []byte) (*UploadMediaResponse, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("image data is empty")
	}
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	digest := md5.Sum(data) // SIU's upload protocol uses MD5 for content integrity.
	metadata := UploadMediaRequest{ContentType: contentType, ContentLength: uint64(len(data)), ContentMD5: base64.StdEncoding.EncodeToString(digest[:])}
	response, err := c.UploadMedia(ctx, metadata)
	if err != nil {
		return nil, err
	}
	if err := c.PutMedia(ctx, response.UploadURL, bytes.NewReader(data), metadata); err != nil {
		return nil, err
	}
	return response, nil
}
