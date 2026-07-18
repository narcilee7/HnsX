package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/domain/harness"
)

func newHarnessCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "harness", Short: "Manage harnesses (Agent + Prompts + Skills + Tools + Policy + EvalSet bundles)"}
	cmd.AddCommand(
		newHarnessListCmd(),
		newHarnessGetCmd(),
		newHarnessCreateCmd(),
		newHarnessDeleteCmd(),
	)
	return cmd
}

func newHarnessListCmd() *cobra.Command {
	var workspaceID, output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List harnesses in a workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			items, err := app.HarnessSvc.ListByWorkspace(cmd.Context(), workspaceID)
			if err != nil {
				return err
			}
			renderHarnesses(cmd.OutOrStdout(), output, items)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	return cmd
}

func newHarnessGetCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a harness by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			got, err := app.HarnessSvc.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			renderHarness(cmd.OutOrStdout(), output, got)
			return nil
		},
	}
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	return cmd
}

func newHarnessCreateCmd() *cobra.Command {
	var (
		workspaceID, name, desc, promptsRaw, skillsRaw, toolsRaw, version, output string
		policyID, evalSetID                                                     string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a harness",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := appFromCmd(cmd)
			if err != nil {
				return err
			}
			defer app.Close()
			prompts, err := decodeJSONList[harness.Prompt](promptsRaw)
			if err != nil {
				return fmt.Errorf("prompts: %w", err)
			}
			skills, err := decodeJSONList[harness.SkillRef](skillsRaw)
			if err != nil {
				return fmt.Errorf("skills: %w", err)
			}
			tools, err := decodeJSONList[harness.ToolRef](toolsRaw)
			if err != nil {
				return fmt.Errorf("tools: %w", err)
			}
			in := harnessCreateInput(workspaceID, name, desc, prompts, skills, tools, policyID, evalSetID, version)
			got, err := app.HarnessSvc.Create(cmd.Context(), in)
			if err != nil {
				return err
			}
			renderHarness(cmd.OutOrStdout(), output, got)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "harness name (required)")
	cmd.Flags().StringVar(&desc, "description", "", "description")
	cmd.Flags().StringVar(&promptsRaw, "prompts", "[]", "JSON array of prompts")
	cmd.Flags().StringVar(&skillsRaw, "skills", "[]", "JSON array of skill refs")
	cmd.Flags().StringVar(&toolsRaw, "tools", "[]", "JSON array of tool refs")
	cmd.Flags().StringVar(&policyID, "policy-id", "", "policy ID (optional)")
	cmd.Flags().StringVar(&evalSetID, "eval-set-id", "", "eval set ID (optional)")
	cmd.Flags().StringVar(&version, "version", "1.0.0", "harness version")
	cmd.Flags().StringVar(&output, "output", "human", "output format: human | json | quiet")
	_ = cmd.MarkFlagRequired("workspace")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newHarnessDeleteCmd() *cobra.Command {
	var confirm bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a harness",
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
			if err := app.HarnessSvc.Delete(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "required to actually delete")
	return cmd
}

func harnessCreateInput(workspaceID, name, desc string, prompts []harness.Prompt, skills []harness.SkillRef, tools []harness.ToolRef, policyID, evalSetID, version string) harnessCreateInputT {
	in := harnessCreateInputT{
		WorkspaceID: workspaceID,
		Name:        name,
		Description: desc,
		Prompts:     prompts,
		Skills:      skills,
		Tools:       tools,
		Version:     version,
	}
	if policyID != "" {
		in.PolicyID = &policyID
	}
	if evalSetID != "" {
		in.EvalSetID = &evalSetID
	}
	return in
}

type harnessCreateInputT = struct {
	WorkspaceID string
	Name        string
	Description string
	Prompts     []harness.Prompt
	Skills      []harness.SkillRef
	Tools       []harness.ToolRef
	PolicyID    *string
	EvalSetID   *string
	Version     string
}

func decodeJSONList[T any](raw string) ([]T, error) {
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var out []T
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func renderHarnesses(w io.Writer, format string, items []*harness.Harness) {
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
		fmt.Fprintln(tw, "ID\tNAME\tVERSION")
		for _, x := range items {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", x.ID, x.Name, x.Version)
		}
		_ = tw.Flush()
	}
}

func renderHarness(w io.Writer, format string, x *harness.Harness) {
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
		fmt.Fprintf(tw, "Version:\t%s\n", x.Version)
		fmt.Fprintf(tw, "Description:\t%s\n", x.Description)
		if x.PolicyID != nil {
			fmt.Fprintf(tw, "PolicyID:\t%s\n", *x.PolicyID)
		}
		if x.EvalSetID != nil {
			fmt.Fprintf(tw, "EvalSetID:\t%s\n", *x.EvalSetID)
		}
		fmt.Fprintf(tw, "Prompts:\t%s\n", stringOrEmpty(x.Prompts))
		fmt.Fprintf(tw, "Skills:\t%s\n", stringOrEmpty(x.Skills))
		fmt.Fprintf(tw, "Tools:\t%s\n", stringOrEmpty(x.Tools))
		_ = tw.Flush()
	}
}

func stringOrEmpty(b []byte) string {
	if len(b) == 0 {
		return "(none)"
	}
	return string(b)
}