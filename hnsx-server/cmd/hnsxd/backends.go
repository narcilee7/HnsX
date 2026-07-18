package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/app"
)

// newBackendsCmd exposes the agentruntime registry for smoke tests and
// operator debugging. Listing registered backends proves that
// app.New() successfully wired the infrastructure layer.
func newBackendsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backends",
		Short: "Inspect registered agent runtime backends (claude / codex / ...)",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all registered agent runtime backends",
		RunE: func(cmd *cobra.Command, _ []string) error {
			application, err := app.New(cmd.Context(), minimalConfig())
			if err != nil {
				return err
			}
			defer application.Close()

			names := application.Backends.List()
			if len(names) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no backends registered)")
				return nil
			}
			for _, name := range names {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n", name)
			}
			return nil
		},
	}

	cmd.AddCommand(listCmd)
	return cmd
}