package siu

import (
	"context"
	"fmt"

	"github.com/111hell/tinker/agent"
	"github.com/111hell/tinkerbot/internal/conversation"
	siuc "github.com/111hell/tinkerbot/internal/siu"
)

type sendPhotoArgs struct {
	ImageURL     string `json:"image_url,omitempty"`
	FileID       string `json:"file_id,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	Text         string `json:"text,omitempty"`
	Width        uint32 `json:"width,omitempty"`
	Height       uint32 `json:"height,omitempty"`
	PreviewARGB  string `json:"preview_argb,omitempty"`
}

func NewSendPhoto(client *siuc.Client) agent.Tool {
	return newTool(SendPhotoName,
		"Send an existing image to the current SIU conversation when requested; recipient and Topic are supplied automatically. Supply an external image URL or uploaded file ID; width and height default to standard dimensions (800x600) if omitted. Text is an optional caption.",
		`{"type":"object","properties":{"image_url":{"type":"string","description":"Existing external image URL; provide this or file_id."},"file_id":{"type":"string","description":"Existing file uploaded by this Bot; provide this or image_url."},"thumbnail_url":{"type":"string","description":"Optional existing thumbnail URL."},"text":{"type":"string","description":"Optional image caption."},"width":{"type":"integer","minimum":1,"description":"Optional image width in pixels; defaults to 800."},"height":{"type":"integer","minimum":1,"description":"Optional image height in pixels; defaults to 600."},"preview_argb":{"type":"string","description":"Optional preview color in the SIU ARGB format."}},"additionalProperties":false}`,
		func(ctx context.Context, current conversation.Current, args sendPhotoArgs) (any, error) {
			if args.Width == 0 {
				args.Width = 800
			}
			if args.Height == 0 {
				args.Height = 600
			}
			request := siuc.SendPhotoRequest{TopicID: current.TopicID, ImageURL: args.ImageURL, FileID: args.FileID, ThumbnailURL: args.ThumbnailURL, Text: args.Text, Width: args.Width, Height: args.Height, PreviewARGB: args.PreviewARGB}
			if current.GroupID != 0 {
				request.ToGroupID = current.GroupID
			} else {
				request.ToUserID = current.UserID
			}
			return client.SendPhoto(ctx, request)
		})
}

type editMessageArgs struct {
	MessageID string `json:"message_id"`
	Text      string `json:"text"`
}

func NewEditMessage(client *siuc.Client) agent.Tool {
	return newTool(EditMessageName,
		"Replace the text of an existing Bot message in the current conversation when the user requests an edit. Use its actual message ID from context or tool results and the complete replacement text, not a diff. Resolve an ambiguous target before editing. Ordinary new replies do not need this tool.",
		`{"type":"object","properties":{"message_id":{"type":"string","description":"Actual ID of the Bot text message to edit."},"text":{"type":"string","description":"Complete replacement text."}},"required":["message_id","text"],"additionalProperties":false}`,
		func(ctx context.Context, current conversation.Current, args editMessageArgs) (any, error) {
			request := siuc.EditMessageRequest{MessageID: args.MessageID, Text: args.Text}
			if current.GroupID != 0 {
				request.ToGroupID = current.GroupID
			} else {
				request.ToUserID = current.UserID
			}
			return client.EditMessage(ctx, request)
		})
}

type messageArgs struct {
	MessageID string `json:"message_id"`
}

func NewRevokeMessage(client *siuc.Client) agent.Tool {
	return newTool(RevokeMessageName,
		"Revoke a specific existing message in the current conversation when the user requests withdrawal. Use an actual message ID from context or tool results; resolve an ambiguous target first. This removes a message, not a Topic or the Bot's memory. SIU checks revoke permissions; only report success after the call succeeds.",
		`{"type":"object","properties":{"message_id":{"type":"string","description":"Actual ID of the message the user requested to revoke."}},"required":["message_id"],"additionalProperties":false}`,
		func(ctx context.Context, current conversation.Current, args messageArgs) (any, error) {
			request := siuc.RevokeMessageRequest{MessageID: args.MessageID}
			if current.GroupID != 0 {
				request.ToGroupID = current.GroupID
			} else {
				request.ToUserID = current.UserID
			}
			return struct{}{}, client.RevokeMessage(ctx, request)
		})
}

type sendEventArgs struct {
	EventType string `json:"event_type"`
	Data      string `json:"data,omitempty"`
}

func NewSendEvent(client *siuc.Client) agent.Tool {
	return newTool(SendEventName,
		"Send a transient protocol event to the current private-chat user's client only when a supported event type and payload contract are known. Do not invent event types. Success means the event was submitted, not that the client acknowledged receipt or completed an action. This does not save a chat message; ordinary replies, reminders and Bot feature configuration do not use this tool.",
		`{"type":"object","properties":{"event_type":{"type":"string","description":"Exact event type documented by the receiving client."},"data":{"type":"string","description":"Event payload string encoded according to that event contract."}},"required":["event_type"],"additionalProperties":false}`,
		func(ctx context.Context, current conversation.Current, args sendEventArgs) (any, error) {
			if err := requirePrivate(current); err != nil {
				return nil, err
			}
			if args.EventType == "" {
				return nil, fmt.Errorf("event_type is required")
			}
			return struct{}{}, client.SendEvent(ctx, siuc.SendEventRequest{UserID: current.UserID, TopicID: current.TopicID, EventType: args.EventType, Data: args.Data})
		})
}
