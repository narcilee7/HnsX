package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/pkg/local"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// newRunCmd preserves the original "run" subcommand under cobra.
func newRunCmd(cfg *Config) *cobra.Command {
	var domainPath string
	var adapterKind string
	var trigger string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "run --domain <path>",
		Short: "Run a single session locally (no control plane)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if domainPath == "" {
				return fmt.Errorf("--domain is required")
			}
			s, err := spec.LoadFile(domainPath)
			if err != nil {
				return fmt.Errorf("load domain: %w", err)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(trigger), &payload); err != nil {
				return fmt.Errorf("parse trigger: %w", err)
			}
			a, err := local.PickAdapter(adapterKind)
			if err != nil {
				return err
			}
			result, err := local.RunLocalSession(nil, s, payload, a)
			if err != nil {
				if jsonOutput || cfg.Output == "json" {
					b, _ := json.MarshalIndent(result, "", "  ")
					fmt.Println(string(b))
				}
				return fmt.Errorf("run failed: %w", err)
			}
			if jsonOutput || cfg.Output == "json" {
				b, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(b))
			} else {
				fmt.Printf("[hnsx] domain '%s' completed in %s mode (adapter=%s)\n",
					s.ID, s.Harness.Session.Mode, adapterKind)
				fmt.Printf("[hnsx] state: %s\n", result.State)
				b, _ := json.MarshalIndent(result.Output, "", "  ")
				fmt.Printf("[hnsx] output:\n%s\n", string(b))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&domainPath, "domain", "", "path to domain YAML")
	cmd.Flags().StringVar(&adapterKind, "adapter", "noop", "adapter kind: noop|echo")
	cmd.Flags().StringVar(&trigger, "trigger", "{}", "JSON trigger payload")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}