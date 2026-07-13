// Package tenant provides tenant identity propagation through context,
// HTTP middleware, and gRPC interceptors.
//
// A zero-value tenant.ID is invalid; use DefaultID when no tenant is supplied.
package tenant

import (
	"context"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ID is a tenant identifier (UUID string). It is intentionally a named type
// to avoid mixing tenant IDs with other strings.
type ID string

// DefaultID is the tenant used when no tenant is explicitly provided. It
// keeps single-tenant deployments working out of the box.
const DefaultID ID = "00000000-0000-0000-0000-000000000000"

// HeaderName is the HTTP header that carries the tenant ID.
const HeaderName = "X-Tenant-ID"

// MetadataKey is the gRPC metadata key that carries the tenant ID.
const MetadataKey = "x-tenant-id"

type contextKey struct{}

// NewContext returns a context with the tenant ID attached.
func NewContext(ctx context.Context, id ID) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the tenant ID from the context, or DefaultID if none.
func FromContext(ctx context.Context) ID {
	if id, ok := ctx.Value(contextKey{}).(ID); ok {
		return id
	}
	return DefaultID
}

// FromContextOK returns the tenant ID and true if one was explicitly attached.
func FromContextOK(ctx context.Context) (ID, bool) {
	id, ok := ctx.Value(contextKey{}).(ID)
	return id, ok
}

// Middleware parses the tenant ID from the X-Tenant-ID header and attaches it
// to the request context. Missing or empty headers default to DefaultID.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := ID(r.Header.Get(HeaderName))
		if id == "" {
			id = DefaultID
		}
		next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), id)))
	})
}

// UnaryServerInterceptor returns a gRPC unary interceptor that extracts the
// tenant ID from incoming metadata and attaches it to the call context.
func UnaryServerInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	ctx = injectFromMetadata(ctx)
	return handler(ctx, req)
}

// StreamServerInterceptor returns a gRPC stream interceptor that extracts the
// tenant ID from incoming metadata and attaches it to the stream context.
func StreamServerInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx := injectFromMetadata(ss.Context())
	return handler(srv, &streamWithContext{ServerStream: ss, ctx: ctx})
}

func injectFromMetadata(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return NewContext(ctx, DefaultID)
	}
	vals := md.Get(MetadataKey)
	if len(vals) == 0 || vals[0] == "" {
		return NewContext(ctx, DefaultID)
	}
	return NewContext(ctx, ID(vals[0]))
}

type streamWithContext struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *streamWithContext) Context() context.Context { return s.ctx }
