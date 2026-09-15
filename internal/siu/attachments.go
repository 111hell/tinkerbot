package siu

import "encoding/json"

type MessageAttachment struct {
	Images     []*AttachmentImage `json:"images,omitempty"`
	Videos     []*AttachmentVideo `json:"videos,omitempty"`
	MediaStack []*AttachmentMedia `json:"media_stack,omitempty"`
	File       *AttachmentFile    `json:"file,omitempty"`
	FileStack  []*AttachmentFile  `json:"file_stack,omitempty"`
	// The Bot API document does not define these payloads' internal fields.
	Voice  json.RawMessage `json:"voice,omitempty"`
	Custom json.RawMessage `json:"custom,omitempty"`
}

type AttachmentImage struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	Size     uint32 `json:"size"`
	Width    uint32 `json:"width"`
	Height   uint32 `json:"height"`
}

type AttachmentVideo struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	Size     uint32 `json:"size"`
	Width    uint32 `json:"width"`
	Height   uint32 `json:"height"`
	Duration uint32 `json:"duration"`
}

type AttachmentMedia struct {
	Type  string           `json:"type"`
	Image *AttachmentImage `json:"image,omitempty"`
	Video *AttachmentVideo `json:"video,omitempty"`
}

type AttachmentFile struct {
	ID            string          `json:"id"`
	Filename      string          `json:"filename"`
	URL           string          `json:"url"`
	Size          uint32          `json:"size"`
	ContentType   string          `json:"content_type"`
	ContentMD5    string          `json:"content_md5"`
	ThumbnailFile *AttachmentFile `json:"thumbnail_file,omitempty"`
}

// RichReasoning is one JSONL row. Updates to an existing integer ID carry
// cumulative text and Replace=true; ParentID nests a row under another.
type RichReasoning struct {
	Type     string `json:"type"`
	ID       int    `json:"id"`
	Payload  string `json:"payload"`
	ParentID int    `json:"parent_id,omitempty"`
	Replace  bool   `json:"replace,omitempty"`
}

func (r RichReasoning) JSONLine() string {
	data, _ := json.Marshal(r)
	return string(data)
}
