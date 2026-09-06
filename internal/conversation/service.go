package conversation

import (
	"context"
	"errors"
	"fmt"

	"github.com/111hell/tinker/agent"

	"myagent/internal/chat"
	"myagent/internal/siu"
	"myagent/internal/storage"
)

type SIUHistory interface {
	ListMessages(ctx context.Context, userID uint64, beforeMessageID string, limit int) (*siu.ListMessagesResponse, error)
}

type Service struct {
	store   chat.Store
	history SIUHistory
}

func NewService(store chat.Store, history SIUHistory) *Service {
	return &Service{store: store, history: history}
}

func (s *Service) LoadHistory(ctx context.Context, key Key, currentMessageID string) ([]agent.Message, error) {
	messages, err := s.store.Load(ctx, key.TopicID)
	if err == nil {
		return messages, nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("load conversation: %w", err)
	}

	page, err := s.history.ListMessages(ctx, key.UserID, "", 100)
	if err != nil {
		return nil, fmt.Errorf("load SIU private history: %w", err)
	}
	messages = toAgentHistory(key, page.Messages, currentMessageID)
	if err := s.SaveHistory(ctx, key, messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *Service) SaveHistory(ctx context.Context, key Key, messages []agent.Message) error {
	if err := s.store.Save(ctx, key.TopicID, messages); err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}
	return nil
}

func (s *Service) ImportFork(ctx context.Context, key Key, messages []*siu.Message) error {
	if _, err := s.store.Load(ctx, key.TopicID); err == nil {
		return nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("load fork conversation: %w", err)
	}
	history := make([]agent.Message, 0, len(messages))
	for _, message := range messages {
		if message != nil {
			history = append(history, historyMessage(key, message.UserID, message.Text))
		}
	}
	return s.SaveHistory(ctx, key, history)
}

func toAgentHistory(key Key, newestFirst []*siu.HistoryMessage, currentMessageID string) []agent.Message {
	start := 0
	if currentMessageID != "" {
		start = len(newestFirst)
		for i, message := range newestFirst {
			if message != nil && message.MessageID == currentMessageID {
				start = i + 1
				break
			}
		}
	}
	messages := make([]agent.Message, 0, len(newestFirst)-start)
	for i := len(newestFirst) - 1; i >= start; i-- {
		message := newestFirst[i]
		if message == nil || message.DeletedAt != nil || message.TopicID != key.TopicID {
			continue
		}
		messages = append(messages, historyMessage(key, message.FromUserID, message.Text))
	}
	return messages
}

func historyMessage(key Key, fromUserID uint64, text string) agent.Message {
	role := agent.AssistantRole
	if fromUserID == key.UserID {
		role = agent.UserRole
	}
	return agent.Message{Role: role, Text: text}
}

func (s *Service) DeleteTopic(ctx context.Context, topicID string) error {
	if err := s.store.Delete(ctx, topicID); err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	return nil
}
