package chat

import (
	"context"
	"errors"
	"iter"
	"sync"
	"time"

	"github.com/111hell/tinker/agent"
)

var ErrNotFound = errors.New("session not found")

type Store interface {
	Load(context.Context, string) ([]agent.Message, error)
	Save(context.Context, string, []agent.Message) error
	Delete(context.Context, string) error
}

type Runner interface {
	Run(context.Context, []agent.Message, string) iter.Seq2[agent.Event, error]
}

// Service shares one agent across sessions. IDs are opaque to the runtime;
// adapters own their mapping from external conversations to IDs.
type Service struct {
	runner  Runner
	timeout time.Duration
	store   Store
	mu      sync.Mutex
	locks   map[string]*sessionLock
}

type sessionLock struct {
	gate chan struct{}
	refs int
}

func NewService(runner Runner, timeout time.Duration, store Store) *Service {
	return &Service{runner: runner, timeout: timeout, store: store, locks: make(map[string]*sessionLock)}
}

func (s *Service) acquire(ctx context.Context, id string) (func(), error) {
	if id == "" {
		return nil, errors.New("session ID is required")
	}
	s.mu.Lock()
	l := s.locks[id]
	if l == nil {
		l = &sessionLock{gate: make(chan struct{}, 1)}
		s.locks[id] = l
	}
	l.refs++
	s.mu.Unlock()
	releaseRef := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		l.refs--
		if l.refs == 0 {
			delete(s.locks, id)
		}
	}
	select {
	case l.gate <- struct{}{}:
		return func() { <-l.gate; releaseRef() }, nil
	case <-ctx.Done():
		releaseRef()
		return nil, ctx.Err()
	}
}

// Run serializes the entire turn for a session, including persistence and event
// delivery. Callbacks must not re-enter this service for the same session.
func (s *Service) Run(ctx context.Context, id, input string, emit func(agent.Event) error) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	release, err := s.acquire(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	history, err := s.store.Load(ctx, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	for event, err := range s.runner.Run(ctx, history, input) {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if event.Type == agent.RunCompleted {
			if err := s.store.Save(ctx, id, event.Messages); err != nil {
				return err
			}
			return emit(event)
		}
		if err := emit(event); err != nil {
			return err
		}
	}
	return errors.New("agent run ended without completion")
}

func (s *Service) History(ctx context.Context, id string) ([]agent.Message, error) {
	release, err := s.acquire(ctx, id)
	if err != nil {
		return nil, err
	}
	defer release()
	return s.store.Load(ctx, id)
}

// Initialize seeds an absent session, preserving any existing history.
func (s *Service) Initialize(ctx context.Context, id string, history []agent.Message) error {
	release, err := s.acquire(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	if _, err := s.store.Load(ctx, id); !errors.Is(err, ErrNotFound) {
		return err
	}
	return s.store.Save(ctx, id, history)
}

func (s *Service) Reset(ctx context.Context, id string) error {
	release, err := s.acquire(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	return s.store.Delete(ctx, id)
}
