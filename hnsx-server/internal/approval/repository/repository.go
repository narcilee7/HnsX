// Package repository defines the approval.Repository contract and its
// in-memory implementation. The Postgres implementation lives in the
// same package (postgres.go).
package repository

import (
	"sort"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/approval/model"
)

// Repository is the persistence contract for Approval aggregates.
//
// Save is upsert. List returns pending-or-other filtered Items in
// newest-first order. ByID returns the full record (including Context).
type Repository interface {
	Save(a *model.Approval) error
	ByID(id string) (*model.Approval, error)
	List(filter ListFilter) ([]model.ListItem, error)
	// PendingForSession returns the pending approval for the session,
	// or model.ErrApprovalNotFound.
	PendingForSession(sessionID string) (*model.Approval, error)
	// Resolve flips an approval's status to approved/rejected/expired.
	Resolve(id, decidedBy, comment string, status model.Status) error
}

// ListFilter controls the read paths.
type ListFilter struct {
	DomainID  string
	SessionID string
	Status    string // empty means "any"
}

// InMemoryRepository is a thread-safe in-memory implementation.
type InMemoryRepository struct {
	mu          sync.RWMutex
	approvals   map[string]*model.Approval
	sessionPend map[string]string // sessionID -> approval ID, at most one pending per session
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		approvals:   map[string]*model.Approval{},
		sessionPend: map[string]string{},
	}
}

func (r *InMemoryRepository) Save(a *model.Approval) error {
	if a == nil || a.ID == "" {
		return model.ErrApprovalNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if existing, ok := r.approvals[a.ID]; ok {
		a.CreatedAt = existing.CreatedAt
	} else {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	r.approvals[a.ID] = a
	if a.Status == model.StatusPending {
		r.sessionPend[a.SessionID] = a.ID
	} else {
		if cur, ok := r.sessionPend[a.SessionID]; ok && cur == a.ID {
			delete(r.sessionPend, a.SessionID)
		}
	}
	return nil
}

func (r *InMemoryRepository) ByID(id string) (*model.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.approvals[id]
	if !ok {
		return nil, model.ErrApprovalNotFound
	}
	copy := *a
	return &copy, nil
}

func (r *InMemoryRepository) List(filter ListFilter) ([]model.ListItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.ListItem, 0)
	ids := make([]string, 0, len(r.approvals))
	for id := range r.approvals {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		a := r.approvals[id]
		if filter.DomainID != "" && a.DomainID != filter.DomainID {
			continue
		}
		if filter.SessionID != "" && a.SessionID != filter.SessionID {
			continue
		}
		if filter.Status != "" && string(a.Status) != filter.Status {
			continue
		}
		out = append(out, model.ListItem{
			ID:          a.ID,
			SessionID:   a.SessionID,
			DomainID:    a.DomainID,
			Action:      a.Action,
			Resource:    a.Resource,
			RiskLevel:   a.RiskLevel,
			Status:      a.Status,
			RequestedBy: a.RequestedBy,
			CreatedAt:   a.CreatedAt,
			UpdatedAt:   a.UpdatedAt,
		})
	}
	// Newest first by CreatedAt desc.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (r *InMemoryRepository) PendingForSession(sessionID string) (*model.Approval, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.sessionPend[sessionID]
	if !ok {
		return nil, model.ErrApprovalNotFound
	}
	a, ok := r.approvals[id]
	if !ok {
		return nil, model.ErrApprovalNotFound
	}
	copy := *a
	return &copy, nil
}

func (r *InMemoryRepository) Resolve(id, decidedBy, comment string, status model.Status) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.approvals[id]
	if !ok {
		return model.ErrApprovalNotFound
	}
	if a.Status != model.StatusPending {
		return model.ErrAlreadyResolved
	}
	now := time.Now().UTC()
	a.Status = status
	a.ReviewedBy = decidedBy
	a.Comment = comment
	a.UpdatedAt = now
	a.ResolvedAt = &now
	if cur, ok := r.sessionPend[a.SessionID]; ok && cur == id {
		delete(r.sessionPend, a.SessionID)
	}
	return nil
}

var _ Repository = (*InMemoryRepository)(nil)
