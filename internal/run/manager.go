package run

import (
	"context"
	"sync"
)

type Handle struct {
	TopicID         string
	Generation      uint64
	Context         context.Context
	ThinkingContext context.Context
}

type activeRun struct {
	generation     uint64
	userMessageID  string
	ctx            context.Context
	cancel         context.CancelFunc
	thinkingCtx    context.Context
	thinkingCancel context.CancelFunc
}

type Manager struct {
	mu      sync.RWMutex
	nextID  uint64
	runs    map[string]*activeRun
	desired map[string]string
}

func NewManager() *Manager {
	return &Manager{runs: make(map[string]*activeRun), desired: make(map[string]string)}
}

// Prepare records the newest message accepted for a topic and immediately
// cancels any active run. A queued older message will receive a canceled
// context when it later calls Begin.
func (m *Manager) Prepare(topicID, userMessageID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.desired[topicID] = userMessageID
	if old := m.runs[topicID]; old != nil {
		old.cancel()
	}
}

func (m *Manager) ShouldRun(topicID, userMessageID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	desired, ok := m.desired[topicID]
	return !ok || desired == userMessageID
}

func (m *Manager) Begin(parent context.Context, topicID, userMessageID string) Handle {
	m.mu.Lock()
	defer m.mu.Unlock()
	if desired, ok := m.desired[topicID]; ok && desired != userMessageID {
		ctx, cancel := context.WithCancel(parent)
		cancel()
		return Handle{TopicID: topicID, Context: ctx, ThinkingContext: ctx}
	}
	if old := m.runs[topicID]; old != nil {
		old.cancel()
	}
	m.nextID++
	ctx, cancel := context.WithCancel(parent)
	thinkingCtx, thinkingCancel := context.WithCancel(ctx)
	active := &activeRun{
		generation: m.nextID, userMessageID: userMessageID,
		ctx: ctx, cancel: cancel, thinkingCtx: thinkingCtx, thinkingCancel: thinkingCancel,
	}
	m.runs[topicID] = active
	return Handle{TopicID: topicID, Generation: active.generation, Context: ctx, ThinkingContext: thinkingCtx}
}

func (m *Manager) Finish(handle Handle) {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.runs[handle.TopicID]
	if active == nil || active.generation != handle.Generation {
		return
	}
	active.cancel()
	delete(m.runs, handle.TopicID)
	if m.desired[handle.TopicID] == active.userMessageID {
		delete(m.desired, handle.TopicID)
	}
}

func (m *Manager) Cancel(topicID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.runs[topicID]
	m.desired[topicID] = ""
	if active == nil {
		return false
	}
	active.cancel()
	return true
}

func (m *Manager) Forget(topicID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.desired, topicID)
}

func (m *Manager) CancelThinking(topicID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.runs[topicID]
	if active == nil {
		return false
	}
	active.thinkingCancel()
	return true
}
