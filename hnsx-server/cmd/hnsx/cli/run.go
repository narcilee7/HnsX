package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/pkg/local"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// newRunCmd runs a single session locally through the embedded Python worker.
// It no longer uses the in-process Go adapters so that local execution supports
// the same adapters, tools, MCP servers and orchestration modes as the worker.
func newRunCmd(cfg *Config) *cobra.Command {
	var domainPath string
	var adapterKind string
	var trigger string
	var jsonOutput bool
	var workerPython string
	var noPolicy bool

	cmd := &cobra.Command{
		Use:   "run --domain <path>",
		Short: "Run a single session locally via the embedded Python worker",
		Long: `Run executes a DomainSpec end-to-end in a local Python worker subprocess.

By default the agents use the adapter declared in the DomainSpec. Use --adapter
to override every agent to the same kind (e.g. noop or echo) for offline demos.`,
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

			opts := local.EmbeddedRunOptions{
				AdapterOverride: adapterKind,
				PythonPath:      workerPython,
				TimeoutSeconds:  120,
				NoPolicy:        noPolicy,
			}
			result, err := local.RunEmbeddedSession(cmd.Context(), s, payload, opts)

			if jsonOutput || cfg.Output == "json" {
				b, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(b))
			} else {
				fmt.Printf("[hnsx] domain '%s' %s in %s mode\n", s.ID, result.State, s.Harness.Session.Mode)
				if result.Stderr != "" {
					fmt.Fprintf(os.Stderr, "[hnsx] worker stderr:\n%s\n", result.Stderr)
				}
				if len(result.Result) > 0 {
					b, _ := json.MarshalIndent(result.Result, "", "  ")
					fmt.Printf("[hnsx] result:\n%s\n", string(b))
				}
			}

			if err != nil {
				return fmt.Errorf("run failed: %w", err)
			}
			if result.State != "completed" {
				return fmt.Errorf("session ended with state: %s", result.State)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&domainPath, "domain", "", "path to domain YAML")
	cmd.Flags().StringVar(&adapterKind, "adapter", "", "override every agent to this adapter kind (e.g. noop, echo, openai)")
	cmd.Flags().StringVar(&trigger, "trigger", "{}", "JSON trigger payload")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	cmd.Flags().StringVar(&workerPython, "worker-python", "", "path to the Python interpreter for the worker (default: auto-detect)")
	cmd.Flags().MarkHidden("worker-python")
	cmd.Flags().BoolVar(&noPolicy, "no-policy", false, "disable worker-side policy engine for debugging")
	return cmd
}
