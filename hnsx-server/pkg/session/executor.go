package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/core/domain"
	"github.com/hnsx-io/hnsx/core/observation"
	"github.com/hnsx-io/hnsx/core/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/telemetry"
)

// Executor wires the runner, broadcaster, and telemetry sinks into a single
// component that the API layer calls per triggered session.
//
// Responsibilities:
//
//   - Generate a session ID.
//   - Run the domain spec via the runner.
//   - Mirror every observation into a per-session broadcaster (for SSE).
//   - Mirror every observation into all registered telemetry sinks.
//   - Persist a final session summary into the database (if a sink is wired).
type Executor struct {
	adapter   runtime.Adapter
	sinks     []telemetry.Sink
	broadcast *Broadcaster
	mu        sync.Mutex
}

// NewExecutor constructs an Executor bound to a single runtime.Adapter and zero or
// more telemetry sinks. The broadcaster is the same per-session broadcaster
// supplied by the API layer; it is shared (one broadcaster per session).
func NewExecutor(adapter runtime.Adapter, sinks ...telemetry.Sink) *Executor {
	return &Executor{
		adapter: adapter,
		sinks:   sinks,
	}
}

// WithBroadcaster attaches a per-session broadcaster. Required for SSE.
func (e *Executor) WithBroadcaster(b *Broadcaster) *Executor {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.broadcast = b
	return e
}

// Execute runs the domain spec synchronously and returns the result. It also
// publishes observations to the broadcaster (if attached) and the configured
// sinks. This call blocks until the session is done.
//
// Phase 1 keeps the runner mostly serial; future PRs will move execution to
// goroutines and surface cancellation via context.
func (e *Executor) Execute(ctx context.Context, spec *domain.DomainSpec, trigger map[string]any) (*runtime.Result, error) {
	if spec == nil {
		return nil, errors.New("executor: nil spec")
	}
	if e.adapter == nil {
		return nil, errors.New("executor: nil adapter")
	}

	runner := runtime.NewRunner(e.adapter)

	// Hook: pump observations into broadcaster + sinks.
	runner.WithHook(func(obs observation.Observation) {
		// Stamp session + domain IDs so subscribers don't need to infer them.
		obs.SessionID = runtime.SessionIDFromContext(ctx)
		if obs.SessionID == "" {
			obs.SessionID = runtime.NewSessionID(spec.ID)
		}
		obs.DomainID = spec.ID
		if obs.Timestamp.IsZero() {
			obs.Timestamp = time.Now().UTC()
		}
		e.publish(ctx, obs)
	})

	result, err := runner.Run(ctx, spec, trigger)
	if err != nil && result == nil {
		return nil, err
	}
	return result, err
}

func (e *Executor) publish(ctx context.Context, obs observation.Observation) {
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
