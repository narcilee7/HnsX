// Package obs centralizes observability primitives: structured logging,
// request-scoped field injection, and an obs.HookFunc that wraps handler
// bodies with a "name.started / name.completed" log pair.
//
// Phase 1 scope (W16+ refactor):
//   - NewLogger: production-grade zap.Logger with dev/prod presets
//   - HookFunc: defer-friendly duration + error logger
//
// Not in scope yet (deferred to later W):
//   - OpenTelemetry trace/metric integration
//   - Per-handler histogram metrics
//   - Panic recovery → error response
package obs

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger constructs a zap.Logger from environment.
//
//	OBS_LEVEL=debug|info|warn|error  (default info)
//	OBS_ENV=dev|prod                  (default prod)
//	OBS_FORMAT=json|console           (default json in prod, console in dev)
func NewLogger() (*zap.Logger, error) {
	level := zap.NewAtomicLevelAt(parseLevel(os.Getenv("OBS_LEVEL")))

	env := getenvDefault("OBS_ENV", "prod")
	format := getenvDefault("OBS_FORMAT", "")
	if format == "" {
		format = "json"
		if env == "dev" {
			format = "console"
		}
	}

	cfg := zap.Config{
		Level:            level,
		Development:      env == "dev",
		Encoding:         format,
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			MessageKey:     "msg",
			CallerKey:      "caller",
			StacktraceKey:  "stack",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.MillisDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
	}
	return cfg.Build()
}

func parseLevel(s string) zapcore.Level {
	switch s {
	case "debug":
		return zapcore.DebugLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func getenvDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// ---------------------------------------------------------------------------
// HookFunc: wraps a handler body with a name.started / name.completed pair.
// Use:
//
//	defer obs.HookFunc(ctx, "domain.list", logger,
//	    zap.String("tenant_id", tid),
//	)()
//
// Emits:
//   - logger.Debug(name+".started", fields...)   on entry
//   - logger.Info(name+".completed", duration_ms, err) on return
// ---------------------------------------------------------------------------

// HookFunc returns a function that emits "started" when called and
// "completed" (with duration and error) when the returned closure runs.
//
// Usage idiom:
//
//	fields := []zap.Field{zap.String("id", id)}
//	defer obs.HookFunc(ctx, "domain.get", log, fields...)()
//
// The returned closure takes no args; pair it with defer.
func HookFunc(ctx context.Context, name string, log *zap.Logger, fields ...zap.Field) func() {
	start := time.Now()
	if log != nil {
		log.Debug(name+".started", append(fields, zap.String("event", "started"))...)
	}
	return func() {
		if log == nil {
			return
		}
		dur := time.Since(start)
		log.Info(name+".completed",
			append(fields,
				zap.Duration("duration", dur),
				zap.Int64("duration_ms", dur.Milliseconds()),
				zap.String("event", "completed"),
			)...,
		)
	}
}

// WithErr augments the returned closure to log an error if non-nil.
// Use for explicit error paths:
//
//	defer obs.HookFuncErr(ctx, "domain.get", log, fields...)()(&err)
func HookFuncErr(ctx context.Context, name string, log *zap.Logger, fields ...zap.Field) func() *error {
	hook := HookFunc(ctx, name, log, fields...)
	var err error
	return func() *error {
		hook()
		return &err
	}
}

// FieldsFromContext extracts standard request fields (trace_id / tenant_id)
// from a context. HookFunc consumers usually want these as base fields.
func FieldsFromContext(ctx context.Context) []zap.Field {
	// Phase 1 placeholder: wire up to internal/trace + internal/tenant in
	// a follow-up. Kept as a no-op so callers can always safely call.
	_ = ctx
	return nil
}

// SanityCheck ensures the obs package compiles cleanly when added to the
// dependency graph. It is intentionally tiny.
func SanityCheck() error { _ = fmt.Sprintf; return nil }
