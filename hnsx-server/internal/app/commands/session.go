package commands

import (
	"context"

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// RunLocalSession executes a DomainSpec locally using the given adapter.
// This is the shared backend for `hnsx run` and server-side local execution.
func RunLocalSession(ctx context.Context, spec *spec.DomainSpec, trigger map[string]any, adapter runtime.Adapter) (*runtime.Result, error) {
	runner := runtime.NewRunner(adapter)
	return runner.Run(ctx, spec, trigger)
}

// PickAdapter returns a built-in adapter by kind.
func PickAdapter(kind string) (runtime.Adapter, error) {
	switch kind {
	case "noop", "":
		return nil, nil // caller provides noop
	case "echo":
		return nil, nil // caller provides echo
	default:
		return nil, nil
	}
}
