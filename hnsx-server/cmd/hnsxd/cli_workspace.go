package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/domain/workspace"
	workspacesvc "github.com/hnsx-io/hnsx/server/internal/service/workspace"
)

// newWorkspaceCmd wires `hnsxd workspace ...` against the HTTP API
// indirectly: the CLI shells out through the local hnsxd server, so we
// exercise the same path external consumers do.
//
// For R1.7 we keep the CLI minimal: list / get / create / archive /
// delete. Update and slug-based get land in R1.x follow-on.
func newWorkspaceCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "workspace", Short: "Manage workspaces"}
	cmd.AddCommand(
		newWorkspaceListCmd(),
		newWorkspaceGetCmd(),
		newWorkspaceCreateCmd(),
		newWorkspaceArchiveCmd(),
		newWorkspaceDeleteCmd(),
	)
	return cmd
}

func newWorkspaceListCmd() *cobra.Command {
	var status, output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspaces",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			f := workspace.ListFilter{Status: workspace.Status(status), Limit: 50}
			ws, err := app.WorkspaceSvc.List(cmd.Context(), f)
			if err != nil {
				return err
			}
			renderWorkspaces(cmd.OutOrStdout(), output, ws)
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status (active | archived)")
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	return cmd
}

func newWorkspaceGetCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a workspace by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			w, err := app.WorkspaceSvc.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			renderWorkspace(cmd.OutOrStdout(), output, w)
			return nil
		},
	}
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	return cmd
}

func newWorkspaceCreateCmd() *cobra.Command {
	var (
		name, slug, description, ctxStr, settingsRaw, output string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			w, err := app.WorkspaceSvc.Create(cmd.Context(), workspaceCreateInput(name, slug, description, ctxStr, settingsRaw))
			if err != nil {
				return err
			}
			renderWorkspace(cmd.OutOrStdout(), output, w)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "workspace name (required)")
	cmd.Flags().StringVar(&slug, "slug", "", "URL slug (required)")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	cmd.Flags().StringVar(&ctxStr, "context", "", "system prompt injected into agent runs")
	cmd.Flags().StringVar(&settingsRaw, "settings", "{}", "JSON settings object")
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("slug")
	return cmd
}

func newWorkspaceArchiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "archive <id>",
		Short: "Archive a workspace (soft delete)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			if err := app.WorkspaceSvc.Archive(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "archived %s\n", args[0])
			return nil
		},
	}
}

func newWorkspaceDeleteCmd() *cobra.Command {
	var confirm bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a workspace (cascades to owned resources)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return fmt.Errorf("refusing to delete without --confirm")
			}
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			if err := app.WorkspaceSvc.Delete(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "required to actually delete")
	return cmd
}

func workspaceCreateInput(name, slug, desc, ctxStr, settingsRaw string) workspacesvc.CreateInput {
	return workspacesvc.CreateInput{
		Name:        name,
		Slug:        slug,
		Description: desc,
		Context:     ctxStr,
		Settings:    []byte(settingsRaw),
	}
}

func renderWorkspaces(w io.Writer, format string, items []*workspace.Workspace) {
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
		fmt.Fprintln(tw, "ID\tNAME\tSLUG\tSTATUS\tCREATED")
		for _, x := range items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				x.ID, x.Name, x.Slug, x.Status, x.CreatedAt.Format("2006-01-02"))
		}
		_ = tw.Flush()
	}
}

func renderWorkspace(w io.Writer, format string, x *workspace.Workspace) {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(x)
	case "quiet":
		fmt.Fprintln(w, x.ID)
	default:
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "ID:\t%s\n", x.ID)
		fmt.Fprintf(tw, "Name:\t%s\n", x.Name)
		fmt.Fprintf(tw, "Slug:\t%s\n", x.Slug)
		fmt.Fprintf(tw, "Status:\t%s\n", x.Status)
		fmt.Fprintf(tw, "Description:\t%s\n", x.Description)
		fmt.Fprintf(tw, "Context:\t%s\n", x.Context)
		fmt.Fprintf(tw, "Created:\t%s\n", x.CreatedAt.Format("2006-01-02"))
		_ = tw.Flush()
	}
}

var _ = os.Stderr // keep import live