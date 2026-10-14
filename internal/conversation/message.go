package conversation

import (
	"encoding/json"

	"github.com/111hell/tinkerbot/internal/siu"
)

// ImageURLs extracts direct image URLs from message attachments.
func ImageURLs(attachment *siu.MessageAttachment) []string {
	if attachment == nil {
		return nil
	}
	var urls []string
	for _, img := range attachment.Images {
		if img != nil && img.URL != "" {
			urls = append(urls, img.URL)
		}
	}
	for _, media := range attachment.MediaStack {
		if media != nil && media.Image != nil && media.Image.URL != "" {
			urls = append(urls, media.Image.URL)
		}
	}
	return urls
}

// MessageText preserves attachment metadata. If images are attached, their URLs
// are also supplied directly to multimodal models.
func MessageText(text string, attachment *siu.MessageAttachment) string {
	if attachment == nil {
		return text
	}
	data, err := json.Marshal(attachment)
	if err != nil {
		return text
	}
	if len(ImageURLs(attachment)) > 0 {
		return text + "\n[SIU attachment metadata; image contents supplied as multimodal input]\n" + string(data)
	}
	return text + "\n[SIU attachment metadata; media contents have not been read]\n" + string(data)
}
