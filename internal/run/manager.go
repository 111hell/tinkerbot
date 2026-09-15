package run

import (
	"context"
	"sync"
	"time"
)

type Handle struct {
	ConversationID  string
	Generation      uint64
	Context         context.Context
	ThinkingContext context.Context
}

type activeRun struct {
	generation     uint64
	userMessageID  string
	replyMessageID string
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
	stopped map[messageKey]time.Time
	skipped map[messageKey]time.Time
}

type messageKey struct{ conversationID, messageID string }

const controlRetention = 5 * time.Minute

func NewManager() *Manager {
	return &Manager{runs: make(map[string]*activeRun), desired: make(map[string]string), stopped: make(map[messageKey]time.Time), skipped: make(map[messageKey]time.Time)}
}

// Prepare records the newest message accepted for a conversation and immediately
// cancels any active run. A queued older message will receive a canceled
// context when it later calls Begin.
func (m *Manager) Prepare(conversationID, userMessageID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneControls()
	if time.Now().Before(m.stopped[messageKey{conversationID, userMessageID}]) {
		return
	}
	m.desired[conversationID] = userMessageID
	if old := m.runs[conversationID]; old != nil {
		old.cancel()
	}
}

func (m *Manager) ShouldRun(conversationID, userMessageID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if time.Now().Before(m.stopped[messageKey{conversationID, userMessageID}]) {
		return false
	}
	desired, ok := m.desired[conversationID]
	return !ok || desired == userMessageID
}

func (m *Manager) Begin(parent context.Context, conversationID, userMessageID string) Handle {
	m.mu.Lock()
	defer m.mu.Unlock()
	desired, ok := m.desired[conversationID]
	if (ok && desired != userMessageID) || time.Now().Before(m.stopped[messageKey{conversationID, userMessageID}]) {
		ctx, cancel := context.WithCancel(parent)
		cancel()
		return Handle{ConversationID: conversationID, Context: ctx, ThinkingContext: ctx}
	}
	if old := m.runs[conversationID]; old != nil {
		old.cancel()
	}
	m.nextID++
	ctx, cancel := context.WithCancel(parent)
	thinkingCtx, thinkingCancel := context.WithCancel(ctx)
	active := &activeRun{
		generation: m.nextID, userMessageID: userMessageID,
		ctx: ctx, cancel: cancel, thinkingCtx: thinkingCtx, thinkingCancel: thinkingCancel,
	}
	m.runs[conversationID] = active
	return Handle{ConversationID: conversationID, Generation: active.generation, Context: ctx, ThinkingContext: thinkingCtx}
}

func (m *Manager) Finish(handle Handle) {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.runs[handle.ConversationID]
	if active == nil || active.generation != handle.Generation {
		return
	}
	active.cancel()
	delete(m.runs, handle.ConversationID)
	if m.desired[handle.ConversationID] == active.userMessageID {
		delete(m.desired, handle.ConversationID)
	}
}

func (m *Manager) Cancel(conversationID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.runs[conversationID]
	m.desired[conversationID] = ""
	if active == nil {
		return false
	}
	active.cancel()
	return true
}

func (m *Manager) Forget(conversationID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.desired[conversationID] == "" {
		delete(m.desired, conversationID)
	}
}

func (m *Manager) CancelThinking(conversationID, replyMessageID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if replyMessageID == "" {
		return false
	}
	m.pruneControls()
	m.skipped[messageKey{conversationID, replyMessageID}] = time.Now().Add(controlRetention)
	active := m.runs[conversationID]
	if active == nil || active.replyMessageID != replyMessageID {
		return false
	}
	active.thinkingCancel()
	return true
}

// CancelMessage ignores a delayed stop for an older turn in the same conversation.
func (m *Manager) CancelMessage(conversationID, userMessageID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if userMessageID == "" {
		return false
	}
	m.pruneControls()
	m.stopped[messageKey{conversationID, userMessageID}] = time.Now().Add(controlRetention)
	if m.desired[conversationID] == userMessageID {
		m.desired[conversationID] = ""
	}
	active := m.runs[conversationID]
	if active == nil || active.userMessageID != userMessageID {
		return false
	}
	active.cancel()
	return true
}

// BindReply associates SIU's reply ID with the generation that created it.
// A skip can arrive before the first HTTP response returns that ID.
func (m *Manager) BindReply(handle Handle, replyMessageID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.runs[handle.ConversationID]
	if active == nil || active.generation != handle.Generation {
		return
	}
	active.replyMessageID = replyMessageID
	if time.Now().Before(m.skipped[messageKey{handle.ConversationID, replyMessageID}]) {
		active.thinkingCancel()
	}
}

// Called with mu held; no timer goroutine is needed for these short-lived IDs.
func (m *Manager) pruneControls() {
	now := time.Now()
	for key, until := range m.stopped {
		if !now.Before(until) {
			delete(m.stopped, key)
		}
	}
	for key, until := range m.skipped {
		if !now.Before(until) {
			delete(m.skipped, key)
		}
	}
}
