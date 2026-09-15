package run

import (
	"context"
	"testing"
)

func TestStopOnlyCancelsMatchingMessage(t *testing.T) {
	m := NewManager()
	m.Prepare("topic", "old")
	old := m.Begin(context.Background(), "topic", "old")
	defer m.Finish(old)
	m.Prepare("topic", "new")
	current := m.Begin(context.Background(), "topic", "new")
	defer m.Finish(current)
	if m.CancelMessage("topic", "old") || current.Context.Err() != nil {
		t.Fatal("old stop canceled new reply")
	}
	if !m.CancelMessage("topic", "new") || current.Context.Err() == nil {
		t.Fatal("matching stop did not cancel reply")
	}
}

func TestStopQueuedMessage(t *testing.T) {
	m := NewManager()
	m.Prepare("topic", "queued")
	m.CancelMessage("topic", "queued")
	if m.ShouldRun("topic", "queued") {
		t.Fatal("stopped queued message is still runnable")
	}
	h := m.Begin(context.Background(), "topic", "queued")
	if h.Context.Err() == nil {
		t.Fatal("queued stopped message was started")
	}
}

func TestStopBeforeMessageDelivery(t *testing.T) {
	m := NewManager()
	m.CancelMessage("user:83", "late-message")
	m.Prepare("user:83", "late-message")
	if m.ShouldRun("user:83", "late-message") {
		t.Fatal("out-of-order stop was lost")
	}
	h := m.Begin(context.Background(), "user:83", "late-message")
	if h.Context.Err() == nil {
		t.Fatal("stopped message started")
	}
	m.Prepare("user:83", "next-message")
	if !m.ShouldRun("user:83", "next-message") {
		t.Fatal("stop affected another message")
	}
}

func TestSkipMatchesReplyID(t *testing.T) {
	m := NewManager()
	m.Prepare("topic", "user-message")
	h := m.Begin(context.Background(), "topic", "user-message")
	defer m.Finish(h)
	m.BindReply(h, "reply")
	if m.CancelThinking("topic", "old-reply") || h.ThinkingContext.Err() != nil {
		t.Fatal("delayed skip affected newer reply")
	}
	if !m.CancelThinking("topic", "reply") || h.ThinkingContext.Err() == nil || h.Context.Err() != nil {
		t.Fatal("skip must cancel only reasoning")
	}
}

func TestSkipBeforeFirstHTTPResponse(t *testing.T) {
	m := NewManager()
	m.Prepare("topic", "user-message")
	h := m.Begin(context.Background(), "topic", "user-message")
	defer m.Finish(h)
	m.CancelThinking("topic", "reply")
	if h.ThinkingContext.Err() != nil {
		t.Fatal("canceled before knowing the reply ID")
	}
	m.BindReply(h, "reply")
	if h.ThinkingContext.Err() == nil || h.Context.Err() != nil {
		t.Fatal("pending skip did not bind to the reply")
	}
}

func TestOldFinishAndBindingCannotAffectNewGeneration(t *testing.T) {
	m := NewManager()
	m.Prepare("topic", "old")
	old := m.Begin(context.Background(), "topic", "old")
	m.Prepare("topic", "new")
	current := m.Begin(context.Background(), "topic", "new")
	defer m.Finish(current)
	m.Finish(old)
	m.BindReply(old, "old-reply")
	m.CancelThinking("topic", "old-reply")
	if current.Context.Err() != nil || current.ThinkingContext.Err() != nil {
		t.Fatal("stale handle affected new generation")
	}
}
