package conversation

import (
	"encoding/json"

	"github.com/111hell/tinkerbot/internal/siu"
)

// MessageText preserves attachment metadata for the text-only Tinker runtime.
// URLs and filenames describe supplied attachments, not their visual contents.
func MessageText(text string, attachment *siu.MessageAttachment) string {
	if attachment == nil {
		return text
	}
	data, err := json.Marshal(attachment)
	if err != nil {
		return text
	}
	return text + "\n[SIU attachment metadata; media contents have not been read]\n" + string(data)
}
