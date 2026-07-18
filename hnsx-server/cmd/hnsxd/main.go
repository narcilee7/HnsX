// Command hnsxd is the single HnsX binary. It can run in any of these modes:
//
//	hnsxd serve                 — HTTP + WebSocket server (default)
//	hnsxd daemon                — agent runtime (claims issues, spawns CLIs)
//	hnsxd workspace ...         — CLI: workspace subcommands
//	hnsxd agent ...             — CLI: agent subcommands
//	hnsxd issue ...             — CLI: issue subcommands
//	hnsxd squad ...             — CLI: squad subcommands
//	hnsxd daemon install        — install daemon as a system service (launchd/systemd)
//
// Phase R1 only wires up the binary skeleton + minimal `serve` so the HTTP
// engine boots. Resource subcommands land in R1.5-R1.7.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/app"
)

func main() {
	if err := root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "hnsxd:", err)
		os.Exit(1)
	}
}

func root() *cobra.Command {
	root := &cobra.Command{
		Use:   "hnsxd",
		Short: "HnsX unified binary: control plane + agent runtime + CLI",
		Long: `hnsxd is the single HnsX binary. Run 'hnsxd serve' to start the HTTP/WS
control plane, 'hnsxd daemon' to spawn the agent runtime, or invoke a resource
subcommand (workspace, agent, issue, squad) to manage state from the CLI.

Configuration is read from environment variables (HNSX_*) and an optional
config file at ~/.config/hnsxd/config.yaml.`,
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	root.AddCommand(serveCmd())
	root.AddCommand(daemonCmd())
	root.AddCommand(newBackendsCmd())
	root.AddCommand(newWorkspaceCmd())
	root.AddCommand(newIssueCmd())
	root.AddCommand(newAgentCmd())
	root.AddCommand(newSquadCmd())
	root.AddCommand(newDaemonCmdGroup())

	return root
}

func serveCmd() *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the HTTP + WebSocket control plane",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := app.LoadConfig(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			application, err := app.New(cmd.Context(), cfg)
			if err != nil {
				return fmt.Errorf("init app: %w", err)
			}
			defer application.Close()

			return application.Serve(cmd.Context())
		},
	}
	cmd.Flags().StringVar(&cfgPath, "config", "", "path to YAML config file")

	return cmd
}

func daemonCmd() *cobra.Command {
	var (
		workspaceID string
		tickSeconds int
	)
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the agent runtime loop (claims issues, spawns CLIs, writes observations)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			application, err := app.New(cmd.Context(), minimalConfig())
			if err != nil {
				return fmt.Errorf("hnsxd daemon: init app: %w", err)
			}
			defer application.Close()
			if application.DaemonRuntime == nil {
				return fmt.Errorf("hnsxd daemon: not started — HNSX_POSTGRES_DSN is not set")
			}
			if workspaceID == "" {
				return fmt.Errorf("hnsxd daemon: --workspace is required")
			}
			tick := time.Duration(tickSeconds) * time.Second
			return application.DaemonRuntime.Run(cmd.Context(), workspaceID, tick)
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID this daemon services (required)")
	cmd.Flags().IntVar(&tickSeconds, "tick-seconds", 5, "how often to poll for new issues")
	return cmd
}