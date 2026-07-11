package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/domain/service"
	"github.com/hnsx-io/hnsx/server/internal/session/broadcaster"
	sessionmodel "github.com/hnsx-io/hnsx/server/internal/session/model"
	sessionservice "github.com/hnsx-io/hnsx/server/internal/session/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/internal/worker"
	workerservice "github.com/hnsx-io/hnsx/server/internal/worker/service"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	pkgexecutor "github.com/hnsx-io/hnsx/server/pkg/session"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// broadcasterManager abstracts the in-process SSE fan-out index. It is
// implemented by *app.State; the interface keeps commands decoupled from the
// concrete type.
type broadcasterManager interface {
	AttachBroadcaster(sessionID string) *broadcaster.Broadcaster
	DetachBroadcaster(sessionID string)
	PublishObservation(sessionID string, obs runtime.Observation) bool
}

// SessionCommands exposes session lifecycle use cases.
type SessionCommands struct {
	sessionSvc *sessionservice.Service
	domainSvc  *service.Service
	workerSvc  *workerservice.Service
	executor   *pkgexecutor.Executor
	bm         broadcasterManager
}

// NewSessionCommands constructs a SessionCommands backed by the supplied services.
func NewSessionCommands(
	sessionSvc *sessionservice.Service,
	domainSvc *service.Service,
	workerSvc *workerservice.Service,
	executor *pkgexecutor.Executor,
	bm broadcasterManager,
) *SessionCommands {
	return &SessionCommands{
		sessionSvc: sessionSvc,
		domainSvc:  domainSvc,
		workerSvc:  workerSvc,
		executor:   executor,
		bm:         bm,
	}
}

// Start creates a session from a domain and dispatches it to either the worker
// pool or the local executor. This is the single entry point for
// "trigger + run" so the HTTP handler no longer chooses between the two paths.
func (c *SessionCommands) Start(ctx context.Context, tenantID tenant.ID, domain *app.RegisteredDomain, trigger map[string]any) (*app.RegisteredSession, error) {
	if c.sessionSvc == nil {
		return nil, errors.New("nil session service")
	}
	if domain == nil || domain.Spec == nil {
		return nil, errors.New("nil domain")
	}

	sess, err := c.sessionSvc.Create(tenantID, sessionservice.CreateParams{
		SessionID:     runtime.NewSessionID(domain.ID),
		DomainID:      domain.ID,
		DomainVersion: domain.Version,
		Orchestration: string(domain.Spec.Harness.Session.Mode),
		Trigger:       trigger,
	})
	if err != nil {
		return nil, err
	}
	registered := app.SessionFromModel(sess)

	if err := c.dispatch(ctx, tenantID, registered, domain, trigger); err != nil {
		return nil, err
	}
	return registered, nil
}

// Rerun creates a new session reusing the trigger of an existing one and
// dispatches it the same way as Start.
func (c *SessionCommands) Rerun(ctx context.Context, tenantID tenant.ID, prevID string) (*app.RegisteredSession, error) {
	if c.sessionSvc == nil {
		return nil, errors.New("nil session service")
	}
	prev, err := c.sessionSvc.Get(tenantID, prevID)
	if err != nil {
		if errors.Is(err, sessionmodel.ErrSessionNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	dm, err := c.domainSvc.Get(tenantID, prev.DomainID)
	if err != nil {
		return nil, fmt.Errorf("lookup domain for rerun: %w", err)
	}
	domain := app.DomainFromModel(dm)

	sess, err := c.sessionSvc.Rerun(tenantID, prev)
	if err != nil {
		return nil, err
	}
	registered := app.SessionFromModel(sess)

	if err := c.dispatch(ctx, tenantID, registered, domain, prev.Trigger); err != nil {
		return nil, err
	}
	return registered, nil
}

// dispatch routes a freshly created session to the worker pool or the local
// executor. It also attaches the session broadcaster so SSE clients see the
// lifecycle events.
func (c *SessionCommands) dispatch(ctx context.Context, tenantID tenant.ID, sess *app.RegisteredSession, domain *app.RegisteredDomain, trigger map[string]any) error {
	_ = c.bm.AttachBroadcaster(sess.ID)

	if c.workerSvc != nil {
		req, err := c.buildWorkerRequest(sess, domain, trigger)
		if err != nil {
			return err
		}
		c.workerSvc.EnqueueSession(req)
		c.bm.PublishObservation(sess.ID, runtime.Observation{
			Kind:      "state",
			SessionID: sess.ID,
			DomainID:  domain.ID,
			Payload:   map[string]any{"state": "pending", "worker_pool": true},
			Timestamp: time.Now().UTC(),
		})
		return nil
	}

	if c.executor == nil {
		return errors.New("executor not configured")
	}

	bc := c.bm.AttachBroadcaster(sess.ID)
	go c.runLocal(context.Background(), tenantID, sess, domain, bc, trigger)
	return nil
}

// buildWorkerRequest serializes the domain spec + trigger into the request the
// worker queue consumes.
func (c *SessionCommands) buildWorkerRequest(sess *app.RegisteredSession, domain *app.RegisteredDomain, trigger map[string]any) (*worker.SessionRequest, error) {
	specJSON, err := json.Marshal(domain.Spec)
	if err != nil {
		return nil, fmt.Errorf("marshal domain spec: %w", err)
	}
	triggerJSON, err := json.Marshal(trigger)
	if err != nil {
		return nil, fmt.Errorf("marshal trigger: %w", err)
	}

	req := &worker.SessionRequest{
		SessionID:            sess.ID,
		DomainID:             domain.ID,
		DomainVersion:        domain.Version,
		DomainSpecJSON:       string(specJSON),
		TriggerPayloadJSON:   string(triggerJSON),
		TraceID:              sess.ID,
		CorrelationID:        sess.ID,
		RequiredCapabilities: spec.DeriveCapabilities(domain.Spec),
	}
	return req, nil
}

// runLocal executes the session in-process via the executor.
func (c *SessionCommands) runLocal(ctx context.Context, tenantID tenant.ID, sess *app.RegisteredSession, domain *app.RegisteredDomain, bc *broadcaster.Broadcaster, trigger map[string]any) {
	_, _ = c.sessionSvc.MarkRunning(tenantID, sess.ID)
	executor := c.executor.WithBroadcaster(bc)

	execCtx := runtime.WithSessionID(ctx, sess.ID)

	result, err := executor.Execute(execCtx, domain.Spec, trigger)
	if result != nil {
		_, _ = c.sessionSvc.MarkCompleted(tenantID, sess.ID, result)
	}
	if err != nil {
		_, _ = c.sessionSvc.MarkFailed(tenantID, sess.ID)
	}

	stateSess, _ := c.sessionSvc.Get(tenantID, sess.ID)
	payload := map[string]any{"state": string(stateSess.State)}
	if result != nil {
		payload["result"] = result
	}
	_ = bc.Publish(execCtx, runtime.Observation{
		Kind:      "state",
		SessionID: sess.ID,
		DomainID:  sess.DomainID,
		Payload:   payload,
	})
}

// Trigger is the low-level "create a session record" command. Prefer Start for
// the full trigger+dispatch flow.
func (c *SessionCommands) Trigger(ctx context.Context, tenantID tenant.ID, domain *app.RegisteredDomain, trigger map[string]any, newID func(string) string) (*app.RegisteredSession, error) {
	if c.sessionSvc == nil {
		return nil, errors.New("nil session service")
	}
	if domain == nil || domain.Spec == nil {
		return nil, errors.New("nil domain")
	}

	sess, err := c.sessionSvc.Create(tenantID, sessionservice.CreateParams{
		SessionID:     newID(domain.ID),
		DomainID:      domain.ID,
		DomainVersion: domain.Version,
		Orchestration: string(domain.Spec.Harness.Session.Mode),
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
	sess, err := c.sessionSvc.Cancel(tenantID, id)
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

// MarkRunning transitions a pending session to running.
func (c *SessionCommands) MarkRunning(ctx context.Context, tenantID tenant.ID, id string) (*app.RegisteredSession, error) {
	if c.sessionSvc == nil {
		return nil, errors.New("nil session service")
	}
	sess, err := c.sessionSvc.MarkRunning(tenantID, id)
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
	sess, err := c.sessionSvc.MarkCompleted(tenantID, id, result)
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
	sess, err := c.sessionSvc.MarkFailed(tenantID, id)
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
