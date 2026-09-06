package siu

const (
	EventTypeUserMessage    = "user_message"
	EventTypeGroupMessage   = "group_message"
	EventTypeEditMessage    = "edit_message"
	EventTypeForkTopic      = "bot:fork_topic"
	EventTypeDeleteTopic    = "bot:delete_topic"
	EventTypeUserEnterChat  = "bot:user_enter_chat"
	EventTypeUserLeaveChat  = "bot:user_leave_chat"
	EventTypeSkipThinking   = "bot:skip_thinking"
	EventTypeStopResponding = "bot:stop_responding"
)

type Event struct {
	ID        string     `json:"id"`
	Version   string     `json:"version"`
	Timestamp int64      `json:"timestamp"`
	EventType string     `json:"event_type"`
	Data      *EventData `json:"data"`
}

type EventData struct {
	User     *User      `json:"user,omitempty"`
	Group    *Group     `json:"group,omitempty"`
	Message  *Message   `json:"message,omitempty"`
	Messages []*Message `json:"messages,omitempty"`
	Topic    *Topic     `json:"topic,omitempty"`
	Extra    *string    `json:"extra,omitempty"`
}

type User struct {
	ID         uint64 `json:"id"`
	ExternalID string `json:"external_id"`
	Nickname   string `json:"nickname"`
	AvatarURL  string `json:"avatar_url"`
}

type Group struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type Topic struct {
	TopicID string `json:"topic_id"`
}

type Message struct {
	MessageID    string `json:"message_id"`
	MessageType  string `json:"message_type"`
	UserID       uint64 `json:"user_id"`
	ToUserID     uint64 `json:"to_user_id,omitempty"`
	ToGroupID    uint64 `json:"to_group_id,omitempty"`
	ExternalID   string `json:"external_id,omitempty"`
	ToExternalID string `json:"to_external_id,omitempty"`
	Text         string `json:"text"`
	Attachment   any    `json:"attachment"`
	Payload      any    `json:"payload,omitempty"`
	TopicID      string `json:"topic_id,omitempty"`
}
