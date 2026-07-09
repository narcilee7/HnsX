package observation_test

import (
	"strings"
	"testing"

	"github.com/hnsx-io/hnsx/core/observation"
)

func TestNewSessionID_Format(t *testing.T) {
	id := observation.NewSessionID("customer-service")
	if !strings.HasPrefix(id, "customer-") {
		t.Errorf("expected customer- prefix, got %q", id)
	}
	parts := strings.SplitN(id, "-", 2)
	if len(parts) < 2 || len(parts[0]) == 0 {
		t.Errorf("id format wrong: %q", id)
	}
}

func TestNewSessionID_Unique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := observation.NewSessionID("x")
		if seen[id] {
			t.Fatalf("collision after %d: %q", i, id)
		}
		seen[id] = true
	}
}

func TestNewSessionID_HandlesUnsafeChars(t *testing.T) {
	id := observation.NewSessionID("a/b c!d")
	if strings.ContainsAny(id, "/ !") {
		t.Errorf("unexpected chars in id: %q", id)
	}
}
