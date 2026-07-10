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

// ApprovalRequest is the minimal contract the executor needs from a
// human-in-the-loop gate. The gate is responsible for surfacing the
// request to operators, blocking until they decide, and reporting the
// outcome. The approval subsystem implements this today; Python
// workers implement their own gRPC equivalent.
type ApprovalRequest struct {
	SessionID string
	DomainID  string
	Action    string
	Resource  string
	Context   map[string]any
}

// ApprovalGate is the runtime↔approval control-plane contract.
type ApprovalGate interface {
	Request(ctx context.Context, req ApprovalRequest) (approved bool, comment string, err error)
}

// noopGate is the default; it logs a policy_check and treats "human_approval"
// as the policy says — i.e. blocks — but yields the same error path as before
// so existing tests keep passing.
type noopGate struct{}

func (noopGate) Request(context.Context, ApprovalRequest) (bool, string, error) {
	return false, "", errors.New("approval: gate not configured")
}

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
//   - Block on policy-approved human gates via the optional ApprovalGate.
type Executor struct {
	adapter    runtime.Adapter
	sinks      []runtime.Sink
	broadcast  *broadcaster.Broadcaster
	policies   PolicyEngineProvider
	audit      AuditRecorder
	approval   ApprovalGate
	mu         sync.Mutex
}

// NewExecutor constructs an Executor bound to a single runtime.Adapter and zero or
// more telemetry sinks. The broadcaster is the same per-session broadcaster
// supplied by the API layer; it is shared (one broadcaster per session).
func NewExecutor(adapter runtime.Adapter, sinks ...runtime.Sink) *Executor {
	return &Executor{
		adapter:  adapter,
		sinks:    sinks,
		policies: permissiveProvider{},
		audit:    noopRecorder{},
		approval: noopGate{},
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

// WithApprovalGate wires the human-in-the-loop gate. When the active
// policy emits a human_approval decision, the executor calls Request
// and blocks until the operator resolves or context is canceled.
func (e *Executor) WithApprovalGate(g ApprovalGate) *Executor {
	e.mu.Lock()
	defer e.mu.Unlock()
	if g != nil {
		e.approval = g
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
		approval:  e.approval,
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
		// Attach the current policy-engine cost snapshot to the observations
		// that represent the outcome of an adapter invocation. This unifies
		// budget tracking with trace/metric aggregation.
		if obs.Kind == "agent_text" || obs.Kind == "error" {
			obs.Cost = engine.Snapshot()
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
		go func(s runtime.Sink) {
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
	approval  ApprovalGate
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
	// it. The estimate is fed into the policy engine so budget enforcement and
	// trace/metric aggregation share the same numbers.
	completionTokens := len(out) / 4
	a.engine.RecordTokens(0, completionTokens)
	a.engine.RecordCost(estimateCostUSD(0, completionTokens))

	// Content guardrails run on the adapter output.
	decision := a.engine.EvaluateGuardrails(policy.GuardrailEvent{
		Kind:    "agent_text",
		AgentID: agent.ID,
		Text:    out,
	})
	if decision.Matched {
		a.recordAllow(ctx, "guardrail_hit", resource, map[string]any{
			"guardrail_id": decision.GuardrailID,
			"action":       decision.Action,
			"message":      decision.Message,
		})
		switch decision.Action {
		case "block":
			a.recordDeny(ctx, "guardrail_block", resource, decision.Message)
			return "", fmt.Errorf("%w: %s", policy.ErrGuardrailBlocked, decision.Message)
		case "human_approval":
			// Suspend the agent call until a human approves or rejects the
			// action. The control-plane gate blocks; if no gate is wired,
			// the noopGate returns an error so the executor surfaces it
			// instead of letting the call proceed silently.
			if a.approval == nil {
				a.recordDeny(ctx, "approval_no_gate", resource,
					"policy requires human approval but no approval gate is wired")
				return "", fmt.Errorf(
					"approval: policy requires human approval but no gate is configured")
			}
			req := ApprovalRequest{
				SessionID: a.sessionID,
				DomainID:  a.spec.ID,
				Action:    fmt.Sprintf("guardrail:%s", decision.GuardrailID),
				Resource:  resource,
				Context: map[string]any{
					"guardrail_id": decision.GuardrailID,
					"message":      decision.Message,
					"input":        input,
				},
			}
			approved, comment, gerr := a.approval.Request(ctx, req)
			a.recordAllow(ctx, "approval_decision", resource, map[string]any{
				"approved": approved,
				"comment":  comment,
				"gate_err": gerr,
			})
			if gerr != nil {
				a.recordDeny(ctx, "approval_gate_error", resource, gerr.Error())
				return "", gerr
			}
			if !approved {
				a.recordDeny(ctx, "approval_rejected", resource, comment)
				return "", fmt.Errorf("approval: rejected by operator: %s", comment)
			}
		}
	}

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

// estimateCostUSD computes a placeholder cost from token counts. Real adapters
// will report actual usage; this fallback lets local runs produce non-zero
// cost traces for development and eval.
func estimateCostUSD(promptTokens, completionTokens int) float64 {
	const (
		promptRateUSDPer1K     = 0.0015
		completionRateUSDPer1K = 0.0060
	)
	return float64(promptTokens)*promptRateUSDPer1K/1000 +
		float64(completionTokens)*completionRateUSDPer1K/1000
}

// String representation for log lines.
func shortError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
