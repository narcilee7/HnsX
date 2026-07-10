package commands

import (
	"context"
	"errors"
	"fmt"

	"github.com/hnsx-io/hnsx/server/internal/app"
	domainservice "github.com/hnsx-io/hnsx/server/internal/domain/service"
	sessionmodel "github.com/hnsx-io/hnsx/server/internal/session/model"
	sessionservice "github.com/hnsx-io/hnsx/server/internal/session/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	pkgexecutor "github.com/hnsx-io/hnsx/server/pkg/session"
	"github.com/hnsx-io/hnsx/server/pkg/worker"
)

// SessionCommands exposes session lifecycle use cases.
type SessionCommands struct {
	sessionSvc *sessionservice.Service
	domainSvc  *domainservice.Service
	queue      worker.SessionQueue
	executor   *pkgexecutor.Executor
}

// NewSessionCommands constructs a SessionCommands backed by the supplied services.
func NewSessionCommands(sessionSvc *sessionservice.Service, domainSvc *domainservice.Service, queue worker.SessionQueue, executor *pkgexecutor.Executor) *SessionCommands {
	return &SessionCommands{
		sessionSvc: sessionSvc,
		domainSvc:  domainSvc,
		queue:      queue,
		executor:   executor,
	}
}

// Trigger creates a registered session from a domain and trigger.
func (c *SessionCommands) Trigger(ctx context.Context, tenantID tenant.ID, domain *app.RegisteredDomain, trigger map[string]any, newID func(string) string) (*app.RegisteredSession, error) {
	if c.sessionSvc == nil {
		return nil, errors.New("nil session service")
	}
	if domain == nil || domain.Spec == nil {
		return nil, errors.New("nil domain")
	}

	sess, err := c.sessionSvc.Create(sessionservice.CreateParams{
		SessionID:     newID(domain.ID),
		DomainID:      domain.ID,
		DomainVersion: domain.Version,
		Orchestration: domain.Spec.Harness.Session.Mode,
		Trigger:       trigger,
	})
	if err != nil {
		return nil, err
	}
	return app.SessionFromModel(sess), nil
}

// Cancel transitions a non-terminal session to cancelled.
func (c *SessionCommands) Cancel(ctx context.Context, tenantID tenant.ID, id string) (*app.RegisteredSession, error) {
	if c.sessionSvc == nil {
		return nil, errors.New("nil session service")
	}
	sess, err := c.sessionSvc.Cancel(id)
	if err != nil {
		if errors.Is(err, sessionmodel.ErrSessionNotFound) {
			return nil, ErrSessionNotFound
		}
		if errors.Is(err, sessionmodel.ErrAlreadyTerminal) {
			return nil, fmt.Errorf("session is already in terminal state %q", sess.State)
		}
		return nil, err
	}
	return app.SessionFromModel(sess), nil
}

// Rerun creates a new session reusing the trigger of an existing one.
func (c *SessionCommands) Rerun(ctx context.Context, tenantID tenant.ID, prevID string) (*app.RegisteredSession, error) {
	if c.sessionSvc == nil {
		return nil, errors.New("nil session service")
	}
	prev, err := c.sessionSvc.Get(prevID)
	if err != nil {
		if errors.Is(err, sessionmodel.ErrSessionNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	sess, err := c.sessionSvc.Rerun(prev)
	if err != nil {
		return nil, err
	}
	return app.SessionFromModel(sess), nil
}

// MarkRunning transitions a pending session to running.
func (c *SessionCommands) MarkRunning(ctx context.Context, tenantID tenant.ID, id string) (*app.RegisteredSession, error) {
	if c.sessionSvc == nil {
		return nil, errors.New("nil session service")
	}
	sess, err := c.sessionSvc.MarkRunning(id)
	if err != nil {
		if errors.Is(err, sessionmodel.ErrSessionNotFound) {
			return nil, ErrSessionNotFound
		}
		if errors.Is(err, sessionmodel.ErrInvalidSession) {
			return nil, fmt.Errorf("session state is not pending")
		}
		return nil, err
	}
	return app.SessionFromModel(sess), nil
}

// MarkCompleted stores the result and transitions to completed.
func (c *SessionCommands) MarkCompleted(ctx context.Context, tenantID tenant.ID, id string, result *runtime.Result) (*app.RegisteredSession, error) {
	if c.sessionSvc == nil {
		return nil, errors.New("nil session service")
	}
	sess, err := c.sessionSvc.MarkCompleted(id, result)
	if err != nil {
		if errors.Is(err, sessionmodel.ErrSessionNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return app.SessionFromModel(sess), nil
}

// MarkFailed transitions a session to failed.
func (c *SessionCommands) MarkFailed(ctx context.Context, tenantID tenant.ID, id string) (*app.RegisteredSession, error) {
	if c.sessionSvc == nil {
		return nil, errors.New("nil session service")
	}
	sess, err := c.sessionSvc.MarkFailed(id)
	if err != nil {
		if errors.Is(err, sessionmodel.ErrSessionNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return app.SessionFromModel(sess), nil
}

// BuildDomainLocation returns the canonical API location for a domain.
func BuildDomainLocation(id string) string {
	return "/api/v1/domains/" + id
}

// BuildSessionLocation returns the canonical API location for a session.
func BuildSessionLocation(id string) string {
	return "/api/v1/sessions/" + id
}
