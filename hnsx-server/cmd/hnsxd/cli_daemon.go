package main

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/domain/daemon"
	daemonsvc "github.com/hnsx-io/hnsx/server/internal/service/daemon"
)

func newDaemonCmdGroup() *cobra.Command {
	cmd := &cobra.Command{Use: "daemon-cmd", Short: "Manage daemon registrations (the cli-side view)"}
	cmd.AddCommand(
		newDaemonListCmd(),
		newDaemonRegisterCmd(),
		newDaemonHeartbeatCmd(),
	)
	return cmd
}

func newDaemonListCmd() *cobra.Command {
	var workspaceID, output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List daemons registered against a workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			items, err := app.DaemonSvc.ListByWorkspace(cmd.Context(), workspaceID)
			if err != nil {
				return err
			}
			renderDaemons(cmd.OutOrStdout(), output, items)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	return cmd
}

func newDaemonRegisterCmd() *cobra.Command {
	var workspaceID, name, output string
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register the local hnsxd daemon with a workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			in := daemonsvc.RegisterInput{
				WorkspaceID: workspaceID,
				Name:        name,
				Platform:    runtime.GOOS,
				OS:          runtime.GOOS,
				Version:     "0.1.0",
			}
			got, err := app.DaemonSvc.Register(cmd.Context(), in)
			if err != nil {
				return err
			}
			renderDaemons(cmd.OutOrStdout(), output, []*daemon.Daemon{got})
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "daemon name (required)")
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newDaemonHeartbeatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "heartbeat <id>",
		Short: "Send a heartbeat for a daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			if err := app.DaemonSvc.Heartbeat(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "heartbeat %s @ %s\n", args[0], time.Now().UTC().Format(time.RFC3339))
			return nil
		},
	}
}

func renderDaemons(w io.Writer, format string, items []*daemon.Daemon) {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(items)
	case "quiet":
		for _, x := range items {
			fmt.Fprintln(w, x.ID)
		}
	default:
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tNAME\tPLATFORM\tSTATUS\tLAST_HEARTBEAT")
		for _, x := range items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				x.ID, x.Name, x.Platform, x.Status,
				x.LastHeartbeat.Format("2006-01-02 15:04:05"))
		}
		_ = tw.Flush()
	}
}