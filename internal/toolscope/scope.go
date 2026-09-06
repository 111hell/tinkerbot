// Package toolscope applies trusted per-session tool capabilities to a shared
// agent. It has no knowledge of channels or external user identities.
package toolscope

import (
	"context"
	"fmt"
	"iter"
	"slices"

	"github.com/111hell/tinker/agent"
)

type scopeKey struct{}

func WithTools(ctx context.Context, names []string) context.Context {
	return context.WithValue(ctx, scopeKey{}, slices.Clone(names))
}

func enabled(ctx context.Context, name string) bool {
	names, _ := ctx.Value(scopeKey{}).([]string)
	return slices.Contains(names, name)
}

type Model struct{ agent.Model }

func (m Model) Stream(ctx context.Context, request agent.Request) iter.Seq2[agent.Delta, error] {
	var visible []agent.Tool
	for _, tool := range request.Tools {
		if enabled(ctx, tool.Name) {
			visible = append(visible, tool)
		}
	}
	request.Tools = visible
	return m.Model.Stream(ctx, request)
}

// Guard also checks execution: omitting a schema alone does not prevent the
// model from emitting a tool call by name.
func Guard(tools []agent.Tool) []agent.Tool {
	guarded := slices.Clone(tools)
	for i, tool := range guarded {
		guarded[i].Execute = func(ctx context.Context, args string) (string, error) {
			if !enabled(ctx, tool.Name) {
				return "", fmt.Errorf("tool %q is not enabled for this session", tool.Name)
			}
			return tool.Execute(ctx, args)
		}
	}
	return guarded
}
