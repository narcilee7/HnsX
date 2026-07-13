// Result type — moved from pkg/runtime/runner.go in Phase 3.

package domain

import "time"

// Result is the structured outcome of a single session run.
type Result struct {
	SessionID  string         `json:"session_id"`
	DomainID   string         `json:"domain_id"`
	State      string         `json:"state"`
	Mode       string         `json:"mode"`
	Output     map[string]any `json:"output"`
	StartedAt  time.Time      `json:"started_at,omitempty"`
	FinishedAt time.Time      `json:"finished_at,omitempty"`
}
