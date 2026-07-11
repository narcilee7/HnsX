package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/client"
)

// newClient returns a client configured for cfg's server URL and auth token.
func newClient(cfg *Config) *client.Client {
	c := client.NewWithBaseURL(cfg.ServerURL)
	c.AuthToken = cfg.Token
	return c
}

// parseSince converts --since flags ("5m", "1h", "2d") into a time.Time.
// Returns time.Time{} (zero) when empty.
func parseSince(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if len(s) < 2 {
		return time.Time{}, fmt.Errorf("invalid --since %q (use 5m, 1h, 2d, ...)", s)
	}
	unit := s[len(s)-1]
	num := s[:len(s)-1]
	var d time.Duration
	switch unit {
	case 's':
		d, _ = time.ParseDuration(num + "s")
	case 'm':
		d, _ = time.ParseDuration(num + "m")
	case 'h':
		d, _ = time.ParseDuration(num + "h")
	case 'd':
		n, err := time.ParseDuration(num + "h")
		if err != nil {
			return time.Time{}, err
		}
		d = n * 24
	default:
		return time.Time{}, fmt.Errorf("invalid --since unit %q (use s|m|h|d)", string(unit))
	}
	if d <= 0 {
		return time.Time{}, fmt.Errorf("invalid --since %q", s)
	}
	return time.Now().Add(-d), nil
}

// shortTime renders a timestamp as a compact relative string ("3s ago", "12m ago").
func shortTime(iso string) string {
	if iso == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339Nano, iso)
	if err != nil {
		t, err = time.Parse(time.RFC3339, iso)
		if err != nil {
			return iso
		}
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// truncate shortens s to n runes, appending an ellipsis when truncated.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// nonEmpty returns def when s is empty (used for table cells).
func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// parseFilters parses "key=value" flags into a map.
func parseFilters(items []string) (map[string]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(items))
	for _, kv := range items {
		idx := strings.Index(kv, "=")
		if idx <= 0 {
			return nil, fmt.Errorf("invalid --filter %q (want key=value)", kv)
		}
		out[kv[:idx]] = kv[idx+1:]
	}
	return out, nil
}

// filterMatches returns true if every key in filters equals the corresponding
// field in item (string compare). Unknown keys always match.
func filterMatches(item map[string]string, filters map[string]string) bool {
	for k, v := range filters {
		if item[k] != v {
			return false
		}
	}
	return true
}
