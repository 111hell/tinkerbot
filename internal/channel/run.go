package channel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// RunAll isolates channel exits and failures. Parent cancellation stops all
// channels; the shared store remains open until every channel has cleaned up.
func RunAll(ctx context.Context, channels ...Channel) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, len(channels))
	for _, ch := range channels {
		go func() {
			err := ch.Run(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("channel stopped", "channel", fmt.Sprintf("%T", ch), "error", err)
			}
			results <- err
		}()
	}
	var first error
	for range channels {
		if err := <-results; err != nil && !errors.Is(err, context.Canceled) && first == nil {
			first = err
		}
	}
	if first != nil {
		return first
	}
	return ctx.Err()
}
