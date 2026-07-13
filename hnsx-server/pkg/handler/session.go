package handler

import (
	"context"
	"errors"
	"sort"
	"time"

	"go.uber.org/zap"

	"github.com/hnsx-io/hnsx/server/internal/app"
	domainmodel "github.com/hnsx-io/hnsx/server/internal/domain/model"
	sessionmodel "github.com/hnsx-io/hnsx/server/internal/session/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	tracemodel "github.com/hnsx-io/hnsx/server/internal/trace/model"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

type ListSessionsInput struct {
	TenantID tenant.ID
	Filters  viewmodel.SessionFilters
	Limit    int
	Offset   int
}

type GetSessionInput struct {
	TenantID  tenant.ID
	SessionID string
}

type TriggerSessionInput struct {
	TenantID      tenant.ID
	DomainID      string
	DomainVersion string
	Trigger       map[string]any
}

type CancelSessionInput struct {
	TenantID  tenant.ID
	SessionID string
}

type RerunSessionInput struct {
	TenantID  tenant.ID
	SessionID string
}

type PauseSessionInput struct {
	TenantID  tenant.ID
	SessionID string
	Reason     string
}

type ResumeSessionInput struct {
	TenantID  tenant.ID
	SessionID string
}

type PauseSessionOutput struct {
	Session *viewmodel.SessionDetail
}

type ResumeSessionOutput struct {
	Session *viewmodel.SessionDetail
}

type GetSessionTraceInput struct {
	TenantID  tenant.ID
	SessionID string
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

type ListSessionsOutput struct {
	Sessions viewmodel.SessionList
}

type GetSessionOutput struct {
	Session *viewmodel.SessionDetail
}

type TriggerSessionOutput struct {
	Session *viewmodel.SessionTriggered
	// Location is the canonical API path for the created session.
	Location string
}

type CancelSessionOutput struct {
	Session *viewmodel.SessionDetail
}

type RerunSessionOutput struct {
	Session  *viewmodel.SessionTriggered
	Location string
}

type GetSessionTraceOutput struct {
	TraceID      string
	SessionID    string
	DomainID     string
	Observations []viewmodel.ObservationItem
}

// ---------------------------------------------------------------------------
// Handler methods
// ---------------------------------------------------------------------------

// ListSessions returns sessions for a tenant, optionally filtered by domain/state.
func (h *Handler) ListSessions(ctx context.Context, in ListSessionsInput) (*ListSessionsOutput, error) {
	defer h.hook(ctx, "session.list", zap.String("tenant_id", string(in.TenantID)))()

	if h.App == nil || h.App.SessionService == nil {
		return nil, sessionmodel.ErrSessionNotFound
	}
	var items []*sessionmodel.Session
	var err error
	if in.Filters.DomainID != "" {
		items, err = h.App.SessionService.ListByDomain(in.TenantID, in.Filters.DomainID)
	} else {
		items, err = h.App.SessionService.List(in.TenantID)
	}
	if err != nil {
		return nil, err
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].StartedAt.After(items[j].StartedAt)
	})

	out := make([]viewmodel.SessionListItem, 0, len(items))
	for _, s := range items {
		if in.Filters.State != "" && string(s.State) != in.Filters.State {
			continue
		}
		out = append(out, viewmodel.SessionListItem{
			ID:            s.ID,
			DomainID:      s.DomainID,
			DomainVersion: s.DomainVersion,
			Orchestration: s.Orchestration,
			State:         string(s.State),
			StartedAt:     s.StartedAt,
			CompletedAt:   s.CompletedAt,
			Summary:       sessionSummary(s),
		})
	}

	limit := in.Limit
	if limit <= 0 {
		limit = len(out)
	}
	return &ListSessionsOutput{Sessions: viewmodel.SessionList{
		Items:  out,
		Total:  len(out),
		Limit:  limit,
		Offset: in.Offset,
	}}, nil
}

// GetSession returns a single session detail.
func (h *Handler) GetSession(ctx context.Context, in GetSessionInput) (*GetSessionOutput, error) {
	defer h.hook(ctx, "session.get", zap.String("tenant_id", string(in.TenantID)), zap.String("session_id", in.SessionID))()

	if h.App == nil || h.App.SessionService == nil {
		return nil, sessionmodel.ErrSessionNotFound
	}
	s, err := h.App.SessionService.Get(in.TenantID, in.SessionID)
	if err != nil {
		return nil, err
	}
	return &GetSessionOutput{Session: toSessionDetail(app.SessionFromModel(s))}, nil
}

// TriggerSession creates and dispatches a new session.
func (h *Handler) TriggerSession(ctx context.Context, in TriggerSessionInput) (*TriggerSessionOutput, error) {
	defer h.hook(ctx, "session.trigger",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("domain_id", in.DomainID),
	)()

	if h.App == nil || h.App.DomainService == nil {
		return nil, domainmodel.ErrDomainNotFound
	}
	dm, err := h.App.DomainService.Get(in.TenantID, in.DomainID)
	if err != nil {
		return nil, err
	}
	d := app.DomainFromModel(dm)

	if h.SessionCommands == nil {
		return nil, errors.New("session commands not configured")
	}
	sess, err := h.SessionCommands.Start(ctx, in.TenantID, d, in.Trigger)
	if err != nil {
		return nil, err
	}
	return &TriggerSessionOutput{
		Session:  &viewmodel.SessionTriggered{ID: sess.ID, State: sess.State},
		Location: "/api/v1/sessions/" + sess.ID,
	}, nil
}

// CancelSession cancels a session and notifies any assigned worker.
func (h *Handler) CancelSession(ctx context.Context, in CancelSessionInput) (*CancelSessionOutput, error) {
	defer h.hook(ctx, "session.cancel",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("session_id", in.SessionID),
	)()

	if h.App == nil || h.SessionCommands == nil {
		return nil, sessionmodel.ErrSessionNotFound
	}
	sess, err := h.SessionCommands.Cancel(ctx, in.TenantID, in.SessionID)
	if err != nil {
		return nil, err
	}

	// Notify the assigned worker, if any.
	if h.App.WorkerService != nil {
		if workerID, ok := h.App.WorkerService.SessionWorker(in.SessionID); ok {
			h.App.WorkerService.SendCancel(workerID, in.SessionID, "user requested cancel", time.Now().Add(5*time.Second).UnixMilli())
		}
	}
	// Detach the in-process broadcaster so SSE consumers close cleanly.
	if h.App.State != nil {
		h.App.State.DetachBroadcaster(in.SessionID)
	}

	return &CancelSessionOutput{Session: toSessionDetail(sess)}, nil
}

// PauseSession flips a running session to paused. The session row stays
// bound to its worker; the worker picks up the new state on its next
// PullSession and stops pulling turns for it. The current turn is allowed
// to finish — this method does not abort mid-flight.
func (h *Handler) PauseSession(ctx context.Context, in PauseSessionInput) (*PauseSessionOutput, error) {
	defer h.hook(ctx, "session.pause",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("session_id", in.SessionID),
	)()
	if h.App == nil || h.SessionCommands == nil {
		return nil, sessionmodel.ErrSessionNotFound
	}
	sess, err := h.SessionCommands.Pause(ctx, in.TenantID, in.SessionID)
	if err != nil {
		return nil, err
	}
	return &PauseSessionOutput{Session: toSessionDetail(sess)}, nil
}

// ResumeSession flips a paused session back to running. The assigned
// worker resumes pulling turns on its next PullSession.
func (h *Handler) ResumeSession(ctx context.Context, in ResumeSessionInput) (*ResumeSessionOutput, error) {
	defer h.hook(ctx, "session.resume",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("session_id", in.SessionID),
	)()
	if h.App == nil || h.SessionCommands == nil {
		return nil, sessionmodel.ErrSessionNotFound
	}
	sess, err := h.SessionCommands.Resume(ctx, in.TenantID, in.SessionID)
	if err != nil {
		return nil, err
	}
	return &ResumeSessionOutput{Session: toSessionDetail(sess)}, nil
}

// RerunSession reruns an existing session.
func (h *Handler) RerunSession(ctx context.Context, in RerunSessionInput) (*RerunSessionOutput, error) {
	defer h.hook(ctx, "session.rerun",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("session_id", in.SessionID),
	)()

	if h.App == nil || h.SessionCommands == nil {
		return nil, sessionmodel.ErrSessionNotFound
	}
	sess, err := h.SessionCommands.Rerun(ctx, in.TenantID, in.SessionID)
	if err != nil {
		return nil, err
	}
	return &RerunSessionOutput{
		Session:  &viewmodel.SessionTriggered{ID: sess.ID, State: sess.State},
		Location: "/api/v1/sessions/" + sess.ID,
	}, nil
}

// GetSessionTrace returns the persisted trace for a session.
func (h *Handler) GetSessionTrace(ctx context.Context, in GetSessionTraceInput) (*GetSessionTraceOutput, error) {
	defer h.hook(ctx, "session.trace.get",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("session_id", in.SessionID),
	)()

	if h.App == nil || h.App.SessionService == nil {
		return nil, sessionmodel.ErrSessionNotFound
	}
	s, err := h.App.SessionService.Get(in.TenantID, in.SessionID)
	if err != nil {
		return nil, err
	}

	out := &GetSessionTraceOutput{
		TraceID:   in.SessionID,
		SessionID: in.SessionID,
		DomainID:  s.DomainID,
	}

	if h.App.TraceService != nil {
		records, err := h.App.TraceService.BySession(in.SessionID)
		if err != nil {
			return nil, err
		}
		for _, rec := range records {
			out.Observations = append(out.Observations, observationRecordToViewModel(rec))
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toSessionDetail(sess *app.RegisteredSession) *viewmodel.SessionDetail {
	if sess == nil {
		return nil
	}
	var completedAt *time.Time
	if sess.CompletedAt != nil && *sess.CompletedAt != "" {
		if t, err := time.Parse(time.RFC3339, *sess.CompletedAt); err == nil {
			completedAt = &t
		}
	}
	var startedAt time.Time
	if sess.StartedAt != "" {
		if t, err := time.Parse(time.RFC3339, sess.StartedAt); err == nil {
			startedAt = t
		}
	}

	var result map[string]any
	if sess.Result != nil {
		result = map[string]any{
			"output": sess.Result.Output,
		}
	}

	return &viewmodel.SessionDetail{
		ID:            sess.ID,
		DomainID:      sess.DomainID,
		DomainVersion: sess.DomainVersion,
		Orchestration: sess.Orchestration,
		State:         sess.State,
		Trigger:       sess.Trigger,
		StartedAt:     startedAt,
		CompletedAt:   completedAt,
		Result:        result,
		Summary:       registeredSessionSummary(sess),
	}
}

func registeredSessionSummary(sess *app.RegisteredSession) *viewmodel.SessionSummary {
	out := &viewmodel.SessionSummary{
		DurationMs:       0,
		Mode:             sess.Orchestration,
		AgentInvocations: 0,
		ToolInvocations:  0,
		TotalCostUSD:     0.0,
	}
	if sess.StartedAt == "" || sess.CompletedAt == nil || *sess.CompletedAt == "" {
		return out
	}
	started, err := time.Parse(time.RFC3339, sess.StartedAt)
	if err != nil {
		return out
	}
	completed, err := time.Parse(time.RFC3339, *sess.CompletedAt)
	if err != nil {
		return out
	}
	delta := completed.Sub(started).Milliseconds()
	if delta > 0 {
		out.DurationMs = uint64(delta)
	}
	return out
}

func sessionSummary(sess *sessionmodel.Session) *viewmodel.SessionSummary {
	out := &viewmodel.SessionSummary{
		DurationMs:       0,
		Mode:             sess.Orchestration,
		AgentInvocations: 0,
		ToolInvocations:  0,
		TotalCostUSD:     0.0,
	}
	if sess.CompletedAt != nil {
		delta := sess.CompletedAt.Sub(sess.StartedAt).Milliseconds()
		if delta > 0 {
			out.DurationMs = uint64(delta)
		}
	}
	return out
}

func observationRecordToViewModel(rec tracemodel.ObservationRecord) viewmodel.ObservationItem {
	return viewmodel.ObservationItem{
		TraceID:       rec.TraceID,
		SessionID:     rec.SessionID,
		DomainID:      rec.DomainID,
		DomainVersion: rec.DomainVersion,
		StepID:        rec.StepID,
		AgentID:       rec.AgentID,
		Kind:          rec.Kind,
		Payload:       rec.Payload,
		Metadata:      rec.Metadata,
		CreatedAt:     rec.CreatedAt,
	}
}

// IsSessionNotFound reports whether err is a session-not-found error.
func IsSessionNotFound(err error) bool {
	return errors.Is(err, sessionmodel.ErrSessionNotFound)
}

// IsInvalidSession reports whether err is an invalid-session error.
func IsInvalidSession(err error) bool {
	return errors.Is(err, sessionmodel.ErrInvalidSession)
}

// IsAlreadyTerminal reports whether err indicates a terminal session.
func IsAlreadyTerminal(err error) bool {
	return errors.Is(err, sessionmodel.ErrAlreadyTerminal)
}
