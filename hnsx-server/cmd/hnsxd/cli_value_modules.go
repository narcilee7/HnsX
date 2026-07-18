package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/domain/approval"
	"github.com/hnsx-io/hnsx/server/internal/domain/eval"
	"github.com/hnsx-io/hnsx/server/internal/domain/policy"
	evalsvc "github.com/hnsx-io/hnsx/server/internal/service/eval"
)

// ---- policy ----

func newPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "policy", Short: "Manage policy rule stacks"}
	cmd.AddCommand(newPolicyListCmd(), newPolicyCreateCmd())
	return cmd
}

func newPolicyListCmd() *cobra.Command {
	var workspaceID, output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List policies in a workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			items, err := app.PolicySvc.ListByWorkspace(cmd.Context(), workspaceID)
			if err != nil {
				return err
			}
			renderPolicies(cmd.OutOrStdout(), output, items)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&output, "output", "human", "human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	return cmd
}

func newPolicyCreateCmd() *cobra.Command {
	var workspaceID, name, desc, rulesRaw, output string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a policy with a JSON rules array",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			rules, err := decodeJSONList[policy.Rule](rulesRaw)
			if err != nil {
				return fmt.Errorf("rules: %w", err)
			}
			in := policyCreateInput(workspaceID, name, desc, rules)
			got, err := app.PolicySvc.Create(cmd.Context(), in)
			if err != nil {
				return err
			}
			renderPolicies(cmd.OutOrStdout(), output, []*policy.Policy{got})
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "policy name (required)")
	cmd.Flags().StringVar(&desc, "description", "", "")
	cmd.Flags().StringVar(&rulesRaw, "rules", "[]", "JSON array of rule objects")
	cmd.Flags().StringVar(&output, "output", "human", "human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

type policyCreateInputT = struct {
	WorkspaceID string
	Name        string
	Description string
	Rules       []policy.Rule
}

func policyCreateInput(workspaceID, name, desc string, rules []policy.Rule) policyCreateInputT {
	return policyCreateInputT{WorkspaceID: workspaceID, Name: name, Description: desc, Rules: rules}
}

func renderPolicies(w io.Writer, format string, items []*policy.Policy) {
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
		fmt.Fprintln(tw, "ID\tNAME\tRULES_COUNT")
		for _, x := range items {
			rs, _ := x.RulesTyped()
			fmt.Fprintf(tw, "%s\t%s\t%d\n", x.ID, x.Name, len(rs))
		}
		_ = tw.Flush()
	}
}

// ---- eval ----

func newEvalCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "eval-set", Short: "Manage eval sets + runs"}
	cmd.AddCommand(newEvalSetListCmd(), newEvalSetCreateCmd(), newEvalSetRunsCmd())
	return cmd
}

func newEvalSetListCmd() *cobra.Command {
	var workspaceID, output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List eval sets in a workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			items, err := app.EvalSvc.ListSetsByWorkspace(cmd.Context(), workspaceID)
			if err != nil {
				return err
			}
			renderEvalSets(cmd.OutOrStdout(), output, items)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&output, "output", "human", "human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	return cmd
}

func newEvalSetCreateCmd() *cobra.Command {
	var workspaceID, name, desc, casesRaw, version, output string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an eval set",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			cases, err := decodeJSONList[eval.Case](casesRaw)
			if err != nil {
				return fmt.Errorf("cases: %w", err)
			}
			in := evalsvc.CreateSetInput{
				WorkspaceID: workspaceID,
				Name:        name,
				Description: desc,
				Cases:       cases,
				Version:     version,
			}
			got, err := app.EvalSvc.CreateSet(cmd.Context(), in)
			if err != nil {
				return err
			}
			renderEvalSets(cmd.OutOrStdout(), output, []*eval.EvalSet{got})
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "eval set name (required)")
	cmd.Flags().StringVar(&desc, "description", "", "")
	cmd.Flags().StringVar(&casesRaw, "cases", "[]", "JSON array of cases")
	cmd.Flags().StringVar(&version, "version", "1.0.0", "")
	cmd.Flags().StringVar(&output, "output", "human", "human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newEvalSetRunsCmd() *cobra.Command {
	var evalSetID, output string
	cmd := &cobra.Command{
		Use:   "runs <eval-set-id>",
		Short: "List runs for an eval set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			evalSetID = args[0]
			items, err := app.EvalSvc.ListRuns(cmd.Context(), evalSetID, 20)
			if err != nil {
				return err
			}
			renderRuns(cmd.OutOrStdout(), output, items)
			return nil
		},
	}
	cmd.Flags().StringVar(&output, "output", "human", "human | json | quiet")
	return cmd
}

func renderEvalSets(w io.Writer, format string, items []*eval.EvalSet) {
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
		fmt.Fprintln(tw, "ID\tNAME\tVERSION\tCASES")
		for _, x := range items {
			cs, _ := x.CasesTyped()
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", x.ID, x.Name, x.Version, len(cs))
		}
		_ = tw.Flush()
	}
}

func renderRuns(w io.Writer, format string, items []*eval.Run) {
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
		fmt.Fprintln(tw, "ID\tSTATUS\tSCORE\tERROR")
		for _, x := range items {
			fmt.Fprintf(tw, "%s\t%s\t%.2f\t%s\n", x.ID, x.Status, x.TotalScore, x.Error)
		}
		_ = tw.Flush()
	}
}

// ---- approval ----

func newApprovalCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "approval", Short: "Manage approvals (human-in-the-loop gates)"}
	cmd.AddCommand(newApprovalListCmd(), newApprovalGrantCmd(), newApprovalDenyCmd())
	return cmd
}

func newApprovalListCmd() *cobra.Command {
	var workspaceID, output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pending approvals in a workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			items, err := app.ApprovalSvc.ListPending(cmd.Context(), workspaceID)
			if err != nil {
				return err
			}
			renderApprovals(cmd.OutOrStdout(), output, items)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&output, "output", "human", "human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	return cmd
}

func newApprovalGrantCmd() *cobra.Command {
	var userID string
	cmd := &cobra.Command{
		Use:   "grant <approval-id>",
		Short: "Grant a pending approval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			if _, err := app.ApprovalSvc.Grant(cmd.Context(), args[0], userID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "granted %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&userID, "user", "", "user ID granting approval (required)")
	_ = cmd.MarkFlagRequired("user")
	return cmd
}

func newApprovalDenyCmd() *cobra.Command {
	var userID string
	cmd := &cobra.Command{
		Use:   "deny <approval-id>",
		Short: "Deny a pending approval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			if _, err := app.ApprovalSvc.Deny(cmd.Context(), args[0], userID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "denied %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&userID, "user", "", "user ID denying approval (required)")
	_ = cmd.MarkFlagRequired("user")
	return cmd
}

func renderApprovals(w io.Writer, format string, items []*approval.Approval) {
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
		fmt.Fprintln(tw, "ID\tISSUE\tAGENT\tACTION\tSTATUS")
		for _, x := range items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", x.ID, x.IssueID, x.AgentID, x.Action, x.Status)
		}
		_ = tw.Flush()
	}
}