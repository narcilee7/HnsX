// Package service implements the approval application use cases.
//
// It owns the gate-decision lifecycle: create on human-approval request,
// resolve (approve / reject) on operator input, and broadcast SSE
// events so the console reflects state changes immediately.
package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/approval/model"
	"github.com/hnsx-io/hnsx/server/internal/approval/repository"
)

// Decision is what the runtime wants to know after the gate resolves.
type Decision string

const (
	DecisionApproved Decision = "approved"
	DecisionRejected Decision = "rejected"
	DecisionExpired  Decision = "expired"
)

// Gate is the synchronous, blocking call the runtime uses when it hits a
// tool that the active policy marks "human_approval". Implementations
// block until the operator resolves the approval OR the context is
// canceled (e.g. session cancel).
type Gate interface {
	Request(ctx context.Context, a *model.Approval) (Decision, string, error)
}

// Broadcaster is the SSE sink the gate (and service.List/Resolve) uses
// to push events to subscribed consoles.
type Broadcaster interface {
	PublishApproval(event string, approval *model.Approval)
}

// Service is the approval control-plane facade.
type Service struct {
	repo    repository.Repository
	clock   func() time.Time
	bcast   Broadcaster
	mu      sync.Mutex
	pending map[string]chan GateResult // approvalID -> channel that wakes the gate
}

// GateResult is what gets sent on the wait channel when an operator
// resolves the approval.
type GateResult struct {
	Decision Decision
	Comment  string
}

// NewService wires the service. The broadcaster may be nil — without
// it, the gate still works (humans hit the REST API to resolve) but
// live UI pushes do not fire.
func NewService(repo repository.Repository, bcast Broadcaster) *Service {
	return &Service{
		repo:    repo,
		clock:   time.Now,
		bcast:   bcast,
		pending: make(map[string]chan GateResult),
	}
}

// SetClock overrides the time source (test-only).
func (s *Service) SetClock(fn func() time.Time) { s.clock = fn }

// Create persists a new pending approval and returns the row.
func (s *Service) Create(a *model.Approval) error {
	if a == nil {
		return errors.New("approval: nil")
	}
	if a.ID == "" {
		return errors.New("approval: id is required")
	}
	if a.Status == "" {
		a.Status = model.StatusPending
	}
	if a.RequestedBy == "" {
		a.RequestedBy = "agent:" + a.SessionID
	}
	if err := s.repo.Save(a); err != nil {
		return err
	}
	if s.bcast != nil {
		s.bcast.PublishApproval("approval_required", a)
	}
	return nil
}

// Get returns the full approval by id.
func (s *Service) Get(id string) (*model.Approval, error) {
	return s.repo.ByID(id)
}

// List returns ListItems (no Context map) matching the filter.
func (s *Service) List(filter repository.ListFilter) ([]model.ListItem, error) {
	return s.repo.List(filter)
}

// PendingForSession returns the pending approval blocking a session, if
// any.
func (s *Service) PendingForSession(sessionID string) (*model.Approval, error) {
	return s.repo.PendingForSession(sessionID)
}

// Resolve calls repo.Resolve and wakes any goroutine blocked in Request.
func (s *Service) Resolve(id, decidedBy, comment string, status model.Status) (*model.Approval, error) {
	if err := s.repo.Resolve(id, decidedBy, comment, status); err != nil {
		return nil, err
	}
	got, err := s.repo.ByID(id)
	if err != nil {
		return nil, err
	}
	if s.bcast != nil {
		s.bcast.PublishApproval("approval_resolved", got)
	}
	// Wake the gate waiter if any.
	s.mu.Lock()
	if ch, ok := s.pending[id]; ok {
		select {
		case ch <- GateResult{Decision: toGateDecision(status), Comment: comment}:
		default:
		}
	}
	s.mu.Unlock()
	return got, nil
}

// Approve / Reject helpers — convenience for the REST layer.
func (s *Service) Approve(id, decidedBy, comment string) (*model.Approval, error) {
	return s.Resolve(id, decidedBy, comment, model.StatusApproved)
}

func (s *Service) Reject(id, decidedBy, comment string) (*model.Approval, error) {
	return s.Resolve(id, decidedBy, comment, model.StatusRejected)
}

// Request is the gate implementation. It blocks until:
//   - operator approves/rejects via the REST API,
//   - context ctx is canceled (session cancel/error),
//   - the service's max-wait timer fires.
//
// "domain_id" / "session_id" / etc are taken from a.
func (s *Service) Request(ctx context.Context, a *model.Approval) (Decision, string, error) {
	if a == nil {
		return "", "", errors.New("approval: nil request")
	}
	if err := s.Create(a); err != nil {
		return "", "", fmt.Errorf("approval: create: %w", err)
	}
	waitCh := make(chan GateResult, 1)
	s.mu.Lock()
	s.pending[a.ID] = waitCh
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, a.ID)
		s.mu.Unlock()
	}()

	select {
	case res := <-waitCh:
		return res.Decision, res.Comment, nil
	case <-ctx.Done():
		// Best-effort: mark expired so the row doesn't stay pending.
		_, _ = s.Resolve(a.ID, "system", "context canceled", model.StatusExpired)
		return DecisionExpired, "", ctx.Err()
	}
}

func toGateDecision(s model.Status) Decision {
	switch s {
	case model.StatusApproved:
		return DecisionApproved
	case model.StatusRejected:
		return DecisionRejected
	default:
		return DecisionExpired
	}
}
