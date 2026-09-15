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
