// Package channel defines the boundary between message transports and the agent.
package channel

import "context"

type Channel interface {
	Run(context.Context) error
}

// Session belongs to one conversation in the shared runtime.
type Session interface {
	Reply(ctx context.Context, input string, emit func(string) error) error
	Reset(context.Context) error
}
