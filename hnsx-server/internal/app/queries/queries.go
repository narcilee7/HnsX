// Package queries implements read-only application queries used by the HTTP
// API and gRPC control plane.
package queries

import (
	"time"

	"github.com/hnsx-io/hnsx/server/internal/app"
	domainservice "github.com/hnsx-io/hnsx/server/internal/domain/service"
	sessionservice "github.com/hnsx-io/hnsx/server/internal/session/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

// Queries exposes read-only use cases backed by domain/session services.
type Queries struct {
	domainSvc  *domainservice.Service
	sessionSvc *sessionservice.Service
}

// NewQueries constructs a Queries backed by the supplied services.
func NewQueries(domainSvc *domainservice.Service, sessionSvc *sessionservice.Service) *Queries {
	return &Queries{domainSvc: domainSvc, sessionSvc: sessionSvc}
}

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
func (q *Queries) ListDomains(tenantID tenant.ID) []DomainListItem {
	if q.domainSvc == nil {
		return nil
	}
	items, err := q.domainSvc.List()
	if err != nil {
		return nil
	}
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
func (q *Queries) GetDomain(tenantID tenant.ID, id string) (*DomainListItem, *app.RegisteredDomain, bool) {
	if q.domainSvc == nil {
		return nil, nil, false
	}
	d, err := q.domainSvc.Get(id)
	if err != nil {
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
	return item, app.DomainFromModel(d), true
}

// ListSessions returns every registered session as a list item.
func (q *Queries) ListSessions(tenantID tenant.ID) []SessionListItem {
	if q.sessionSvc == nil {
		return nil
	}
	items, err := q.sessionSvc.List()
	if err != nil {
		return nil
	}
	out := make([]SessionListItem, 0, len(items))
	for _, s := range items {
		out = append(out, SessionListItem{
			ID:            s.ID,
			DomainID:      s.DomainID,
			DomainVersion: s.DomainVersion,
			Orchestration: s.Orchestration,
			State:         string(s.State),
			StartedAt:     s.StartedAt,
			CompletedAt:   s.CompletedAt,
		})
	}
	return out
}

// GetSession returns a single session by ID.
func (q *Queries) GetSession(tenantID tenant.ID, id string) (*app.RegisteredSession, bool) {
	if q.sessionSvc == nil {
		return nil, false
	}
	s, err := q.sessionSvc.Get(id)
	if err != nil {
		return nil, false
	}
	return app.SessionFromModel(s), true
}

// GetSessionTrace returns the trace envelope for a session.
func (q *Queries) GetSessionTrace(tenantID tenant.ID, id string) (*SessionTrace, bool) {
	if q.sessionSvc == nil {
		return nil, false
	}
	if _, err := q.sessionSvc.Get(id); err != nil {
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
