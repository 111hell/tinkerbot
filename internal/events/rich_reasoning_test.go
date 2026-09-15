package events

import (
	"encoding/json"
	"testing"

	"github.com/111hell/tinker/agent"
	"github.com/111hell/tinkerbot/internal/siu"
)

func TestRichReasoningCumulativeRowsAndToolLifecycle(t *testing.T) {
	var r richReasoning
	visible := map[int]siu.RichReasoning{}
	apply := func(event agent.Event) {
		t.Helper()
		for _, line := range r.update(event) {
			var row siu.RichReasoning
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				t.Fatal(err)
			}
			if row.ID <= 0 {
				t.Fatalf("non-integer row identity: %s", line)
			}
			if previous, exists := visible[row.ID]; exists && !row.Replace {
				t.Fatalf("existing row lacks replace=true: previous=%+v update=%s", previous, line)
			}
			visible[row.ID] = row
		}
	}
	apply(agent.Event{Type: agent.MessageDelta, Delta: agent.Delta{Reasoning: "我先查"}})
	apply(agent.Event{Type: agent.MessageDelta, Delta: agent.Delta{Reasoning: "联系人"}})
	if visible[1].Payload != "我先查联系人" {
		t.Fatalf("reasoning overwritten: %+v", visible)
	}
	call := agent.ToolCall{ID: "call-1", Name: "siu_search_contacts"}
	apply(agent.Event{Type: agent.ToolStarted, ToolCall: call})
	if visible[3].ParentID != 2 || visible[3].Payload != "正在执行" {
		t.Fatalf("missing tool hierarchy: %+v", visible)
	}
	apply(agent.Event{Type: agent.ToolCompleted, ToolCall: call, Message: agent.Message{Text: `{"error":"permission denied"}`}})
	if visible[3].Payload != "执行失败" {
		t.Fatalf("failed tool shown as success: %+v", visible)
	}
	apply(agent.Event{Type: agent.MessageDelta, Delta: agent.Delta{Reasoning: "查询未成功"}})
	if visible[4].Payload != "查询未成功" || visible[1].Payload != "我先查联系人" {
		t.Fatalf("phases mixed: %+v", visible)
	}
	final := r.snapshot()
	if len(final) != 4 {
		t.Fatalf("final contains incremental updates: %v", final)
	}
	for _, line := range final {
		var row siu.RichReasoning
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		if row.Replace {
			t.Fatalf("final row must not be a replacement: %s", line)
		}
		want := visible[row.ID]
		want.Replace = false
		if row != want {
			t.Fatalf("stream and final disagree: %s", line)
		}
	}
}
