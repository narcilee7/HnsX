package app

import (
	"time"

	domainmodel "github.com/hnsx-io/hnsx/server/internal/domain/model"
	sessionmodel "github.com/hnsx-io/hnsx/server/internal/session/model"
)

// DomainFromModel maps a domain aggregate to the API-layer runtime view.
func DomainFromModel(d *domainmodel.RegisteredDomain) *RegisteredDomain {
	if d == nil {
		return nil
	}
	return &RegisteredDomain{
		ID:          d.ID,
		Version:     d.Version,
		Description: d.Description,
		Spec:        d.Spec,
		Harness:     d.Harness(),
		CreatedAt:   formatTime(d.CreatedAt),
		UpdatedAt:   formatTime(d.UpdatedAt),
	}
}

// SessionFromModel maps a session aggregate to the API-layer runtime view.
func SessionFromModel(s *sessionmodel.Session) *RegisteredSession {
	if s == nil {
		return nil
	}
	return &RegisteredSession{
		ID:            s.ID,
		DomainID:      s.DomainID,
		DomainVersion: s.DomainVersion,
		Orchestration: s.Orchestration,
		State:         string(s.State),
		Trigger:       s.Trigger,
		Result:        s.Result,
		StartedAt:     formatTime(s.StartedAt),
		CompletedAt:   formatPtrTime(s.CompletedAt),
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func formatPtrTime(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	v := t.UTC().Format(time.RFC3339)
	return &v
}
