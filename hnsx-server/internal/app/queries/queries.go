// Package queries implements read-only application queries shared between
// the CLI and the HTTP API.
package queries

import "time"

// DomainListItem is the public view returned by ListDomains.
type DomainListItem struct {
	ID          string
	Version     string
	Description string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SessionListItem is the public view returned by ListSessions.
type SessionListItem struct {
	ID            string
	DomainID      string
	DomainVersion string
	Orchestration string
	State         string
	StartedAt     time.Time
	CompletedAt   *time.Time
}

// FormatTime returns the RFC3339 representation or empty string for nil.
func FormatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// FormatTimeValue returns the RFC3339 representation of a time value.
func FormatTimeValue(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
