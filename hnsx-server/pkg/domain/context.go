// Context helpers — moved from pkg/runtime/context.go in Phase 3.

package domain

import "context"

// sessionIDKey is the unexported context key under which a session ID
// travels from the API layer down through the runtime.
type sessionIDKey struct{}

// WithSessionID returns ctx with the given session ID attached.
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, id)
}

// SessionIDFromContext returns the session ID stored via WithSessionID, or
// an empty string if none was set. Exported because the server's session
// pipeline and the Python worker both need to read it.
func SessionIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(sessionIDKey{}).(string); ok {
		return id
	}
	return ""
}
