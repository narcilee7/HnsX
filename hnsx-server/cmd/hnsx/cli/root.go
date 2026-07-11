package cli

import (
	"fmt"
	"os"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// isTerminal reports whether stdout is a terminal.
func isTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

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
		// NoArgs -> launch TUI when running in a terminal, otherwise show help.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !cfg.NoTui && isTerminal() {
				return tui.Run(cfg.ServerURL)
			}
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

	// v0.5 Bridge: surface commands. TUI is the default no-arg entry point;
	// console remains an explicit command.
	cmd.AddCommand(newConsoleCmd(&cfg))
	cmd.AddCommand(newUpdateCmd(&cfg))

	// v0.6 Governance: policy / secret / approval / audit / auth.
	cmd.AddCommand(newGovernanceCmd(&cfg))

	// v0.7 Power: format / diff / replay / debug bundle / plugin.
	cmd.AddCommand(newPowerCmd(&cfg))

	// v1.0 Product: telemetry opt-in.
	cmd.AddCommand(newTelemetryCmd(&cfg))

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