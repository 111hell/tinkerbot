package chat

import (
	"context"
	"errors"
	"iter"
	"sync"
	"time"

	"github.com/111hell/tinker/agent"

	"github.com/111hell/tinkerbot/internal/logging"
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
func (s *Service) Run(ctx context.Context, id, input string, emit func(agent.Event) error) (runErr error) {
	logger := logging.FromContext(ctx).With("session_id", id)
	ctx = logging.WithLogger(ctx, logger)
	started := time.Now()
	logger.InfoContext(ctx, "agent run waiting", "input_bytes", len(input), "timeout_ms", s.timeout.Milliseconds())
	defer func() {
		attrs := []any{"duration_ms", time.Since(started).Milliseconds()}
		switch {
		case runErr == nil:
			logger.InfoContext(ctx, "agent run completed", attrs...)
		case errors.Is(runErr, context.Canceled):
			logger.InfoContext(ctx, "agent run canceled", attrs...)
		default:
			logger.ErrorContext(ctx, "agent run failed", append(attrs, "error", runErr)...)
		}
	}()
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
	logger.InfoContext(ctx, "agent run started", "history_messages", len(history), "wait_ms", time.Since(started).Milliseconds())
	firstOutput := false
	toolStarts := make(map[string]time.Time)
	for event, err := range s.runner.Run(ctx, history, input) {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		switch event.Type {
		case agent.MessageDelta:
			if !firstOutput && (event.Delta.Text != "" || event.Delta.Reasoning != "" || event.Delta.ToolCall != nil) {
				firstOutput = true
				logger.InfoContext(ctx, "agent first output", "elapsed_ms", time.Since(started).Milliseconds())
			}
		case agent.ToolStarted:
			toolStarts[event.ToolCall.ID] = time.Now()
			logger.InfoContext(ctx, "agent tool started", "tool", event.ToolCall.Name, "tool_call_id", event.ToolCall.ID)
		case agent.ToolCompleted:
			attrs := []any{"tool", event.ToolCall.Name, "tool_call_id", event.ToolCall.ID, "result_bytes", len(event.Message.Text)}
			if start, ok := toolStarts[event.ToolCall.ID]; ok {
				attrs = append(attrs, "duration_ms", time.Since(start).Milliseconds())
				delete(toolStarts, event.ToolCall.ID)
			}
			logger.InfoContext(ctx, "agent tool completed", attrs...)
		case agent.RunCompleted:
			logger.InfoContext(ctx, "agent generation completed", "input_tokens", event.Usage.InputTokens, "output_tokens", event.Usage.OutputTokens, "reasoning_tokens", event.Usage.ReasoningTokens, "total_tokens", event.Usage.TotalTokens)
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
