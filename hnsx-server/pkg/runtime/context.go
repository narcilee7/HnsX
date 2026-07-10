package runtime

import "context"

// sessionIDKey is the unexported context key under which a session ID
// travels from the API layer down through the runtime.
type sessionIDKey struct{}

// SessionIDFromContext returns the session ID stored via WithSessionID, or
// an empty string if none was set. Exported because the server's session
// executor and api layer need to stamp every observation with the same id.
func SessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(sessionIDKey{}).(string); ok {
		return v
	}
	return ""
}

// WithSessionID returns a derived context that carries the given session ID.
// Used by the API layer so all observations emitted by the runner are stamped
// with the same session_id that was registered with the HTTP request.
func WithSessionID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, sessionIDKey{}, id)
}
