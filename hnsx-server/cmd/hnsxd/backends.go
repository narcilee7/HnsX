package main

import (
	"fmt"

	"github.com/spf13/cobra"

	agentinfra "github.com/hnsx-io/hnsx/server/internal/infra/agentruntime"
)

// newBackendsCmd exposes the agentruntime registry for smoke tests and
// operator debugging. Listing registered backends proves the agent
// runtime layer is wired correctly.
//
// This command deliberately bypasses app.New (which would try to open
// Postgres + wire the gin router) so it stays usable without a database
// — useful for ops debugging in a broken-DB scenario.
func newBackendsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backends",
		Short: "Inspect registered agent runtime backends (claude / codex / ...)",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all registered agent runtime backends",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := minimalConfig()
			registry := agentinfra.NewRegistry(nil)
			claudeRunner := agentinfra.NewClaudeRunner(cfg.ClaudeExecutable, nil)
			registry.Register(agentinfra.NewClaudeBackend(claudeRunner))

			names := registry.List()
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