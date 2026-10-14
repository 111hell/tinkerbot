// Package toolscope applies trusted per-session tool capabilities to a shared
// agent. It has no knowledge of channels or external user identities.
package toolscope

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/111hell/tinker/agent"

	"github.com/111hell/tinkerbot/internal/logging"
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
	return func(yield func(agent.Delta, error) bool) {
		logger := logging.FromContext(ctx)
		started := time.Now()
		logger.InfoContext(ctx, "model request started", "history_messages", len(request.Messages), "visible_tools", len(visible))
		status := "completed"
		defer func() {
			logger.InfoContext(ctx, "model request ended", "status", status, "duration_ms", time.Since(started).Milliseconds())
		}()
		for delta, err := range m.Model.Stream(ctx, request) {
			if err != nil {
				status = "failed"
				if errors.Is(err, context.Canceled) {
					status = "canceled"
				} else {
					logger.ErrorContext(ctx, "model request failed", "error", err)
				}
			}
			if !yield(delta, err) {
				if err == nil {
					status = "consumer_stopped"
				}
				return
			}
		}
	}
}

func ValidateInputSchema(schemaJSON, argsJSON string) error {
	if !json.Valid([]byte(argsJSON)) {
		return errors.New("invalid JSON arguments")
	}
	trimmedSchema := strings.TrimSpace(schemaJSON)
	if trimmedSchema == "" || trimmedSchema == "{}" {
		return nil
	}
	var schema struct {
		Type       string                 `json:"type"`
		Required   []string               `json:"required"`
		Properties map[string]interface{} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(trimmedSchema), &schema); err != nil {
		return nil
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Errorf("tool arguments must be a JSON object: %w", err)
	}
	for _, req := range schema.Required {
		val, exists := args[req]
		if !exists || val == nil {
			return fmt.Errorf("missing required field %q", req)
		}
		if s, ok := val.(string); ok && strings.TrimSpace(s) == "" {
			return fmt.Errorf("required field %q cannot be empty", req)
		}
	}
	return nil
}

// Guard also checks execution: omitting a schema alone does not prevent the
// model from emitting a tool call by name.
func Guard(tools []agent.Tool) []agent.Tool {
	guarded := slices.Clone(tools)
	for i, tool := range guarded {
		guarded[i].Execute = func(ctx context.Context, args string) (string, error) {
			if !enabled(ctx, tool.Name) {
				logging.FromContext(ctx).WarnContext(ctx, "tool execution denied", "tool", tool.Name)
				return "", fmt.Errorf("tool %q is not enabled for this session", tool.Name)
			}
			if err := ValidateInputSchema(tool.InputSchema, args); err != nil {
				logging.FromContext(ctx).WarnContext(ctx, "tool schema validation failed", "tool", tool.Name, "error", err)
				return "", fmt.Errorf("tool %q arguments schema validation failed: %w", tool.Name, err)
			}
			output, err := tool.Execute(ctx, args)
			if err != nil {
				logger := logging.FromContext(ctx)
				if errors.Is(err, context.Canceled) {
					logger.InfoContext(ctx, "tool execution canceled", "tool", tool.Name)
				} else {
					logger.WarnContext(ctx, "tool execution failed", "tool", tool.Name, "error", err)
				}
			}
			return output, err
		}
	}
	return guarded
}
