package events

import (
	"encoding/json"

	"github.com/111hell/tinker/agent"
	"github.com/111hell/tinkerbot/internal/siu"
)

// richReasoning keeps the latest visible rows, independently of wire updates.
// IDs are local to a single bot reply and text updates contain cumulative text.
type richReasoning struct {
	rows       []siu.RichReasoning
	thinkingID int
	tools      map[string]int
}

func (r *richReasoning) add(parent int, text string) siu.RichReasoning {
	row := siu.RichReasoning{Type: "text", ID: len(r.rows) + 1, ParentID: parent, Payload: text}
	r.rows = append(r.rows, row)
	return row
}

func (r *richReasoning) update(event agent.Event) []string {
	var changed []siu.RichReasoning
	switch event.Type {
	case agent.MessageDelta:
		if text := event.Delta.Reasoning; text != "" {
			if r.thinkingID == 0 {
				row := r.add(0, text)
				r.thinkingID = row.ID
				changed = append(changed, row)
			} else {
				row := &r.rows[r.thinkingID-1]
				row.Payload += text
				row.Replace = true
				changed = append(changed, *row)
			}
		}
		if event.Delta.Text != "" {
			r.thinkingID = 0
		}
	case agent.ToolStarted:
		r.thinkingID = 0
		parent := r.add(0, "调用工具："+event.ToolCall.Name)
		child := r.add(parent.ID, "正在执行")
		if r.tools == nil {
			r.tools = make(map[string]int)
		}
		r.tools[event.ToolCall.ID] = child.ID
		changed = append(changed, parent, child)
	case agent.ToolCompleted:
		if id := r.tools[event.ToolCall.ID]; id != 0 {
			row := &r.rows[id-1]
			row.Payload = "已返回结果"
			row.Replace = true
			var result struct {
				Error json.RawMessage `json:"error"`
			}
			if json.Unmarshal([]byte(event.Message.Text), &result) == nil && len(result.Error) > 0 && string(result.Error) != "null" {
				row.Payload = "执行失败"
			}
			changed = append(changed, *row)
			delete(r.tools, event.ToolCall.ID)
		}
	}
	var lines []string
	for _, row := range changed {
		lines = append(lines, row.JSONLine())
	}
	return lines
}

func (r *richReasoning) snapshot() []string {
	var lines []string
	for _, row := range r.rows {
		row.Replace = false // Finish supplies a fresh, collapsed list.
		lines = append(lines, row.JSONLine())
	}
	return lines
}
