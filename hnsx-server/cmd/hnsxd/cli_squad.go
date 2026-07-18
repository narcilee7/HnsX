package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/domain/squad"
	squadsvc "github.com/hnsx-io/hnsx/server/internal/service/squad"
)

func newSquadCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "squad", Short: "Manage squads"}
	cmd.AddCommand(
		newSquadListCmd(),
		newSquadCreateCmd(),
	)
	return cmd
}

func newSquadListCmd() *cobra.Command {
	var workspaceID, output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List squads in a workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			items, err := app.SquadSvc.ListByWorkspace(cmd.Context(), workspaceID)
			if err != nil {
				return err
			}
			renderSquads(cmd.OutOrStdout(), output, items)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	return cmd
}

func newSquadCreateCmd() *cobra.Command {
	var (
		workspaceID, name, desc, leaderID, output string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a squad (a named group of agents)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			in := squadsvc.CreateInput{
				WorkspaceID: workspaceID,
				Name:        name,
				Description: desc,
				Members: []squad.Member{
					{ID: leaderID, Kind: squad.KindAgent, Role: squad.RoleLeader},
				},
			}
			got, err := app.SquadSvc.Create(cmd.Context(), in)
			if err != nil {
				return err
			}
			renderSquads(cmd.OutOrStdout(), output, []*squad.Squad{got})
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "squad name (required)")
	cmd.Flags().StringVar(&desc, "description", "", "squad description")
	cmd.Flags().StringVar(&leaderID, "leader", "", "leader agent ID (required)")
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("leader")
	return cmd
}

func renderSquads(w io.Writer, format string, items []*squad.Squad) {
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
		fmt.Fprintln(tw, "ID\tNAME\tMEMBERS")
		for _, x := range items {
			fmt.Fprintf(tw, "%s\t%s\t%d\n", x.ID, x.Name, len(x.Members))
		}
		_ = tw.Flush()
	}
}