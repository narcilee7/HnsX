package cli

import (
	"testing"
	"time"
)

func TestParseSince(t *testing.T) {
	cases := []struct {
		in   string
		want bool // true = expect non-zero result
		err  bool
	}{
		{"", false, false},
		{"5m", true, false},
		{"1h", true, false},
		{"2d", true, false},
		{"30s", true, false},
		{"abc", false, true},
		{"5", false, true},
		{"5x", false, true},
	}
	for _, c := range cases {
		got, err := parseSince(c.in)
		if (err != nil) != c.err {
			t.Errorf("parseSince(%q): err=%v want-err=%v", c.in, err, c.err)
		}
		if c.err {
			continue
		}
		if got.IsZero() == c.want {
			t.Errorf("parseSince(%q): zero=%v want-zero=%v", c.in, got.IsZero(), !c.want)
		}
	}
}

func TestParseFilters(t *testing.T) {
	out, err := parseFilters([]string{"id=foo", "state=running"})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "foo" || out["state"] != "running" {
		t.Fatalf("unexpected: %+v", out)
	}
	if _, err := parseFilters([]string{"badflag"}); err == nil {
		t.Fatal("expected error on missing =")
	}
}

func TestFilterMatches(t *testing.T) {
	item := map[string]string{"id": "x", "state": "running"}
	if !filterMatches(item, nil) {
		t.Fatal("nil filters should match")
	}
	if !filterMatches(item, map[string]string{"id": "x"}) {
		t.Fatal("matching key/value should match")
	}
	if filterMatches(item, map[string]string{"id": "y"}) {
		t.Fatal("non-matching key should not match")
	}
}

func TestShortTime(t *testing.T) {
	if got := shortTime(time.Time{}); got != "-" {
		t.Fatalf("empty: got %q want -", got)
	}
	if got := shortTime(time.Now().Add(-30 * time.Second)); got == "" {
		t.Fatal("recent should return non-empty")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Fatalf("under: got %q", got)
	}
	if got := truncate("hello world", 5); got != "hell…" {
		t.Fatalf("over: got %q", got)
	}
	if got := truncate("hi", 0); got != "" {
		t.Fatalf("n=0: got %q", got)
	}
}

func TestStateColor(t *testing.T) {
	cases := map[string]string{
		"running":          "●",
		"completed":        "✓",
		"failed":           "✗",
		"paused":           "⏸",
		"pending_approval": "⏸",
		"cancelled":        "⊘",
		"unknown":          "?",
	}
	for in, want := range cases {
		if got := stateColor(in); got != want {
			t.Errorf("stateColor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewClient(t *testing.T) {
	cfg := Default()
	c := newClient(&cfg)
	if c == nil {
		t.Fatal("newClient returned nil")
	}
	if c.BaseURL != cfg.ServerURL {
		t.Fatalf("BaseURL = %q, want %q", c.BaseURL, cfg.ServerURL)
	}
}
