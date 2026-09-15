package conversation

import (
	"fmt"

	"github.com/111hell/tinkerbot/internal/siu"
)

type Key struct {
	TopicID string
	UserID  uint64
	GroupID uint64
}

// ID is an internal scheduling/storage key, never an API topic_id. Existing
// Topic keys stay unchanged so persisted sessions remain usable.
func (k Key) ID() string {
	if k.GroupID != 0 {
		return fmt.Sprintf("group:%d", k.GroupID)
	}
	if k.TopicID != "" {
		return k.TopicID
	}
	return fmt.Sprintf("user:%d", k.UserID)
}

func KeyFromEvent(event *siu.Event, botID uint64) (Key, error) {
	if event == nil || event.Data == nil {
		return Key{}, fmt.Errorf("event data is required")
	}
	topicID := TopicID(event)
	if topicID == "" && (event.EventType == siu.EventTypeForkTopic || event.EventType == siu.EventTypeDeleteTopic) {
		return Key{}, fmt.Errorf("topic_id is required for event %q", event.EventType)
	}

	key := Key{TopicID: topicID}
	if event.Data.User != nil {
		key.UserID = event.Data.User.ID
	}
	if message := event.Data.Message; message != nil {
		key.UserID = message.UserID
		key.GroupID = message.ToGroupID
		if event.EventType == siu.EventTypeEditMessage && key.GroupID == 0 {
			switch {
			case botID == 0:
				return Key{}, fmt.Errorf("bot identity is required for private edit events")
			case message.UserID == botID:
				key.UserID = message.ToUserID
			case message.ToUserID != botID:
				return Key{}, fmt.Errorf("edited message does not belong to this bot")
			}
		}
	}
	if key.UserID == 0 {
		return Key{}, fmt.Errorf("user_id is required for topic %q", topicID)
	}
	if key.GroupID != 0 && topicID != "" {
		return Key{}, fmt.Errorf("group messages cannot have a topic_id")
	}
	if event.EventType == siu.EventTypeGroupMessage && key.GroupID == 0 {
		return Key{}, fmt.Errorf("to_group_id is required for group messages")
	}
	if event.EventType == siu.EventTypeUserMessage && key.GroupID != 0 {
		return Key{}, fmt.Errorf("private messages cannot have to_group_id")
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
