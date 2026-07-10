package local

import (
	"context"
	"fmt"

	"github.com/hnsx-io/hnsx/server/pkg/adapter"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// RunLocalSession executes a DomainSpec locally using the given adapter.
// This is the shared backend for `hnsx run` and server-side local execution.
func RunLocalSession(ctx context.Context, s *spec.DomainSpec, trigger map[string]any, a runtime.Adapter) (*runtime.Result, error) {
	runner := runtime.NewRunner(a)
	return runner.Run(ctx, s, trigger)
}

// PickAdapter returns a built-in adapter by kind.
func PickAdapter(kind string) (runtime.Adapter, error) {
	switch kind {
	case "noop", "":
		return adapter.NewNoopAdapter(), nil
	case "echo":
		return adapter.NewEchoAdapter(), nil
	default:
		return nil, fmt.Errorf("unknown adapter: %s (built-in: noop, echo)", kind)
	}
}
