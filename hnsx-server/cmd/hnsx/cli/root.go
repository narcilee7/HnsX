package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewRootCmd constructs the hnsx root cobra command. It binds persistent
// flags, registers subcommands, and returns the assembled tree.
//
// Adding a new top-level command: implement it in its own file and call the
// constructor from here. Keep command construction cheap; expensive work
// belongs in RunE.
func NewRootCmd() *cobra.Command {
	cfg := Default()

	cmd := &cobra.Command{
		Use:   "hnsx",
		Short: "Harness for Autonomous Agents",
		Long: `hnsx is the operator-facing CLI for HnsX.

It exposes local commands (validate, run, format) and remote commands
that talk to a running hnsx-server (domain, session, eval, trace, ...).
Lifecycle commands (up, down, status, doctor) orchestrate the local
Postgres + server + worker stack.

Use "hnsx <command> --help" for per-command details.
`,
		SilenceUsage:  true,
		SilenceErrors: false,
		// NoArgs -> show help (better UX than silent exit 1).
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	BindPersistentFlags(cmd, &cfg)
	cmd.SetVersionTemplate("hnsx {{.Version}}\n")

	// Lifecycle — v0.3 Lifesaver.
	cmd.AddCommand(newUpCmd(&cfg))
	cmd.AddCommand(newDownCmd(&cfg))
	cmd.AddCommand(newRestartCmd(&cfg))
	cmd.AddCommand(newStatusCmd(&cfg))
	cmd.AddCommand(newDoctorCmd(&cfg))
	cmd.AddCommand(newLogsCmd(&cfg))
	cmd.AddCommand(newResetCmd(&cfg))

	// Discovery — v0.3 Lifesaver.
	cmd.AddCommand(newTryCmd(&cfg))
	cmd.AddCommand(newExamplesCmd(&cfg))
	cmd.AddCommand(newCompletionCmd())

	// Existing local commands (preserved, refactored to cobra).
	cmd.AddCommand(newValidateCmd(&cfg))
	cmd.AddCommand(newRunCmd(&cfg))

	// v0.4 Operator: resource-oriented command groups.
	cmd.AddCommand(newDomainCmd(&cfg))
	cmd.AddCommand(newSessionCmd(&cfg))
	cmd.AddCommand(newTraceCmd(&cfg))
	cmd.AddCommand(newEvalCmd(&cfg))

	// Remote commands (preserved; deprecation warnings begin in v0.4).
	cmd.AddCommand(newRemoteCmd(&cfg))

	// Meta.
	cmd.AddCommand(newVersionCmd())

	return cmd
}

// Execute is the entrypoint used by main.go.
func Execute() error {
	cmd := NewRootCmd()
	if err := cmd.Execute(); err != nil {
		// cobra already prints the error; main only needs the exit code.
		return fmt.Errorf("hnsx: %w", err)
	}
	return nil
}