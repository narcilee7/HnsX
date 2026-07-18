// Package eval hosts the application-level orchestration for EvalSet /
// Run. The Runner scores a list of observations against an EvalSet's
// cases and persists the resulting Run.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/domain/eval"
	"github.com/hnsx-io/hnsx/server/internal/domain/observation"
)

// Service is the eval application service.
type Service struct {
	setRepo eval.EvalSetRepo
	runRepo eval.RunRepo
	sink    observation.Sink
	logger  *slog.Logger
}

// New wires a service from its dependencies.
func New(setRepo eval.EvalSetRepo, runRepo eval.RunRepo, sink observation.Sink, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{setRepo: setRepo, runRepo: runRepo, sink: sink, logger: logger}
}

// AutoRun triggers an eval run for the issue's harness. Called by
// daemon_runtime when an issue closes.
//
// Returns the persisted Run on success, or (nil, nil) when the workspace
// has no EvalSet configured (a legitimate no-op state). Errors surface
// real failures (DB / scoring).
func (s *Service) AutoRun(ctx context.Context, workspaceID, issueID string, harnessID *string) (*eval.Run, error) {
	// For R3, AutoRun assumes a workspace default EvalSet — R3.x adds
	// harness-pinned EvalSets via EvalSetID.
	sets, err := s.setRepo.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("eval: list sets: %w", err)
	}
	if len(sets) == 0 {
		s.logger.Info("eval.AutoRun: no eval sets in workspace; skipping", "workspace", workspaceID)
		return nil, nil
	}
	es := sets[0] // pick the first; R3.x picks by harness binding

	observations, err := s.sink.ListByIssue(ctx, issueID, 500)
	if err != nil {
		return nil, fmt.Errorf("eval: list observations: %w", err)
	}

	run := &eval.Run{
		ID:          uuid.NewString(),
		WorkspaceID: workspaceID,
		EvalSetID:   es.ID,
		IssueID:     &issueID,
		HarnessID:   harnessID,
		Status:      eval.RunRunning,
		StartedAt:   time.Now().UTC(),
	}
	if err := s.runRepo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("eval: create run: %w", err)
	}

	// Score
	results, total := s.score(ctx, es, observations)
	now := time.Now().UTC()
	run.Status = eval.RunCompleted
	run.TotalScore = total
	run.Results = results
	run.CompletedAt = &now
	if err := s.runRepo.Update(ctx, run); err != nil {
		return nil, fmt.Errorf("eval: update run: %w", err)
	}

	// Write a KindEvalScore Observation so the flywheel can join scores
	// to observation streams by EvalRunID.
	if err := s.sink.Write(ctx, &observation.Observation{
		ID:           uuid.NewString(),
		WorkspaceID:  workspaceID,
		IssueID:      issueID,
		AgentID:      "",
		Kind:         observation.KindEvalScore,
		Payload:      json.RawMessage(fmt.Sprintf(`{"run_id":"%s","total_score":%f}`, run.ID, total)),
		OccurredAt:   now,
		EvalRunID:    run.ID,
	}); err != nil {
		s.logger.Warn("eval.AutoRun: write EvalScore observation failed", "err", err)
	}

	return run, nil
}

// AutoRunErr is the daemon_runtime-compatible signature: returns just
// an error so it satisfies daemon_runtime.EvalAutoRunner. Internally
// wraps AutoRun.
func (s *Service) AutoRunErr(ctx context.Context, workspaceID, issueID string, harnessID *string) error {
	_, err := s.AutoRun(ctx, workspaceID, issueID, harnessID)
	return err
}

// score runs each case through its scorer and accumulates weighted total.
func (s *Service) score(ctx context.Context, es *eval.EvalSet, observations []*observation.Observation) (json.RawMessage, float64) {
	cases, err := es.CasesTyped()
	if err != nil {
		s.logger.Warn("eval: decode cases failed", "err", err)
		return json.RawMessage("[]"), 0
	}
	results := make([]eval.CaseResult, 0, len(cases))
	var total float64
	for _, c := range cases {
		observed := pickLastAssistantMessage(observations)
		passed, score, reason := scoreOne(c, observed)
		results = append(results, eval.CaseResult{
			CaseName: c.Name,
			Score:    score,
			Passed:   passed,
			Reason:   reason,
			Observed: json.RawMessage(fmt.Sprintf("%q", observed)),
		})
		total += score * c.Weight
	}
	out, _ := json.Marshal(results)
	return out, total
}

// pickLastAssistantMessage picks the last assistant message payload's
// text content out of the observation stream. R3.x wires a real
// reducer; for R3 we just extract the last "message" with kind=assistant.
func pickLastAssistantMessage(observations []*observation.Observation) string {
	for i := len(observations) - 1; i >= 0; i-- {
		o := observations[i]
		if o.Kind != observation.KindMessage {
			continue
		}
		// best-effort extract of "message.content[*].text"
		var parsed struct {
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(o.Payload, &parsed); err != nil {
			continue
		}
		for _, c := range parsed.Message.Content {
			if c.Type == "text" && c.Text != "" {
				return c.Text
			}
		}
	}
	return ""
}

// scoreOne applies the case's scorer to the observed text and returns
// (passed, score in [0,1], reason).
func scoreOne(c eval.Case, observed string) (bool, float64, string) {
	expected := string(c.Expected)
	if len(expected) > 0 && expected[0] == '"' {
		expected = expected[1 : len(expected)-1]
	}
	switch c.Scorer {
	case eval.ScorerExact:
		if observed == expected {
			return true, 1.0, "exact match"
		}
		return false, 0.0, fmt.Sprintf("expected %q, got %q", expected, observed)
	case eval.ScorerContains:
		if strings.Contains(observed, expected) {
			return true, 1.0, "substring present"
		}
		return false, 0.0, fmt.Sprintf("expected to contain %q", expected)
	case eval.ScorerRegex:
		re, err := regexp.Compile(expected)
		if err != nil {
			return false, 0.0, "invalid regex: " + err.Error()
		}
		if re.MatchString(observed) {
			return true, 1.0, "regex matched"
		}
		return false, 0.0, "regex did not match"
	case eval.ScorerLLMJudge, eval.ScorerCustomFunc:
		// R3.x: real implementations. For now, score 0.5 with note.
		return false, 0.5, fmt.Sprintf("scorer %q not yet implemented (R3.x)", c.Scorer)
	}
	return false, 0.0, "unknown scorer"
}

// _ conformance.
var _ = json.Marshal
var _ = slog.Default