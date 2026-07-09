package runtime

import "github.com/hnsx-io/hnsx/core/observation"

// Type aliases so callers can keep using runtime.Observation / runtime.Cost
// without importing the observation package directly. The canonical
// definitions live in pkg/observation.

type (
	// Observation is the canonical observation event. See
	// pkg/observation.Observation for fields.
	Observation = observation.Observation

	// Cost captures per-call spend. See pkg/observation.Cost for fields.
	Cost = observation.Cost
)

// NewSessionID is a thin wrapper that delegates to observation.NewSessionID
// so internal call sites don't need to import the observation package.
func NewSessionID(domain string) string { return observation.NewSessionID(domain) }
