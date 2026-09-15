package events

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/111hell/tinker/agent"
)

const streamChunkRunes = 48
const streamChunkInterval = 300 * time.Millisecond

// streamBuffer coalesces model tokens without reordering reasoning and text.
// The age limit is checked on incoming events; boundaries always flush.
type streamBuffer struct {
	pending   strings.Builder
	reasoning bool
	since     time.Time
	emit      func(agent.Event) error
}

func (b *streamBuffer) flush() error {
	if b.pending.Len() == 0 {
		return nil
	}
	delta := agent.Delta{}
	if b.reasoning {
		delta.Reasoning = b.pending.String()
	} else {
		delta.Text = b.pending.String()
	}
	b.pending.Reset()
	return b.emit(agent.Event{Type: agent.MessageDelta, Delta: delta})
}

func (b *streamBuffer) add(text string, reasoning bool) error {
	if text == "" {
		return nil
	}
	if b.pending.Len() > 0 && b.reasoning != reasoning {
		if err := b.flush(); err != nil {
			return err
		}
	}
	if b.pending.Len() == 0 {
		b.since = time.Now()
		b.reasoning = reasoning
	}
	b.pending.WriteString(text)
	if utf8.RuneCountInString(b.pending.String()) >= streamChunkRunes || time.Since(b.since) >= streamChunkInterval {
		return b.flush()
	}
	return nil
}

func (b *streamBuffer) accept(event agent.Event) error {
	if event.Type != agent.MessageDelta {
		if err := b.flush(); err != nil {
			return err
		}
		return b.emit(event)
	}
	if err := b.add(event.Delta.Reasoning, true); err != nil {
		return err
	}
	return b.add(event.Delta.Text, false)
}
