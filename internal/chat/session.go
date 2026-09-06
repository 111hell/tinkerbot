// Package chat owns sessions and a shared, channel-independent agent runtime.
package chat

import (
	"context"
	"github.com/111hell/tinker/agent"
	"myagent/internal/toolscope"
)

// Session binds a conversation ID to the shared service.
type Session struct {
	service *Service
	id      string
	tools   []string
}

func (s *Service) Session(id string, tools ...string) *Session {
	return &Session{service: s, id: id, tools: append([]string(nil), tools...)}
}

func (s *Session) Reset(ctx context.Context) error { return s.service.Reset(ctx, s.id) }

func (s *Session) Reply(ctx context.Context, input string, emit func(string) error) error {
	return s.service.Run(toolscope.WithTools(ctx, s.tools), s.id, input, func(event agent.Event) error {
		if event.Type == agent.MessageDelta && event.Delta.Text != "" {
			return emit(event.Delta.Text)
		}
		return nil
	})
}
