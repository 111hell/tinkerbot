package conversation

import (
	"fmt"

	"github.com/111hell/tinkerbot/internal/siu"
)

type Key struct {
	TopicID string
	UserID  uint64
}

func KeyFromEvent(event *siu.Event) (Key, error) {
	if event == nil || event.Data == nil {
		return Key{}, fmt.Errorf("event data is required")
	}
	topicID := TopicID(event)
	if topicID == "" {
		return Key{}, fmt.Errorf("topic_id is required for event %q", event.EventType)
	}

	key := Key{TopicID: topicID}
	if event.Data.User != nil {
		key.UserID = event.Data.User.ID
	}
	if message := event.Data.Message; message != nil {
		key.UserID = message.UserID
	}
	if key.UserID == 0 {
		return Key{}, fmt.Errorf("user_id is required for topic %q", topicID)
	}
	return key, nil
}

func TopicID(event *siu.Event) string {
	if event == nil || event.Data == nil {
		return ""
	}
	if event.Data.Topic != nil && event.Data.Topic.TopicID != "" {
		return event.Data.Topic.TopicID
	}
	if event.Data.Message != nil {
		return event.Data.Message.TopicID
	}
	return ""
}
