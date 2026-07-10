// Package queries implements read-only application queries shared between
// the CLI and the HTTP API.
package queries

import (
	"time"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

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

// SessionTrace is the public view returned by GetSessionTrace.
type SessionTrace struct {
	TraceID   string
	SessionID string
	Replay    string
}

// ListDomains returns every registered domain as a list item.
func ListDomains(state *app.State, tenantID tenant.ID) []DomainListItem {
	if state == nil {
		return nil
	}
	items := state.ListDomains(tenantID)
	out := make([]DomainListItem, 0, len(items))
	for _, d := range items {
		out = append(out, DomainListItem{
			ID:          d.ID,
			Version:     d.Version,
			Description: d.Description,
			Status:      "active",
			CreatedAt:   d.CreatedAt,
			UpdatedAt:   d.UpdatedAt,
		})
	}
	return out
}

// GetDomain returns the public view of a single domain.
func GetDomain(state *app.State, tenantID tenant.ID, id string) (*DomainListItem, *app.RegisteredDomain, bool) {
	if state == nil {
		return nil, nil, false
	}
	d, ok := state.LookupDomain(tenantID, id)
	if !ok {
		return nil, nil, false
	}
	item := &DomainListItem{
		ID:          d.ID,
		Version:     d.Version,
		Description: d.Description,
		Status:      "active",
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
	return item, d, true
}

// ListSessions returns every registered session as a list item.
func ListSessions(state *app.State, tenantID tenant.ID) []SessionListItem {
	if state == nil {
		return nil
	}
	items := state.ListSessions(tenantID)
	out := make([]SessionListItem, 0, len(items))
	for _, s := range items {
		out = append(out, SessionListItem{
			ID:            s.ID,
			DomainID:      s.DomainID,
			DomainVersion: s.DomainVersion,
			Orchestration: s.Orchestration,
			State:         s.State,
			StartedAt:     s.StartedAt,
			CompletedAt:   s.CompletedAt,
		})
	}
	return out
}

// GetSession returns a single session by ID.
func GetSession(state *app.State, tenantID tenant.ID, id string) (*app.RegisteredSession, bool) {
	if state == nil {
		return nil, false
	}
	return state.LookupSession(tenantID, id)
}

// GetSessionTrace returns the trace envelope for a session.
func GetSessionTrace(state *app.State, tenantID tenant.ID, id string) (*SessionTrace, bool) {
	if state == nil {
		return nil, false
	}
	if _, ok := state.LookupSession(tenantID, id); !ok {
		return nil, false
	}
	return &SessionTrace{
		TraceID:   id,
		SessionID: id,
		Replay:    "/api/v1/sessions/" + id + "/events",
	}, true
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
