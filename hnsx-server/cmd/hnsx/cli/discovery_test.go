package cli

import (
	"testing"
)

func TestDiscoverExamples(t *testing.T) {
	cfg := Default()
	if !hasMarker(cfg.RepoRoot) {
		t.Skip("running outside HnsX repo")
	}
	cfgPtr := &cfg
	items, err := discoverExamples(cfgPtr)
	if err != nil {
		t.Fatalf("discoverExamples: %v", err)
	}
	if len(items) < 3 {
		t.Fatalf("expected at least 3 examples, got %d", len(items))
	}
	seen := map[string]bool{}
	for _, e := range items {
		if e.Name == "" || e.Path == "" {
			t.Fatalf("incomplete example: %+v", e)
		}
		if seen[e.Name] {
			t.Fatalf("duplicate name: %s", e.Name)
		}
		seen[e.Name] = true
	}
	// customer-service must be among them.
	if !seen["customer-service"] {
		t.Fatalf("expected customer-service among examples")
	}
}

func TestHealthLabel(t *testing.T) {
	if got := healthLabel(nil); !strings_Contains(got, "ok") {
		t.Fatalf("nil err should yield ok, got %q", got)
	}
	if got := healthLabel(errString("boom")); !strings_Contains(got, "boom") {
		t.Fatalf("err should appear, got %q", got)
	}
}

// tiny shims so we don't pull in fmt-style helpers just for tests.
func strings_Contains(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

type errString string

func (e errString) Error() string { return string(e) }
