package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/internal/client"
)

func newEvalCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Manage Eval resources",
	}
	cmd.AddCommand(newEvalSetCmd(cfg))
	cmd.AddCommand(newEvalRunCmd(cfg))
	return cmd
}

func newEvalSetCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Manage EvalSets",
	}
	cmd.AddCommand(newEvalSetListCmd(cfg))
	cmd.AddCommand(newEvalSetShowCmd(cfg))
	cmd.AddCommand(newEvalSetCreateCmd(cfg))
	cmd.AddCommand(newEvalSetDeleteCmd(cfg))
	return cmd
}

func newEvalSetListCmd(cfg *Config) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List eval sets",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			items, err := c.ListEvalSets()
			if err != nil {
				return err
			}
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}
			o := NewOutput(cfg.Output)
			if cfg.Output == "json" {
				o.Print(items)
				return nil
			}
			if cfg.Output == "quiet" {
				for _, e := range items {
					o.Line("%s", e.ID)
				}
				return nil
			}
			rows := make([][]string, 0, len(items))
			for _, e := range items {
				rows = append(rows, []string{e.ID, e.DomainID, truncate(nonEmpty(e.Description, "-"), 50)})
			}
			o.Table([]string{"SET", "DOMAIN", "DESCRIPTION"}, rows)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "max number of items (0 = unlimited)")
	return cmd
}

func newEvalSetShowCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show an eval set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			set, err := c.GetEvalSet(args[0])
			if err != nil {
				return err
			}
			NewOutput(cfg.Output).Print(set)
			return nil
		},
	}
	return cmd
}

func newEvalSetCreateCmd(cfg *Config) *cobra.Command {
	var (
		setID, domainID, description, file string
	)
	cmd := &cobra.Command{
		Use:   "create --set-id <id> --domain <id> [--file <cases.yaml|json>]",
		Short: "Create an eval set",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			var cases []client.EvalCase
			if file != "" {
				b, err := os.ReadFile(file)
				if err != nil {
					return fmt.Errorf("read cases file: %w", err)
				}
				if err := json.Unmarshal(b, &cases); err != nil {
					return fmt.Errorf("parse cases file: %w", err)
				}
			}
			set, err := c.CreateEvalSet(setID, domainID, description, cases)
			if err != nil {
				return err
			}
			NewOutput(cfg.Output).Print(set)
			return nil
		},
	}
	cmd.Flags().StringVar(&setID, "set-id", "", "eval set id")
	cmd.Flags().StringVar(&domainID, "domain", "", "domain id")
	cmd.Flags().StringVar(&description, "description", "", "human-readable description")
	cmd.Flags().StringVar(&file, "file", "", "path to cases JSON file")
	return cmd
}

func newEvalSetDeleteCmd(cfg *Config) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an eval set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			if !yes {
				NewOutput(cfg.Output).Line("⚠ re-run with --yes to confirm deletion of %s", args[0])
				return nil
			}
			if err := c.DeleteEvalSet(args[0]); err != nil {
				return err
			}
			NewOutput(cfg.Output).Line("✓ deleted %s", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

func newEvalRunCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Manage EvalRuns",
	}
	cmd.AddCommand(newEvalRunStartCmd(cfg))
	cmd.AddCommand(newEvalRunListCmd(cfg))
	cmd.AddCommand(newEvalRunShowCmd(cfg))
	cmd.AddCommand(newEvalRunDiffCmd(cfg))
	return cmd
}

func newEvalRunStartCmd(cfg *Config) *cobra.Command {
	var baseline string
	cmd := &cobra.Command{
		Use:   "start <set-id> [--baseline <run-id>]",
		Short: "Run an eval set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			run, err := c.RunEval(args[0])
			if err != nil {
				return err
			}
			o := NewOutput(cfg.Output)
			o.Print(run)
			if baseline != "" && cfg.Output != "quiet" {
				o.Line("\n→ baseline set: %s; use `hnsx eval run diff %s %s %s` to compare", baseline, args[0], baseline, run.ID)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&baseline, "baseline", "", "baseline run id (used by later diff)")
	return cmd
}

func newEvalRunListCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <set-id>",
		Short: "List runs of an eval set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			runs, err := c.ListEvalRuns(args[0])
			if err != nil {
				return err
			}
			o := NewOutput(cfg.Output)
			if cfg.Output == "json" {
				o.Print(runs)
				return nil
			}
			rows := make([][]string, 0, len(runs))
			for _, r := range runs {
				rows = append(rows, []string{r.ID, r.State, nonEmpty(r.CreatedAt, "-")})
			}
			o.Table([]string{"RUN", "STATE", "CREATED"}, rows)
			return nil
		},
	}
	return cmd
}

func newEvalRunShowCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <set-id> <run-id>",
		Short: "Show an eval run report",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			run, err := c.GetEvalRun(args[0], args[1])
			if err != nil {
				return err
			}
			NewOutput(cfg.Output).Print(run)
			return nil
		},
	}
	return cmd
}

// newEvalRunDiffCmd compares two eval runs. v0.4 emits a JSON object
// summarising case-level deltas; v0.7 will swap in a richer renderer.
func newEvalRunDiffCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <set-id> <run-a> <run-b>",
		Short: "Diff two eval runs (case-level deltas)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			a, err := c.GetEvalRun(args[0], args[1])
			if err != nil {
				return err
			}
			b, err := c.GetEvalRun(args[0], args[2])
			if err != nil {
				return err
			}
			type delta struct {
				CaseID string  `json:"case_id"`
				AScore float64 `json:"a_score"`
				BScore float64 `json:"b_score"`
				Delta  float64 `json:"delta"`
			}
			scores := func(run *client.EvalRun) map[string]float64 {
				out := map[string]float64{}
				for _, r := range run.Cases {
					out[r.CaseID] = r.Score
				}
				return out
			}
			am, bm := scores(a), scores(b)
			seen := map[string]bool{}
			var deltas []delta
			for id, av := range am {
				bv := bm[id]
				deltas = append(deltas, delta{CaseID: id, AScore: av, BScore: bv, Delta: bv - av})
				seen[id] = true
			}
			for id, bv := range bm {
				if seen[id] {
					continue
				}
				deltas = append(deltas, delta{CaseID: id, AScore: 0, BScore: bv, Delta: bv})
			}
			NewOutput(cfg.Output).Print(deltas)
			return nil
		},
	}
	return cmd
}
