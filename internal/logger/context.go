package logger

import "context"

// contextKey is a private type to prevent collisions with other context keys.
type contextKey struct{}

// requestIDKey is the context key for the request ID.
var requestIDKey = contextKey{}

// WithRequestID returns a new context with the given request ID stored.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID extracts the request ID from the context.
// Returns an empty string if no request ID is set.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}
