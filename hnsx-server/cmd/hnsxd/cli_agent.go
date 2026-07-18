package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/domain/agent"
	agentsvc "github.com/hnsx-io/hnsx/server/internal/service/agent"
)

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Manage agents"}
	cmd.AddCommand(
		newAgentListCmd(),
		newAgentCreateCmd(),
		newAgentArchiveCmd(),
	)
	return cmd
}

func newAgentListCmd() *cobra.Command {
	var workspaceID, status, output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agents in a workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			f := agent.ListFilter{Status: agent.Status(status), Limit: 50}
			items, err := app.AgentSvc.ListByWorkspace(cmd.Context(), workspaceID, f)
			if err != nil {
				return err
			}
			renderAgents(cmd.OutOrStdout(), output, items)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	return cmd
}

func newAgentCreateCmd() *cobra.Command {
	var (
		workspaceID, name, desc, runtimeMode, visibility, output string
		maxTasks                                                  int
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an agent in a workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			in := agentsvc.CreateInput{
				WorkspaceID:        workspaceID,
				Name:               name,
				Description:        desc,
				RuntimeMode:        agent.RuntimeMode(runtimeMode),
				Visibility:         agent.Visibility(visibility),
				MaxConcurrentTasks: maxTasks,
			}
			got, err := app.AgentSvc.Create(cmd.Context(), in)
			if err != nil {
				return err
			}
			renderAgents(cmd.OutOrStdout(), output, []*agent.Agent{got})
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "agent name (required)")
	cmd.Flags().StringVar(&desc, "description", "", "agent description")
	cmd.Flags().StringVar(&runtimeMode, "runtime", "local", "runtime mode (local|cloud)")
	cmd.Flags().StringVar(&visibility, "visibility", "workspace", "visibility (workspace|private)")
	cmd.Flags().IntVar(&maxTasks, "max-tasks", 1, "max concurrent tasks")
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newAgentArchiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "archive <id>",
		Short: "Archive an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			if err := app.AgentSvc.Archive(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "archived %s\n", args[0])
			return nil
		},
	}
}

func renderAgents(w io.Writer, format string, items []*agent.Agent) {
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
		fmt.Fprintln(tw, "ID\tNAME\tRUNTIME\tSTATUS\tVISIBILITY")
		for _, x := range items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				x.ID, x.Name, x.RuntimeMode, x.Status, x.Visibility)
		}
		_ = tw.Flush()
	}
}