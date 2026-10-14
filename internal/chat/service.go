package chat

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"sync"
	"time"

	"github.com/111hell/tinker/agent"

	"github.com/111hell/tinkerbot/internal/logging"
)

var (
	ErrNotFound         = errors.New("session not found")
	ErrMaxStepsExceeded = errors.New("agent: maximum tool execution steps exceeded")
)

type Store interface {
	Load(context.Context, string) ([]agent.Message, error)
	Save(context.Context, string, []agent.Message) error
	Delete(context.Context, string) error
}

type Runner interface {
	Run(context.Context, []agent.Message, string) iter.Seq2[agent.Event, error]
}

type MessageRunner interface {
	RunMessage(context.Context, []agent.Message, agent.Message) iter.Seq2[agent.Event, error]
}

type Option func(*Service)

func WithMaxHistoryMessages(max int) Option {
	return func(s *Service) { s.maxHistoryMessages = max }
}

func WithMaxSteps(max int) Option {
	return func(s *Service) { s.maxSteps = max }
}

func WithMemoryCompaction(enabled bool) Option {
	return func(s *Service) { s.memoryCompaction = enabled }
}

// Service shares one agent across sessions. IDs are opaque to the runtime;
// adapters own their mapping from external conversations to IDs.
type Service struct {
	runner             Runner
	timeout            time.Duration
	store              Store
	maxHistoryMessages int
	maxSteps           int
	memoryCompaction   bool
	mu                 sync.Mutex
	locks              map[string]*sessionLock
}

type sessionLock struct {
	gate chan struct{}
	refs int
}

func NewService(runner Runner, timeout time.Duration, store Store, opts ...Option) *Service {
	s := &Service{runner: runner, timeout: timeout, store: store, locks: make(map[string]*sessionLock)}
	for _, opt := range opts {
		opt(s)
	}
	return s
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

// truncateHistory trims history to at most maxMessages, ensuring conversational integrity:
// it seeks forward to the next UserRole message boundary to prevent orphaned tool messages.
func truncateHistory(history []agent.Message, maxMessages int) []agent.Message {
	if maxMessages <= 0 || len(history) <= maxMessages {
		return history
	}
	start := len(history) - maxMessages
	for i := start; i < len(history); i++ {
		if history[i].Role == agent.UserRole {
			return history[i:]
		}
	}
	return history[start:]
}

// compactHistory compacts older history beyond maxMessages into a rolling context summary,
// preserving critical facts and intent while keeping active turns strictly valid.
func compactHistory(history []agent.Message, maxMessages int) []agent.Message {
	if maxMessages <= 0 || len(history) <= maxMessages {
		return history
	}
	start := len(history) - maxMessages
	cut := -1
	for i := start; i < len(history); i++ {
		if history[i].Role == agent.UserRole {
			cut = i
			break
		}
	}
	if cut <= 0 {
		return history[start:]
	}
	dropped := history[:cut]
	summary := buildContextSummary(dropped)
	if summary == "" {
		return history[cut:]
	}
	summaryMsg := agent.Message{
		Role: agent.UserRole,
		Text: fmt.Sprintf("[Conversation Context Summary: Prior discussions compacted for context retention]\n%s\n[End of Context Summary]", summary),
	}
	assistantAck := agent.Message{
		Role: agent.AssistantRole,
		Text: "Understood. I have preserved the prior context and will continue assisting seamlessly.",
	}
	return append([]agent.Message{summaryMsg, assistantAck}, history[cut:]...)
}

func buildContextSummary(dropped []agent.Message) string {
	if len(dropped) == 0 {
		return ""
	}
	var lines []string
	for _, m := range dropped {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		if strings.Contains(text, "[Conversation Context Summary:") {
			lines = append(lines, extractInnerSummary(text)...)
			continue
		}
		if len(text) > 160 {
			text = text[:160] + "..."
		}
		switch m.Role {
		case agent.UserRole:
			lines = append(lines, fmt.Sprintf("- User: %s", text))
		case agent.AssistantRole:
			lines = append(lines, fmt.Sprintf("- Assistant: %s", text))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	return strings.Join(lines, "\n")
}

func extractInnerSummary(text string) []string {
	var lines []string
	scanner := strings.Split(text, "\n")
	for _, l := range scanner {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "- ") {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// Run serializes the entire turn for a session, delegating to RunMessage with a UserMessage.
func (s *Service) Run(ctx context.Context, id, input string, emit func(agent.Event) error) error {
	return s.RunMessage(ctx, id, agent.UserMessage(input), emit)
}

// RunMessage serializes the entire turn for a session, supporting multimodal messages with image URLs.
func (s *Service) RunMessage(ctx context.Context, id string, userMsg agent.Message, emit func(agent.Event) error) (runErr error) {
	logger := logging.FromContext(ctx).With("session_id", id)
	ctx = logging.WithLogger(ctx, logger)
	started := time.Now()
	logger.InfoContext(ctx, "agent run waiting", "input_bytes", len(userMsg.Text), "image_urls", len(userMsg.ImageURLs), "timeout_ms", s.timeout.Milliseconds())
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
	originalHistoryLen := len(history)
	if s.maxHistoryMessages > 0 {
		if s.memoryCompaction {
			history = compactHistory(history, s.maxHistoryMessages)
		} else {
			history = truncateHistory(history, s.maxHistoryMessages)
		}
		if len(history) < originalHistoryLen {
			logger.InfoContext(ctx, "history compacted for context window",
				"original_messages", originalHistoryLen,
				"active_messages", len(history),
				"max_limit", s.maxHistoryMessages,
				"compaction", s.memoryCompaction)
		}
	}
	logger.InfoContext(ctx, "agent run started", "history_messages", len(history), "wait_ms", time.Since(started).Milliseconds())
	firstOutput := false
	toolStarts := make(map[string]time.Time)
	toolStepCount := 0
	maxSteps := s.maxSteps
	if maxSteps <= 0 {
		maxSteps = 20
	}
	var seq iter.Seq2[agent.Event, error]
	if mr, ok := s.runner.(MessageRunner); ok {
		seq = mr.RunMessage(ctx, history, userMsg)
	} else {
		seq = s.runner.Run(ctx, history, userMsg.Text)
	}
	for event, err := range seq {
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
			toolStepCount++
			if toolStepCount > maxSteps {
				logger.WarnContext(ctx, "maximum agent tool steps exceeded", "steps", toolStepCount, "max_steps", maxSteps)
				cancel()
				return ErrMaxStepsExceeded
			}
			toolStarts[event.ToolCall.ID] = time.Now()
			logger.InfoContext(ctx, "agent tool started", "tool", event.ToolCall.Name, "tool_call_id", event.ToolCall.ID, "step", toolStepCount)
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
