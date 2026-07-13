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
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// broadcasterManager abstracts the in-process SSE fan-out index. It is
// implemented by *app.State; the interface keeps commands decoupled from the
// concrete type.
type broadcasterManager interface {
	AttachBroadcaster(sessionID string) *broadcaster.Broadcaster
	DetachBroadcaster(sessionID string)
	PublishObservation(sessionID string, obs domain.Observation) bool
}

// SessionCommands exposes session lifecycle use cases.
type SessionCommands struct {
	sessionSvc *sessionservice.Service
	domainSvc  *service.Service
	workerSvc  *workerservice.Service
	bm         broadcasterManager
}

// NewSessionCommands constructs a SessionCommands backed by the supplied services.
func NewSessionCommands(
	sessionSvc *sessionservice.Service,
	domainSvc *service.Service,
	workerSvc *workerservice.Service,
	bm broadcasterManager,
) *SessionCommands {
	return &SessionCommands{
		sessionSvc: sessionSvc,
		domainSvc:  domainSvc,
		workerSvc:  workerSvc,
		bm:         bm,
	}
}

// Start creates a session from a domain and dispatches it to either the worker
// pool or the local executor. This is the single entry point for
// "trigger + run" so the HTTP handler no longer chooses between the two paths.
func (c *SessionCommands) Start(ctx context.Context, tenantID tenant.ID, d *app.RegisteredDomain, trigger map[string]any) (*app.RegisteredSession, error) {
	if c.sessionSvc == nil {
		return nil, errors.New("nil session service")
	}
	if d == nil || d.Spec == nil {
		return nil, errors.New("nil domain")
	}

	sess, err := c.sessionSvc.Create(tenantID, sessionservice.CreateParams{
		SessionID:     domain.NewSessionID(d.ID),
		DomainID:      d.ID,
		DomainVersion: d.Version,
		Orchestration: string(d.Spec.Harness.Session.Mode),
		Trigger:       trigger,
	})
	if err != nil {
		return nil, err
	}
	registered := app.SessionFromModel(sess)

	if err := c.dispatch(ctx, tenantID, registered, d, trigger); err != nil {
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
func (c *SessionCommands) dispatch(ctx context.Context, tenantID tenant.ID, sess *app.RegisteredSession, d *app.RegisteredDomain, trigger map[string]any) error {
	_ = c.bm.AttachBroadcaster(sess.ID)

	if c.workerSvc != nil {
		req, err := c.buildWorkerRequest(sess, d, trigger)
		if err != nil {
			return err
		}
		c.workerSvc.EnqueueSession(req)
		c.bm.PublishObservation(sess.ID, domain.Observation{
			Kind:      "state",
			SessionID: sess.ID,
			DomainID:  d.ID,
			Payload:   map[string]any{"state": "pending", "worker_pool": true},
			Timestamp: time.Now().UTC(),
		})
		return nil
	}

	// Phase 3: removed local executor fallback. All sessions go through
	// the worker pool (c.workerSvc). If workerSvc is nil, we fail loud
	// — this is a deployment misconfiguration.
	return errors.New("no worker pool configured; set HNSX_GRPC_ADDR or run a local hnsx-worker")
}

// buildWorkerRequest serializes the domain spec + trigger into the request the
// worker queue consumes.
func (c *SessionCommands) buildWorkerRequest(sess *app.RegisteredSession, d *app.RegisteredDomain, trigger map[string]any) (*worker.SessionRequest, error) {
	specJSON, err := json.Marshal(d.Spec)
	if err != nil {
		return nil, fmt.Errorf("marshal domain spec: %w", err)
	}
	triggerJSON, err := json.Marshal(trigger)
	if err != nil {
		return nil, fmt.Errorf("marshal trigger: %w", err)
	}

	req := &worker.SessionRequest{
		SessionID:            sess.ID,
		DomainID:             d.ID,
		DomainVersion:        d.Version,
		DomainSpecJSON:       string(specJSON),
		TriggerPayloadJSON:   string(triggerJSON),
		TraceID:              sess.ID,
		CorrelationID:        sess.ID,
		RequiredCapabilities: domain.DeriveCapabilities(d.Spec),
	}
	return req, nil
}

// runLocal was removed in W16+ Phase 3 (Executor gone). Local execution
// goes through the Python worker; the Go side only orchestrates and
// persists session state.
//
// This stub is kept so dispatch()'s call site still compiles. The
// caller (dispatch) is also a no-op in W16+ — see the comment there.
// All session work now goes through the worker pool.
func (c *SessionCommands) runLocal(ctx context.Context, tenantID tenant.ID, sess *app.RegisteredSession, d *app.RegisteredDomain, bc *broadcaster.Broadcaster, trigger map[string]any) {
	_ = ctx
	_ = tenantID
	_ = sess
	_ = d
	_ = bc
	_ = trigger
	// Intentionally empty: no in-process executor.
}

// Trigger is the low-level "create a session record" command. Prefer Start for
// the full trigger+dispatch flow.
func (c *SessionCommands) Trigger(ctx context.Context, tenantID tenant.ID, d *app.RegisteredDomain, trigger map[string]any, newID func(string) string) (*app.RegisteredSession, error) {
	if c.sessionSvc == nil {
		return nil, errors.New("nil session service")
	}
	if d == nil || d.Spec == nil {
		return nil, errors.New("nil domain")
	}

	sess, err := c.sessionSvc.Create(tenantID, sessionservice.CreateParams{
		SessionID:     newID(d.ID),
		DomainID:      d.ID,
		DomainVersion: d.Version,
		Orchestration: string(d.Spec.Harness.Session.Mode),
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
func (c *SessionCommands) MarkCompleted(ctx context.Context, tenantID tenant.ID, id string, result *domain.Result) (*app.RegisteredSession, error) {
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
