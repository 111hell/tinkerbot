package conversation

import "context"

type Current struct {
	UserID  uint64
	TopicID string
}

type currentKey struct{}

func WithCurrent(ctx context.Context, current Current) context.Context {
	return context.WithValue(ctx, currentKey{}, current)
}

func FromContext(ctx context.Context) (Current, bool) {
	current, ok := ctx.Value(currentKey{}).(Current)
	return current, ok
}
