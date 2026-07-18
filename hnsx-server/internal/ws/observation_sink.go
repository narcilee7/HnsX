package ws

import (
	"context"
	"errors"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/observation"
)

// ObservationSink implements observation.Sink over the daemon ↔ server
// WebSocket protocol. It is used by the daemon runtime so that observations
// are sent to the server first, and the server persists them to Postgres.
//
// A fallback Sink (typically the Postgres sink) is required for List* methods
// because the WS protocol is write-only for observations, and for Write when
// the WS connection is down.
type ObservationSink struct {
	client   *Client
	fallback observation.Sink
}

// NewObservationSink wires a WS client and a fallback sink. The fallback
// must implement the full observation.Sink interface; it is used for reads
// and for writes when the WS path fails.
func NewObservationSink(client *Client, fallback observation.Sink) *ObservationSink {
	return &ObservationSink{client: client, fallback: fallback}
}

// Write converts the observation to an ObservationEvent and sends it over
// the WebSocket. If the WS send fails and a fallback sink was configured,
// the observation is written to the fallback instead.
func (s *ObservationSink) Write(ctx context.Context, obs *observation.Observation) error {
	if obs == nil {
		return errors.New("ws observation sink: nil observation")
	}

	occurred := obs.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	}

	evt := ObservationEvent{
		ID:              obs.ID,
		WorkspaceID:     obs.WorkspaceID,
		IssueID:         obs.IssueID,
		AgentID:         obs.AgentID,
		Kind:            string(obs.Kind),
		Sequence:        obs.Sequence,
		Payload:         obs.Payload,
		OccurredAt:      occurred.Format(time.RFC3339Nano),
		PromptHash:      obs.PromptHash,
		AgentTemplateID: obs.AgentTemplateID,
		ToolSignatures:  obs.ToolSignatures,
		PolicyDecision:  string(obs.PolicyDecision),
		EvalRunID:       obs.EvalRunID,
	}

	if s.client != nil {
		if err := s.client.WriteObservations(ctx, []ObservationEvent{evt}); err == nil {
			return nil
		} else if s.fallback != nil {
			return s.fallback.Write(ctx, obs)
		}
		return nil
	}

	if s.fallback != nil {
		return s.fallback.Write(ctx, obs)
	}
	return errors.New("ws observation sink: no client or fallback")
}

// ListByIssue delegates to the fallback sink because the WS protocol is
// write-only for observations.
func (s *ObservationSink) ListByIssue(ctx context.Context, issueID string, limit int) ([]*observation.Observation, error) {
	if s.fallback == nil {
		return nil, errors.New("ws observation sink: ListByIssue requires a fallback")
	}
	return s.fallback.ListByIssue(ctx, issueID, limit)
}

// ListByEvalRun delegates to the fallback sink because the WS protocol is
// write-only for observations.
func (s *ObservationSink) ListByEvalRun(ctx context.Context, evalRunID string) ([]*observation.Observation, error) {
	if s.fallback == nil {
		return nil, errors.New("ws observation sink: ListByEvalRun requires a fallback")
	}
	return s.fallback.ListByEvalRun(ctx, evalRunID)
}

var _ observation.Sink = (*ObservationSink)(nil)
