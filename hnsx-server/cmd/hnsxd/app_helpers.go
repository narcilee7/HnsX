package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/app"
)

// appFromCmd constructs an *app.Application from the cobra command's
// context. Centralized so every CLI subcommand goes through the same
// wiring path; tests can override the factory in the future.
func appFromCmd(cmd *cobra.Command) (*app.Application, error) {
	cfg := minimalConfig()
	if v, err := cmd.Flags().GetString("server"); err == nil && v != "" {
		// Reserved for future: --server flag pointing at a remote hnsxd.
		// For R1.7 the CLI always talks to the in-process application.
		_ = v
	}
	application, err := app.New(cmd.Context(), cfg)
	if err != nil {
		return nil, fmt.Errorf("hnsxd: init app: %w", err)
	}
	return application, nil
}