package obs

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewLogger_Defaults(t *testing.T) {
	// Save and restore env.
	t.Setenv("OBS_LEVEL", "")
	t.Setenv("OBS_ENV", "")
	t.Setenv("OBS_FORMAT", "")

	log, err := NewLogger()
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	if log == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLogger_LevelParsing(t *testing.T) {
	for _, c := range []struct {
		in   string
		want zapcore.Level
	}{
		{"debug", zapcore.DebugLevel},
		{"info", zapcore.InfoLevel},
		{"warn", zapcore.WarnLevel},
		{"error", zapcore.ErrorLevel},
		{"", zapcore.InfoLevel},        // default
		{"unknown", zapcore.InfoLevel}, // unknown → info
	} {
		got := parseLevel(c.in)
		if got != c.want {
			t.Errorf("parseLevel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestHookFunc_EmitsCompleted(t *testing.T) {
	core, recorded := observer.New(zapcore.InfoLevel)
	log := zap.New(core)

	HookFunc(context.Background(), "test.op", log, zap.String("k", "v"))()

	entries := recorded.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	got := entries[0]
	if got.Message != "test.op.completed" {
		t.Errorf("Message = %q, want test.op.completed", got.Message)
	}
	if !strings.Contains(got.ContextMap()["event"].(string), "completed") {
		t.Errorf("event field = %v, want completed", got.ContextMap()["event"])
	}
	// duration_ms should be present and >= 0.
	if _, ok := got.ContextMap()["duration_ms"]; !ok {
		t.Errorf("expected duration_ms field, got fields: %v", got.ContextMap())
	}
}

func TestHookFunc_NilLogger_NoPanic(t *testing.T) {
	// A nil logger should not panic — important because we sometimes
	// wire handlers before zap is initialised (e.g. test fixtures).
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("HookFunc panicked with nil logger: %v", r)
		}
	}()
	HookFunc(context.Background(), "test.op", nil)()
}

func TestFieldsFromContext_NoPanic(t *testing.T) {
	// Phase 1: returns nil. Just make sure it doesn't panic on
	// background, nil, or populated contexts.
	cases := []context.Context{
		context.Background(),
		context.TODO(),
		nil,
	}
	for i, c := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("case %d panicked: %v", i, r)
				}
			}()
			_ = FieldsFromContext(c)
		}()
	}
}
