// Package daemon_runtime hosts the long-running "daemon" loop that pulls
// issues assigned to agents in this daemon's workspace, spawns the agent
// backend (Claude / Codex / ...), streams messages into the Observation
// sink, and updates issue status as work progresses.
//
// The daemon is opt-in: it does nothing unless Run is called. `hnsxd
// daemon` wires it; `hnsxd serve` does not. The loop is sequential per
// agent — one issue at a time per agent — so R1.9 keeps concurrency low
// and reasoning simple. R2+ may parallelize per-agent or across agents.
package daemon_runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/domain/agent"
	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
	"github.com/hnsx-io/hnsx/server/internal/domain/approval"
	"github.com/hnsx-io/hnsx/server/internal/domain/issue"
	"github.com/hnsx-io/hnsx/server/internal/domain/observation"
	"github.com/hnsx-io/hnsx/server/internal/domain/policy"
	"github.com/hnsx-io/hnsx/server/internal/ws"
)

// PolicyLookup fetches the policy attached to a workspace. The default
// implementation in app picks the workspace's first policy; tests pass
// a stub that returns nil.
type PolicyLookup interface {
	FirstPolicyForWorkspace(ctx context.Context, workspaceID string) (*policy.Policy, error)
}

type approvalGate interface {
	Request(ctx context.Context, a *approval.Approval) error
	Wait(ctx context.Context, approvalID string) (approval.Status, error)
}

// WSClient is the typed subset of the daemon ↔ server WS protocol
// the runtime uses. Implemented by ws.Client; tests pass a stub.
type WSClient interface {
	Claim(ctx context.Context, workspaceID string) ([]ws.ClaimedIssue, error)
	WriteObservations(ctx context.Context, batch []ws.ObservationEvent) error
	UpdateStatus(ctx context.Context, issueID string, status issue.Status) error
	Heartbeat(ctx context.Context, workspaceID string) error
}

// Service is the daemon-runtime orchestrator. It pulls work from the issue
// service, runs it through the agent runtime registry, and records every
// emitted message as an observation.
type Service struct {
	issues   *issue_svc_handle
	agents   *agent_svc_handle
	registry agentruntime.Registry
	sink     observation.Sink
	wsClient WSClient
	eval     EvalAutoRunner
	policies PolicyLookup
	gate     approvalGate
	engine   policy.Engine
	logger   *slog.Logger
}

// Config bundles the dependencies the runtime needs. We use small handles
// (structs with the subset of service methods we call) rather than
// concrete *svc.Service so the runtime is testable with stubs.
type Config struct {
	Issues   IssueListerAndUpdater
	Agents   AgentGetter
	Registry agentruntime.Registry
	Sink     observation.Sink
	WS       WSClient
	Eval     EvalAutoRunner
	Policies PolicyLookup
	Gate     approvalGate
	Engine   policy.Engine
	Logger   *slog.Logger
}

// IssueListerAndUpdater is the issue service subset we depend on.
type IssueListerAndUpdater interface {
	ListAssignedToAgent(ctx context.Context, agentID string, statuses []issue.Status) ([]*issue.Issue, error)
	Get(ctx context.Context, id string) (*issue.Issue, error)
	UpdateStatus(ctx context.Context, id string, status issue.Status) error
}

// AgentGetter is the agent service subset we depend on.
type AgentGetter interface {
	Get(ctx context.Context, id string) (*agent.Agent, error)
	ListByWorkspace(ctx context.Context, workspaceID string, f agent.ListFilter) ([]*agent.Agent, error)
}

// EvalAutoRunner is the eval hook the runtime calls when an issue closes.
// Implementations live in service/eval; the interface lets tests pass
// stubs without dragging the eval package in.
type EvalAutoRunner interface {
	AutoRun(ctx context.Context, workspaceID, issueID string, harnessID *string) error
}

// New wires a daemon runtime from a Config.
func New(cfg Config) *Service {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Service{
		issues:   &issue_svc_handle{IssueListerAndUpdater: cfg.Issues},
		agents:   &agent_svc_handle{AgentGetter: cfg.Agents},
		registry: cfg.Registry,
		sink:     cfg.Sink,
		wsClient: cfg.WS,
		eval:     cfg.Eval,
		policies: cfg.Policies,
		gate:     cfg.Gate,
		engine:   cfg.Engine,
		logger:   cfg.Logger.With("component", "daemon_runtime"),
	}
}

// Run loops until ctx is cancelled. Each iteration:
//   1. Picks the first agent whose status is "working" or "idle"
//   2. Lists its assigned issues in todo / in_progress
//   3. Runs the first one through the configured backend
//   4. Streams messages into the observation sink
//   5. Marks the issue done (or reverts to todo on error)
//
// Concurrency: one agent at a time, one issue at a time. R2 can lift this.
func (s *Service) Run(ctx context.Context, workspaceID string, tick time.Duration) error {
	if tick <= 0 {
		tick = 5 * time.Second
	}
	s.logger.Info("daemon_runtime: starting", "workspace", workspaceID, "tick", tick)

	t := time.NewTicker(tick)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("daemon_runtime: stopping")
			return ctx.Err()
		case <-t.C:
			if err := s.tick(ctx, workspaceID); err != nil && !errors.Is(err, context.Canceled) {
				s.logger.Warn("daemon_runtime: tick error", "err", err)
			}
		}
	}
}

// tick runs one pass: claim work via WS, then for each claimed
// issue run it through the configured backend.
func (s *Service) tick(ctx context.Context, workspaceID string) error {
	// Heartbeat: cheap liveness signal so the server knows this
	// daemon is alive. R3.5h+ writes this to the daemons table.
	if s.wsClient != nil {
		if err := s.wsClient.Heartbeat(ctx, workspaceID); err != nil {
			s.logger.Warn("daemon_runtime: ws heartbeat failed", "err", err)
		}
	}

	// Claim: ask the server for the workspace's assigned issues.
	// R3.5h+ uses WS exclusively; R1.9-R3.5h fell back to direct DB
	// via s.issues.ListAssignedToAgent.
	var issues []*issue.Issue
	if s.wsClient != nil {
		claimed, err := s.wsClient.Claim(ctx, workspaceID)
		if err != nil {
			s.logger.Warn("daemon_runtime: ws claim failed", "err", err)
		} else {
			for _, c := range claimed {
				issues = append(issues, &issue.Issue{
					ID:          c.ID,
					WorkspaceID: c.WorkspaceID,
					Title:       c.Title,
					Description: c.Description,
					AssigneeID:  strPtrOrNil(c.AgentID),
				})
			}
		}
	}
	if s.wsClient == nil {
		// Fallback: direct DB (used by the CLI's e2e paths that
		// share the server's Postgres pool).
		agents, err := s.listAgentsForWorkspace(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("list agents: %w", err)
		}
		if len(agents) == 0 {
			return nil
		}
		for _, a := range agents {
			its, err := s.issues.ListAssignedToAgent(ctx, a.ID, []issue.Status{issue.StatusTodo, issue.StatusInProgress})
			if err != nil {
				s.logger.Warn("daemon_runtime: list issues failed", "agent", a.ID, "err", err)
				continue
			}
			issues = append(issues, its...)
		}
	}

	for _, i := range issues {
		if err := s.runIssue(ctx, workspaceID, i); err != nil {
			s.logger.Warn("daemon_runtime: run issue failed",
				"issue", i.ID, "err", err)
			_ = s.updateIssueStatus(ctx, i, issue.StatusBlocked)
		}
	}
	return nil
}

// strPtrOrNil returns &s when s is non-empty, else nil.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// strDeref returns *s when non-nil, else "".
func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// updateIssueStatus prefers the WS path (daemon ↔ server) when wired;
// falls back to direct IssueSvc for CLI-only paths that don't open a
// WebSocket connection.
func (s *Service) updateIssueStatus(ctx context.Context, i *issue.Issue, status issue.Status) error {
	if s.wsClient != nil {
		if err := s.wsClient.UpdateStatus(ctx, i.ID, status); err != nil {
			s.logger.Warn("daemon_runtime: ws status update failed, falling back",
				"issue", i.ID, "err", err)
			// fall through
		} else {
			return nil
		}
	}
	return s.issues.UpdateStatus(ctx, i.ID, status)
}

// runIssue spawns the configured backend for the agent, streams messages
// into the observation sink, and updates issue status accordingly.
//
// The agent's RuntimeConfig (JSON) is expected to contain {"backend":
// "claude", "model": "..."} — for R1.9 we only honor "backend" and
// default to "claude".
// runIssue runs one issue end-to-end: claim → spawn → stream messages
// → update status. The agent is fetched by ID (WS claim does not yet
// carry full agent row, just the agent_id on the issue).
func (s *Service) runIssue(ctx context.Context, workspaceID string, i *issue.Issue) error {
	agentID := strDeref(i.AssigneeID)
	a, err := s.agents.Get(ctx, agentID)
	if err != nil {
		return fmt.Errorf("load agent %q: %w", agentID, err)
	}
	backendName := "claude"
	model := ""
	if len(a.RuntimeConfig) > 0 {
		var cfg struct {
			Backend string `json:"backend"`
			Model   string `json:"model"`
		}
		_ = jsonUnmarshal(a.RuntimeConfig, &cfg)
		if cfg.Backend != "" {
			backendName = cfg.Backend
		}
		model = cfg.Model
	}

	backend, err := s.registry.Get(backendName)
	if err != nil {
		return fmt.Errorf("resolve backend %q: %w", backendName, err)
	}

	// Move issue to in_progress before we spawn.
	if err := s.issues.UpdateStatus(ctx, i.ID, issue.StatusInProgress); err != nil {
		return fmt.Errorf("mark in_progress: %w", err)
	}

	prompt := i.Description
	if prompt == "" {
		prompt = i.Title
	}

	sess, err := backend.Execute(ctx, prompt, agentruntime.ExecOptions{
		Model: model,
	})
	if err != nil {
		return fmt.Errorf("spawn backend: %w", err)
	}

	// Stream messages into the observation sink.
	promptHash := hashPrompt(prompt)
	agentTemplateID := agentTemplateID(a, backendName, model)
	signatures := newToolSignatureSet()
	pol := s.loadPolicy(ctx, i.WorkspaceID)
	var seq int64
	for msg := range sess.Messages() {
		// Accumulate tool signatures as we see tool_use events — covers
		// both top-level tool_use events AND tool_use blocks embedded
		// inside an assistant message's content[].
		for _, name := range toolNamesForMessage(msg) {
			signatures.Add(name)
		}

		// Policy gate: tool_use events come either as a top-level
		// "type":"tool_use" line OR embedded inside an assistant message's
		// content[] (the Claude stream-json format). We extract both
		// forms and run the policy on each tool_name.
		toolNames := toolNamesForMessage(msg)
		if len(toolNames) > 0 {
			s.logger.Info("daemon_runtime: tool names extracted from msg",
				"msg_kind", msg.Kind, "tools", toolNames)
		}
		for _, toolName := range toolNames {
			if s.engine == nil || pol == nil {
				break
			}
			ec := policy.EvalContext{
				WorkspaceID: i.WorkspaceID,
				IssueID:     i.ID,
				AgentID:     a.ID,
				Action:      "tool_call",
				ToolName:    toolName,
			}
			dec, err := s.engine.Evaluate(ctx, pol, ec)
			if err != nil {
				s.logger.Warn("daemon_runtime: policy eval failed", "err", err)
				break
			}
			switch dec.Action {
			case policy.ActionDeny:
				s.logger.Warn("daemon_runtime: policy denied tool",
					"issue", i.ID, "tool", toolName, "rule", dec.RuleID)
				_ = sess.Cancel(ctx)
				for drain := range sess.Messages() {
					_ = drain
				}
				_ = s.recordPolicyDecision(ctx, i, pol, dec, observation.PolicyDeny)
				return s.issues.UpdateStatus(ctx, i.ID, issue.StatusBlocked)
			case policy.ActionApprovalRequired:
				if s.gate == nil {
					s.logger.Warn("daemon_runtime: approval required but no gate wired; defaulting to allow")
					continue
				}
				s.logger.Info("daemon_runtime: approval required, gating",
					"issue", i.ID, "tool", toolName, "rule", dec.RuleID)
				appr := &approval.Approval{
					ID:          uuid.NewString(),
					WorkspaceID: i.WorkspaceID,
					IssueID:     i.ID,
					AgentID:     a.ID,
					Action:      "tool_call:" + toolName,
					Reason:      dec.Message,
				}
				if err := s.gate.Request(ctx, appr); err != nil {
					s.logger.Warn("daemon_runtime: approval request failed", "err", err)
					continue
				}
				status, werr := s.gate.Wait(ctx, appr.ID)
				if werr != nil {
					s.logger.Warn("daemon_runtime: approval wait failed", "err", werr)
					continue
				}
				if status != approval.StatusGranted {
					s.logger.Info("daemon_runtime: approval denied/expired; aborting",
						"issue", i.ID, "status", status)
					_ = sess.Cancel(ctx)
					for drain := range sess.Messages() {
						_ = drain
					}
					_ = s.recordPolicyDecision(ctx, i, pol, dec, observation.PolicyDeny)
					return s.issues.UpdateStatus(ctx, i.ID, issue.StatusBlocked)
				}
				s.logger.Info("daemon_runtime: approval granted; continuing",
					"issue", i.ID, "approval", appr.ID)
			}
		}

		obs := &observation.Observation{
			ID:              uuid.NewString(),
			WorkspaceID:     i.WorkspaceID,
			IssueID:         i.ID,
			AgentID:         a.ID,
			Kind:            kindFromMessage(msg),
			Sequence:        seq,
			Payload:         msg.Payload,
			OccurredAt:      time.Now().UTC(),
			PromptHash:      promptHash,
			AgentTemplateID: agentTemplateID,
			ToolSignatures:  signatures.JSON(),
		}
		seq++
		if err := s.sink.Write(ctx, obs); err != nil {
			s.logger.Warn("daemon_runtime: write observation failed", "err", err)
		}
	}

	res, err := sess.Result()
	finalStatus := issue.StatusDone
	if err != nil || (res != nil && res.ErrorMessage != "") {
		finalStatus = issue.StatusBlocked
		s.logger.Warn("daemon_runtime: session ended with error",
			"issue", i.ID, "err", err, "result_err", resOrErrMsg(res, err))
	}
	if err := s.issues.UpdateStatus(ctx, i.ID, finalStatus); err != nil {
		return fmt.Errorf("final status update: %w", err)
	}

	// Flywheel hook: when an issue completes (done OR blocked), trigger
	// eval.AutoRun. Failures here are non-fatal — eval results land in
	// their own table and are recoverable.
	if s.eval != nil {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.eval.AutoRun(bgCtx, i.WorkspaceID, i.ID, nil); err != nil {
				s.logger.Warn("daemon_runtime: eval.AutoRun failed",
					"issue", i.ID, "err", err)
			}
		}()
	}

	return nil
}

// listAgentsForWorkspace fetches every agent in the workspace. R2+
// replaces this with a heartbeat-driven registry keyed on daemon_id;
// for now we use the agent service's workspace-scoped list so the
// daemon can find its work.
func (s *Service) listAgentsForWorkspace(ctx context.Context, workspaceID string) ([]*agent.Agent, error) {
	if s.agents == nil {
		return nil, nil
	}
	return s.agents.ListByWorkspace(ctx, workspaceID, agent.ListFilter{Limit: 100})
}

// loadPolicy fetches the workspace's first policy. Returns nil when
// no policy is configured (which the runtime treats as default-allow).
func (s *Service) loadPolicy(ctx context.Context, workspaceID string) *policy.Policy {
	if s.policies == nil {
		return nil
	}
	p, err := s.policies.FirstPolicyForWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.Warn("daemon_runtime: load policy failed", "err", err)
		return nil
	}
	if p == nil {
		s.logger.Info("daemon_runtime: no policy configured for workspace", "workspace", workspaceID)
	} else {
		s.logger.Info("daemon_runtime: policy loaded", "policy", p.ID, "workspace", workspaceID, "name", p.Name)
	}
	return p
}

// log is the package-level slog logger used for debug-level events.
// We avoid the Slogger inside Service.Debug to keep slog.Level config
// at the application boundary.

// recordPolicyDecision writes a KindPolicyDecision Observation so the
// flywheel can join every policy outcome to the message stream.
func (s *Service) recordPolicyDecision(ctx context.Context, i *issue.Issue, p *policy.Policy, dec policy.Decision, outcome observation.PolicyDecision) error {
	payload, _ := json.Marshal(map[string]any{
		"rule_id":  dec.RuleID,
		"message":  dec.Message,
		"policy_id": func() string {
			if p != nil {
				return p.ID
			}
			return ""
		}(),
	})
	obs := &observation.Observation{
		ID:             uuid.NewString(),
		WorkspaceID:    i.WorkspaceID,
		IssueID:        i.ID,
		Kind:           observation.KindPolicyDecision,
		Payload:        payload,
		OccurredAt:     time.Now().UTC(),
		PolicyDecision: outcome,
	}
	return s.sink.Write(ctx, obs)
}

// kindFromMessage maps an agentruntime.Message onto an observation.Kind.
func kindFromMessage(m agentruntime.Message) observation.Kind {
	switch m.Kind {
	case agentruntime.MsgAssistant:
		return observation.KindMessage
	case agentruntime.MsgToolUse, agentruntime.MsgToolResult:
		return observation.KindMessage
	case agentruntime.MsgProgress:
		return observation.KindMessage
	case agentruntime.MsgError:
		return observation.KindMessage
	case agentruntime.MsgSystem:
		return observation.KindMessage
	default:
		return observation.KindMessage
	}
}

func resOrErrMsg(res *agentruntime.Result, err error) string {
	if res != nil && res.ErrorMessage != "" {
		return res.ErrorMessage
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// jsonUnmarshal is a tiny indirection so tests can swap it.
var jsonUnmarshal = func(data []byte, v any) error {
	return defaultJSONUnmarshal(data, v)
}