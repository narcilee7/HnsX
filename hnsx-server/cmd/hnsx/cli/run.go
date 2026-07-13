package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// newRunCmd used to spawn a local Python worker via the in-process Go
// executor (pkg/runtime) and the embedded worker launcher (pkg/local).
// Both are deleted as of W16+ Phase 3. Local execution is now the user's
// choice between:
//
//   - `hnsx up`  + `hnsx deploy`  to send to a real worker (recommended)
//   - running `hnsx-worker` directly with the spec on stdin
//
// This stub stays so the `hnsx run` help text doesn't 404, but it returns
// a clear "removed" error.
func newRunCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run --domain <path>",
		Short: "[Removed in W16+ Phase 3] Use `hnsx deploy` to send to a worker",
		Long: `Removed in W16+ Phase 3.

The in-process Go executor (pkg/runtime) and the embedded worker
launcher (pkg/local) are gone. Local execution is now:

  hnsx up           # start the local server + worker
  hnsx deploy ...   # send your DomainSpec to the running worker

For offline use, run hnsx-worker directly with the spec on stdin.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf(
				"`hnsx run` removed in W16+ Phase 3; use `hnsx up` + `hnsx deploy` instead",
			)
		},
	}
	_ = cfg
	_ = (*domain.DomainSpec)(nil) // keep import (no-op; will be removed once we delete this stub)
	return cmd
}
