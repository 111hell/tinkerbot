package events

import (
	"context"
	"fmt"

	"myagent/internal/conversation"
	"myagent/internal/run"
	"myagent/internal/siu"
)

type EventScheduler interface {
	Submit(context.Context, string, *siu.Event) error
}

type Dispatcher struct {
	runs      *run.Manager
	scheduler EventScheduler
}

func NewDispatcher(runs *run.Manager, scheduler EventScheduler) *Dispatcher {
	return &Dispatcher{runs: runs, scheduler: scheduler}
}

func (d *Dispatcher) Dispatch(ctx context.Context, event *siu.Event) error {
	if event == nil {
		return fmt.Errorf("event is required")
	}
	if event.EventType == siu.EventTypeGroupMessage ||
		event.EventType == siu.EventTypeEditMessage ||
		event.EventType == siu.EventTypeUserEnterChat ||
		event.EventType == siu.EventTypeUserLeaveChat {
		return nil
	}
	topicID := conversation.TopicID(event)
	if topicID == "" {
		return fmt.Errorf("event %q has no topic ID", event.EventType)
	}
	switch event.EventType {
	case siu.EventTypeUserMessage:
		if event.Data == nil || event.Data.Message == nil {
			return fmt.Errorf("message data is required for event %q", event.EventType)
		}
		if event.Data.Message.MessageID == "" {
			return fmt.Errorf("message_id is required for event %q", event.EventType)
		}
		d.runs.Prepare(topicID, event.Data.Message.MessageID)
	case siu.EventTypeDeleteTopic:
		d.runs.Cancel(topicID)
	case siu.EventTypeStopResponding:
		d.runs.Cancel(topicID)
		return nil
	case siu.EventTypeSkipThinking:
		d.runs.CancelThinking(topicID)
		return nil
	}
	return d.scheduler.Submit(ctx, topicID, event)
}
