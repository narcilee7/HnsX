package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/session/broadcaster"
	"github.com/hnsx-io/hnsx/server/pkg/policy"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
	"github.com/hnsx-io/hnsx/server/pkg/telemetry"
)

// PolicyEngineProvider returns a session-scoped policy engine for a domain.
// A nil engine means "permissive".
type PolicyEngineProvider interface {
	SessionEngine(domainID, sessionID string) (*policy.Engine, error)
}

// AuditEntry is the minimal audit event emitted by the executor.
type AuditEntry struct {
	SessionID string
	DomainID  string
	Action    string
	Resource  string
	Decision  string
	Reason    string
	Details   map[string]any
}

// AuditRecorder records security-relevant events during session execution.
type AuditRecorder interface {
	Record(ctx context.Context, entry AuditEntry) error
}

// permissiveProvider is used when no policy provider is configured.
type permissiveProvider struct{}

func (permissiveProvider) SessionEngine(_, _ string) (*policy.Engine, error) {
	return policy.NewEngine(spec.PolicySpec{}), nil
}

// noopRecorder is used when no audit recorder is configured.
type noopRecorder struct{}

func (noopRecorder) Record(context.Context, AuditEntry) error { return nil }

// Executor wires the runner, broadcaster, telemetry sinks, policy engine, and
// audit recorder into a single component that the API layer calls per session.
//
// Responsibilities:
//
//   - Resolve a session-scoped policy engine.
//   - Run the domain spec via a policy-wrapped adapter.
//   - Enforce budget and permission checks at every adapter invocation.
//   - Mirror every observation into a per-session broadcaster (for SSE).
//   - Mirror every observation into all registered telemetry sinks.
//   - Record policy decisions and lifecycle events to the audit log.
type Executor struct {
	adapter   runtime.Adapter
	sinks     []telemetry.Sink
	broadcast *broadcaster.Broadcaster
	policies  PolicyEngineProvider
	audit     AuditRecorder
	mu        sync.Mutex
}

// NewExecutor constructs an Executor bound to a single runtime.Adapter and zero or
// more telemetry sinks. The broadcaster is the same per-session broadcaster
// supplied by the API layer; it is shared (one broadcaster per session).
func NewExecutor(adapter runtime.Adapter, sinks ...telemetry.Sink) *Executor {
	return &Executor{
		adapter:  adapter,
		sinks:    sinks,
		policies: permissiveProvider{},
		audit:    noopRecorder{},
	}
}

// WithBroadcaster attaches a per-session broadcaster. Required for SSE.
func (e *Executor) WithBroadcaster(b *broadcaster.Broadcaster) *Executor {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.broadcast = b
	return e
}

// WithPolicyProvider wires the policy engine provider. Required for budget,
// permission, and guardrail enforcement.
func (e *Executor) WithPolicyProvider(p PolicyEngineProvider) *Executor {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p != nil {
		e.policies = p
	}
	return e
}

// WithAuditRecorder wires the audit recorder. Required for audit logging.
func (e *Executor) WithAuditRecorder(a AuditRecorder) *Executor {
	e.mu.Lock()
	defer e.mu.Unlock()
	if a != nil {
		e.audit = a
	}
	return e
}

// Execute runs the domain spec synchronously and returns the result. It also
// publishes observations to the broadcaster (if attached) and the configured
// sinks. This call blocks until the session is done.
//
// Phase 1 keeps the runner mostly serial; future PRs will move execution to
// goroutines and surface cancellation via context.
func (e *Executor) Execute(ctx context.Context, s *spec.DomainSpec, trigger map[string]any) (*runtime.Result, error) {
	if s == nil {
		return nil, errors.New("executor: nil spec")
	}
	if e.adapter == nil {
		return nil, errors.New("executor: nil adapter")
	}

	sessID := runtime.SessionIDFromContext(ctx)
	if sessID == "" {
		sessID = runtime.NewSessionID(s.ID)
		ctx = runtime.WithSessionID(ctx, sessID)
	}

	e.mu.Lock()
	policies := e.policies
	audit := e.audit
	e.mu.Unlock()

	engine, err := policies.SessionEngine(s.ID, sessID)
	if err != nil {
		return nil, fmt.Errorf("executor: policy engine: %w", err)
	}

	wrapped := &policyAdapter{
		inner:     e.adapter,
		engine:    engine,
		audit:     audit,
		spec:      s,
		sessionID: sessID,
	}

	runner := runtime.NewRunner(wrapped)

	// Hook: pump observations into broadcaster + sinks + audit.
	runner.WithHook(func(obs runtime.Observation) {
		// Stamp session + domain IDs so subscribers don't need to infer them.
		obs.SessionID = sessID
		obs.DomainID = s.ID
		if obs.Timestamp.IsZero() {
			obs.Timestamp = time.Now().UTC()
		}
		e.publish(ctx, obs)
		e.auditObservation(ctx, audit, obs)
	})

	e.auditEvent(ctx, audit, AuditEntry{
		SessionID: sessID,
		DomainID:  s.ID,
		Action:    "session_start",
		Decision:  AuditDecisionAllow,
		Reason:    "session accepted by executor",
	})

	result, err := runner.Run(ctx, s, trigger)
	if err != nil && result == nil {
		e.auditEvent(ctx, audit, AuditEntry{
			SessionID: sessID,
			DomainID:  s.ID,
			Action:    "session_fail",
			Decision:  AuditDecisionDeny,
			Reason:    err.Error(),
		})
		return nil, err
	}

	e.auditEvent(ctx, audit, AuditEntry{
		SessionID: sessID,
		DomainID:  s.ID,
		Action:    "session_end",
		Decision:  AuditDecisionAllow,
		Details: map[string]any{
			"state": result.State,
		},
	})
	return result, err
}

func (e *Executor) publish(ctx context.Context, obs runtime.Observation) {
	e.mu.Lock()
	sinks := e.sinks
	bc := e.broadcast
	e.mu.Unlock()

	for _, s := range sinks {
		// Telemetry sinks should not stall the runner — fan out concurrently.
		go func(s telemetry.Sink) {
			_ = s.Record(ctx, obs)
		}(s)
	}
	if bc != nil {
		// Best-effort — if all subscribers are gone, this is still a no-op.
		_ = bc.Publish(ctx, obs)
	}
}

func (e *Executor) auditObservation(ctx context.Context, audit AuditRecorder, obs runtime.Observation) {
	decision := AuditDecisionAllow
	if obs.Kind == "error" {
		decision = AuditDecisionDeny
	}
	_ = audit.Record(ctx, AuditEntry{
		SessionID: obs.SessionID,
		DomainID:  obs.DomainID,
		Action:    obs.Kind,
		Resource:  obs.AgentID,
		Decision:  decision,
		Details:   obs.Payload,
	})
}

func (e *Executor) auditEvent(ctx context.Context, audit AuditRecorder, entry AuditEntry) {
	_ = audit.Record(ctx, entry)
}

// ----------------------------------------------------------------------------
// Policy adapter
// ----------------------------------------------------------------------------

// policyAdapter wraps a runtime.Adapter and enforces policy checks before and
// after each agent invocation.
type policyAdapter struct {
	inner     runtime.Adapter
	engine    *policy.Engine
	audit     AuditRecorder
	spec      *spec.DomainSpec
	sessionID string
}

func (a *policyAdapter) Name() string { return a.inner.Name() }

func (a *policyAdapter) Invoke(ctx context.Context, agent spec.AgentSpec, prompt string, input map[string]any) (string, error) {
	resource := a.inner.Name()

	if err := a.engine.CheckTurns(); err != nil {
		a.recordDeny(ctx, "budget_turns", resource, err.Error())
		return "", err
	}
	if err := a.engine.CheckTokens(); err != nil {
		a.recordDeny(ctx, "budget_tokens", resource, err.Error())
		return "", err
	}

	for _, tool := range agent.Tools {
		if err := a.checkToolPermission(tool); err != nil {
			a.recordDeny(ctx, "permission", tool, err.Error())
			return "", err
		}
	}

	a.recordAllow(ctx, "adapter_invoke", resource, "budget and permission checks passed")

	out, err := a.inner.Invoke(ctx, agent, prompt, input)

	// Estimate token spend from output length when the adapter does not report
	// it. Real adapters will populate runtime.Observation.Cost in future PRs.
	completionTokens := len(out) / 4
	a.engine.RecordTokens(0, completionTokens)
	a.engine.RecordCost(0)

	if err != nil {
		a.recordDeny(ctx, "adapter_invoke", resource, err.Error())
		return out, err
	}

	a.recordAllow(ctx, "adapter_invoke_complete", resource, map[string]any{
		"completion_tokens": completionTokens,
	})
	return out, nil
}

func (a *policyAdapter) checkToolPermission(toolName string) error {
	cfg, ok := a.spec.Harness.Tools[toolName]
	if !ok {
		// Unknown tools are denied by default to avoid policy bypass.
		return fmt.Errorf("unknown tool %q", toolName)
	}
	switch strings.ToLower(cfg.Kind) {
	case "file_write", "filewrite":
		return a.engine.CanUseFileWrite()
	case "file_delete", "filedelete":
		return a.engine.CanUseFileDelete()
	case "shell", "bash", "execute", "command":
		return a.engine.CanUseShell()
	case "network", "web_search", "fetch", "http", "https":
		return a.engine.CanUseNetwork()
	}
	return nil
}

func (a *policyAdapter) recordAllow(ctx context.Context, action, resource string, reason any) {
	reasonStr := ""
	switch v := reason.(type) {
	case string:
		reasonStr = v
	case error:
		reasonStr = v.Error()
	default:
		if b, err := json.Marshal(v); err == nil {
			reasonStr = string(b)
		}
	}
	_ = a.audit.Record(ctx, AuditEntry{
		SessionID: a.sessionID,
		DomainID:  a.spec.ID,
		Action:    action,
		Resource:  resource,
		Decision:  AuditDecisionAllow,
		Reason:    reasonStr,
	})
}

func (a *policyAdapter) recordDeny(ctx context.Context, action, resource, reason string) {
	_ = a.audit.Record(ctx, AuditEntry{
		SessionID: a.sessionID,
		DomainID:  a.spec.ID,
		Action:    action,
		Resource:  resource,
		Decision:  AuditDecisionDeny,
		Reason:    reason,
	})
}

// ----------------------------------------------------------------------------
// Audit decision constants
// ----------------------------------------------------------------------------

const (
	AuditDecisionAllow = "allow"
	AuditDecisionDeny  = "deny"
	AuditDecisionSkip  = "skip"
)

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// EncodeSessionID is a stable JSON encoding for a session's metadata, used
// when persisting into the `sessions` table or the SSE `:state` event.
func EncodeSessionID(id, domainID string) ([]byte, error) {
	return json.Marshal(map[string]string{
		"session_id": id,
		"domain_id":  domainID,
	})
}

// ErrCanceled is returned when the executor is canceled mid-session.
var ErrCanceled = errors.New("executor: canceled")

// String representation for log lines.
func shortError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
