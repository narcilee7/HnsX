package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/domain/issue"
	issuesvc "github.com/hnsx-io/hnsx/server/internal/service/issue"
)

func newIssueCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "issue", Short: "Manage issues"}
	cmd.AddCommand(
		newIssueListCmd(),
		newIssueCreateCmd(),
		newIssueAssignCmd(),
	)
	return cmd
}

func newIssueListCmd() *cobra.Command {
	var workspaceID, status, assigneeID, output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues in a workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			f := issue.ListFilter{Status: issue.Status(status), Limit: 50}
			if assigneeID != "" {
				f.AssigneeID = &assigneeID
			}
			items, err := app.IssueSvc.ListByWorkspace(cmd.Context(), workspaceID, f)
			if err != nil {
				return err
			}
			renderIssues(cmd.OutOrStdout(), output, items)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	cmd.Flags().StringVar(&assigneeID, "assignee", "", "filter by assignee ID")
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	return cmd
}

func newIssueCreateCmd() *cobra.Command {
	var (
		workspaceID, title, desc, priority, creatorID, output string
		status                                                string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an issue",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			in := issuesvc.CreateInput{
				WorkspaceID: workspaceID,
				Title:       title,
				Description: desc,
				Status:      issue.Status(status),
				Priority:    issue.Priority(priority),
				CreatorType: issue.CreatorMember,
				CreatorID:   creatorID,
			}
			got, err := app.IssueSvc.Create(cmd.Context(), in)
			if err != nil {
				return err
			}
			renderIssues(cmd.OutOrStdout(), output, []*issue.Issue{got})
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&title, "title", "", "issue title (required)")
	cmd.Flags().StringVar(&desc, "description", "", "issue description")
	cmd.Flags().StringVar(&status, "status", "backlog", "initial status")
	cmd.Flags().StringVar(&priority, "priority", "none", "priority (urgent|high|medium|low|none)")
	cmd.Flags().StringVar(&creatorID, "creator", "", "creator member ID (required)")
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("creator")
	return cmd
}

func newIssueAssignCmd() *cobra.Command {
	var issueID, assigneeType, assigneeID string
	cmd := &cobra.Command{
		Use:   "assign <issue-id>",
		Short: "Assign an issue to an agent or member",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			issueID = args[0]
			at := issue.AssigneeType(assigneeType)
			got, err := app.IssueSvc.Assign(cmd.Context(), issueID, &at, &assigneeID)
			if err != nil {
				return err
			}
			renderIssues(cmd.OutOrStdout(), "human", []*issue.Issue{got})
			return nil
		},
	}
	cmd.Flags().StringVar(&assigneeType, "type", "agent", "assignee type (agent|member)")
	cmd.Flags().StringVar(&assigneeID, "to", "", "assignee ID (required for agent)")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func renderIssues(w io.Writer, format string, items []*issue.Issue) {
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
		fmt.Fprintln(tw, "ID\tNUMBER\tTITLE\tSTATUS\tASSIGNEE")
		for _, x := range items {
			asg := ""
			if x.AssigneeID != nil {
				asg = string(*x.AssigneeType) + ":" + *x.AssigneeID
			}
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n",
				x.ID, x.Number, x.Title, x.Status, asg)
		}
		_ = tw.Flush()
	}
}