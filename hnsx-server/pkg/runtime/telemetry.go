package runtime

import "context"

// Sink is the contract any telemetry backend must satisfy. Implementations
// live in internal/telemetry; pkg/runtime only declares the interface so the
// executor and runner can stay free of infrastructure dependencies.
//
// Implementations MUST be safe for concurrent use.
type Sink interface {
	// Name returns the sink identifier for logging/metrics.
	Name() string
	// Record is called for each observation the runtime emits.
	Record(ctx context.Context, obs Observation) error
	// Flush forces any buffered events to the backend. Best-effort; nil
	// return value indicates success.
	Flush(ctx context.Context) error
	// Close releases any resources (spans, exporters, etc.).
	Close(ctx context.Context) error
}
