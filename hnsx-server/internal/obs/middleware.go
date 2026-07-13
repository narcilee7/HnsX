// Package obs — middleware/integration layer.
//
// Phase 5b (W16+ refactor): instead of touching every handler to
// emit log pairs, this file exposes two thin adapters that cover
// every route with a single registration:
//
//   - GinMiddleware: HTTP, applied in pkg/api/router.go once.
//   - ConnectInterceptor: gRPC, applied in pkg/controlplane/connect_handlers.go
//     once.
//
// Both produce the same structured log shape:
//
/*
Both produce the same structured log shape:

    {
      "ts": "...",
      "level": "info",
      "msg": "api.domains.list.completed",
      ...
    }
*/
package obs

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

// ---------------------------------------------------------------------------
// HTTP (Gin)
// ---------------------------------------------------------------------------

// GinMiddleware logs a name.completed line for every request handled by
// the router it's attached to. The name defaults to "<method> <path>".
//
// Install once in router.go:
//
//	r.Use(obs.GinMiddleware(log))
func GinMiddleware(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		emitHandlerLog(log, c.Request.Context(), ginHandlerName(c), ginLogFields(c, start)...)
	}
}

func ginHandlerName(c *gin.Context) string {
	return "api." + sanitizeRoute(c.FullPath()) + "." + c.Request.Method
}

func ginLogFields(c *gin.Context, start time.Time) []zap.Field {
	return []zap.Field{
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
		zap.String("raw_path", c.Request.URL.Path),
		zap.Int("status", c.Writer.Status()),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.String("client_ip", c.ClientIP()),
		zap.String("user_agent", c.Request.UserAgent()),
		zap.Int("response_size", c.Writer.Size()),
	}
}

// sanitizeRoute turns "/api/v1/domains/:id" into "domains.id" so the log
// name is a stable identifier (not a concrete id value).
func sanitizeRoute(p string) string {
	if p == "" {
		return "unknown"
	}
	out := make([]rune, 0, len(p))
	skipSlash := true
	for _, r := range p {
		switch r {
		case '/':
			if !skipSlash {
				out = append(out, '.')
				skipSlash = true
			}
		case ':':
			out = append(out, '_')
		default:
			out = append(out, r)
			skipSlash = false
		}
	}
	// Strip leading/trailing dots.
	s := string(out)
	for len(s) > 0 && s[0] == '.' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	if s == "" {
		return "unknown"
	}
	return s
}

// ---------------------------------------------------------------------------
// gRPC (Connect)
// ---------------------------------------------------------------------------

// ConnectInterceptor logs a name.completed line for every unary RPC handled
// by the server it's attached to. Stream RPCs are not wrapped by this
// interceptor yet — that's a future change.
//
// Install once in connect_handlers.go:
//
//	interceptors := connect.WithInterceptors(obs.ConnectInterceptor(log))
func ConnectInterceptor(log *zap.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()
			resp, err := next(ctx, req)
			emitHandlerLog(log, ctx, connectHandlerName(req), connectLogFields(req, resp, err, start)...)
			return resp, err
		}
	}
}

func connectHandlerName(req connect.AnyRequest) string {
	// Spec().Procedure is e.g. "/hnsx.v1.DomainRegistryService/ListDomains".
	spec := req.Spec()
	if spec.Procedure == "" {
		return "grpc.unknown"
	}
	// Procedure: "/hnsx.v1.<Svc>/<Method>"; we strip the leading /pkg/ prefix
	// and produce "grpc.<svc>.<method>".
	p := spec.Procedure
	const prefix = "/hnsx.v1."
	if len(p) > len(prefix) && p[:len(prefix)] == prefix {
		p = p[len(prefix):]
	}
	return "grpc." + p
}

func connectLogFields(req connect.AnyRequest, resp connect.AnyResponse, err error, start time.Time) []zap.Field {
	fields := []zap.Field{
		zap.String("procedure", req.Spec().Procedure),
		zap.String("peer", req.Peer().Addr),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		fields = append(fields,
			zap.String("error", err.Error()),
			zap.String("error_code", connect.CodeOf(err).String()),
		)
	}
	if resp != nil {
		fields = append(fields, zap.Int("response_size", 0)) // placeholder
	}
	return fields
}

// ---------------------------------------------------------------------------
// Shared emit
// ---------------------------------------------------------------------------

func emitHandlerLog(log *zap.Logger, ctx context.Context, name string, fields ...zap.Field) {
	if log == nil {
		return
	}
	if tid := tenant.FromContext(ctx); tid != "" {
		fields = append(fields, zap.String("tenant_id", string(tid)))
	}
	log.Info(name+".completed", append(fields, zap.String("event", "completed"))...)
}
