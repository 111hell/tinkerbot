package siu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/111hell/tinker/agent"
	"github.com/111hell/tinkerbot/internal/conversation"
)

const (
	ListMessagesName   = "siu_list_messages"
	SearchContactsName = "siu_search_contacts"
	SearchChatsName    = "siu_search_chats"
	QueryUserName      = "siu_query_user"
	SendPhotoName      = "siu_send_photo"
	EditMessageName    = "siu_edit_message"
	RevokeMessageName  = "siu_revoke_message"
	SendEventName      = "siu_send_event"
)

// newTool keeps argument decoding and trusted conversation identity at the
// boundary. Conversation ownership and message recipients come from context;
// explicit profile lookup selectors may come from model arguments.
func newTool[A any](name, description, schema string, execute func(context.Context, conversation.Current, A) (any, error)) agent.Tool {
	return agent.Tool{
		Name: name, Description: description, InputSchema: schema,
		Execute: func(ctx context.Context, arguments string) (string, error) {
			current, ok := conversation.FromContext(ctx)
			if !ok || current.UserID == 0 {
				return "", fmt.Errorf("current SIU conversation is unavailable")
			}
			var args A
			decoder := json.NewDecoder(strings.NewReader(arguments))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&args); err != nil {
				return "", fmt.Errorf("invalid %s arguments: %w", name, err)
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				return "", fmt.Errorf("expected a single JSON argument object")
			}
			result, err := execute(ctx, current, args)
			if err != nil {
				return "", fmt.Errorf("%s: %w", name, err)
			}
			data, err := json.Marshal(result)
			return string(data), err
		},
	}
}

func requirePrivate(current conversation.Current) error {
	if current.GroupID != 0 {
		return fmt.Errorf("this tool is available only in private SIU chats")
	}
	return nil
}
