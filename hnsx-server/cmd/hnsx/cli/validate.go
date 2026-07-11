package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// newValidateCmd preserves the original "validate" subcommand under cobra.
func newValidateCmd(cfg *Config) *cobra.Command {
	var domainPath string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "validate --domain <path>",
		Short: "Parse and validate a DomainSpec YAML",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := NewOutput(cfg.Output)
			if domainPath == "" {
				return fmt.Errorf("--domain is required")
			}
			s, err := spec.LoadFile(domainPath)
			if err != nil {
				if jsonOutput || cfg.Output == "json" {
					out.Print(map[string]any{"valid": false, "error": err.Error()})
				} else {
					out.Line("✗ invalid domain: %v", err)
				}
				return err
			}
			count := len(s.Harness.Agents)
			steps := 0
			if s.Harness.Session.Workflow != nil {
				steps = len(s.Harness.Session.Workflow.Steps)
			}
			result := map[string]any{
				"valid":       true,
				"id":          s.ID,
				"version":     s.Version,
				"mode":        s.Harness.Session.Mode,
				"agent_count": count,
				"step_count":  steps,
			}
			if jsonOutput || cfg.Output == "json" {
				b, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(b))
			} else {
				out.Line("✓ domain '%s' v%s is valid", s.ID, s.Version)
				out.Line("  mode:    %s", s.Harness.Session.Mode)
				out.Line("  agents:  %d", count)
				if steps > 0 {
					out.Line("  steps:   %d", steps)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&domainPath, "domain", "", "path to domain YAML")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}
