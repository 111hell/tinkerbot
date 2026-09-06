package chat

import (
	"context"
	"slices"
	"sync"

	"github.com/111hell/tinker/agent"
)

type MemoryStore struct {
	mu       sync.Mutex
	messages map[string][]agent.Message
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{messages: make(map[string][]agent.Message)} }

func clone(messages []agent.Message) []agent.Message {
	result := slices.Clone(messages)
	for i := range result {
		result[i].ToolCalls = slices.Clone(result[i].ToolCalls)
	}
	return result
}

func (s *MemoryStore) Load(ctx context.Context, id string) ([]agent.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	messages, ok := s.messages[id]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(messages), nil
}

func (s *MemoryStore) Save(ctx context.Context, id string, messages []agent.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[id] = clone(messages)
	return nil
}

func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.messages, id)
	return nil
}
