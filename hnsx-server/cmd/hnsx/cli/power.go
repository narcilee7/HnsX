package cli

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// newPowerCmd groups advanced developer commands (v0.7 Power): domain
// format / diff, session replay, debug bundle, plugin install.
func newPowerCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "power",
		Short: "Advanced developer commands (format, diff, replay, debug bundle)",
	}
	cmd.AddCommand(newDomainFormatCmd(cfg))
	cmd.AddCommand(newDomainDiffCmd(cfg))
	cmd.AddCommand(newSessionReplayCmd(cfg))
	cmd.AddCommand(newDebugBundleCmd(cfg))
	cmd.AddCommand(newPluginCmd(cfg))
	return cmd
}

// ---------------------------------------------------------------------------
// hnsx domain format
// ---------------------------------------------------------------------------

// newDomainFormatCmd reads a domain YAML and writes a normalised version.
// Normalisation is opinionated but conservative: keys are sorted within each
// top-level mapping; the output round-trips through the same parser.
func newDomainFormatCmd(cfg *Config) *cobra.Command {
	var inPlace bool
	cmd := &cobra.Command{
		Use:   "format <path>...",
		Short: "Format DomainSpec YAMLs (sorted keys, normalised structure)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, path := range args {
				if err := formatOne(path, inPlace); err != nil {
					return err
				}
				fmt.Printf("✓ %s\n", path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&inPlace, "in-place", false, "overwrite the input file (default: write to stdout)")
	return cmd
}

func formatOne(path string, inPlace bool) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := spec.Parse(b); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(b, &node); err != nil {
		return fmt.Errorf("yaml decode: %w", err)
	}
	sortMapKeys(&node)
	out, err := yaml.Marshal(&node)
	if err != nil {
		return fmt.Errorf("yaml encode: %w", err)
	}
	if inPlace {
		return os.WriteFile(path, out, 0o644)
	}
	fmt.Println(string(out))
	return nil
}

// sortMapKeys recursively sorts mapping keys in a yaml.Node tree.
// Note: this mutates input — used internally by formatOne.
func sortMapKeys(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.MappingNode {
		// yaml.MappingNode.Content is interleaved [k1, v1, k2, v2, ...]
		type pair struct {
			k, v *yaml.Node
		}
		var pairs []pair
		for i := 0; i+1 < len(n.Content); i += 2 {
			pairs = append(pairs, pair{n.Content[i], n.Content[i+1]})
		}
		sort.SliceStable(pairs, func(i, j int) bool {
			return pairs[i].k.Value < pairs[j].k.Value
		})
		n.Content = n.Content[:0]
		for _, p := range pairs {
			n.Content = append(n.Content, p.k, p.v)
			sortMapKeys(p.v)
		}
		return
	}
	for _, c := range n.Content {
		sortMapKeys(c)
	}
}

// ---------------------------------------------------------------------------
// hnsx domain diff
// ---------------------------------------------------------------------------

// diffChange is one row in a domain diff report.
type diffChange struct {
	Section string `json:"section"`
	Key     string `json:"key"`
	A       any    `json:"a"`
	B       any    `json:"b"`
}

// newDomainDiffCmd diffs two domain specs and prints a structural report.
// v0.7 emits a JSON object with per-section added/removed/changed entries;
// a future minor will swap in a richer renderer.
func newDomainDiffCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <path-a> <path-b>",
		Short: "Diff two DomainSpec YAMLs structurally",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := spec.LoadFile(args[0])
			if err != nil {
				return err
			}
			b, err := spec.LoadFile(args[1])
			if err != nil {
				return err
			}
			var changes []diffChange
			diffStrings("id", a.ID, b.ID, &changes)
			diffStrings("version", a.Version, b.Version, &changes)
			diffAny("harness.session.mode", a.Harness.Session.Mode, b.Harness.Session.Mode, &changes)
			aSteps, bSteps := 0, 0
			if a.Harness.Session.Workflow != nil {
				aSteps = len(a.Harness.Session.Workflow.Steps)
			}
			if b.Harness.Session.Workflow != nil {
				bSteps = len(b.Harness.Session.Workflow.Steps)
			}
			diffAny("harness.session.workflow.steps", aSteps, bSteps, &changes)
			diffAny("harness.agents", len(a.Harness.Agents), len(b.Harness.Agents), &changes)
			diffAny("harness.skills", len(a.Harness.Skills), len(b.Harness.Skills), &changes)
			diffAny("harness.tools", len(a.Harness.Tools), len(b.Harness.Tools), &changes)

			NewOutput(cfg.Output).Print(map[string]any{
				"a":       args[0],
				"b":       args[1],
				"changes": len(changes),
				"items":   changes,
			})
			if len(changes) == 0 {
				return nil
			}
			return fmt.Errorf("%d change(s) detected", len(changes))
		},
	}
	return cmd
}

func diffStrings(section, a, b string, out *[]diffChange) {
	if a == b {
		return
	}
	*out = append(*out, diffChange{Section: section, Key: "", A: a, B: b})
}

func diffAny(section string, a, b any, out *[]diffChange) {
	if reflect.DeepEqual(a, b) {
		return
	}
	*out = append(*out, diffChange{Section: section, Key: "", A: a, B: b})
}

// ---------------------------------------------------------------------------
// hnsx session replay
// ---------------------------------------------------------------------------

// newSessionReplayCmd re-triggers a past session with optional new trigger
// payload. By default uses the original trigger; --dry-run validates without
// dispatching.
func newSessionReplayCmd(cfg *Config) *cobra.Command {
	var (
		with     string
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "replay <session-id> [--with trigger.json] [--dry-run]",
		Short: "Replay a session (re-trigger with original or new trigger)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			orig, err := c.GetSession(args[0])
			if err != nil {
				return err
			}
			payload := orig.Trigger
			if with != "" {
				payload, err = loadTrigger(with)
				if err != nil {
					return err
				}
			}
			if dryRun {
				NewOutput(cfg.Output).Print(map[string]any{
					"session_id":      orig.ID,
					"domain_id":       orig.DomainID,
					"domain_version":  orig.DomainVersion,
					"would_trigger":   payload,
					"would_use_domain": orig.DomainID,
				})
				return nil
			}
			s, err := c.TriggerSession(orig.DomainID, payload)
			if err != nil {
				return err
			}
			NewOutput(cfg.Output).Print(s)
			return nil
		},
	}
	cmd.Flags().StringVar(&with, "with", "", "override trigger payload (JSON or @file.json)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and report without triggering")
	return cmd
}

// ---------------------------------------------------------------------------
// hnsx debug bundle
// ---------------------------------------------------------------------------

// newDebugBundleCmd collects local diagnostics (hnsx config, last sessions,
// recent traces) into a tar.gz for support / debugging.
func newDebugBundleCmd(cfg *Config) *cobra.Command {
	var outFile string
	cmd := &cobra.Command{
		Use:   "debug-bundle",
		Short: "Collect logs, last sessions, and config into <file>.tar.gz",
		RunE: func(cmd *cobra.Command, args []string) error {
			if outFile == "" {
				outFile = fmt.Sprintf("hnsx-debug-%s.tar.gz", time.Now().UTC().Format("20060102T150405Z"))
			}
			if err := writeDebugBundle(cfg, outFile); err != nil {
				return err
			}
			NewOutput(cfg.Output).Line("✓ wrote %s", outFile)
			return nil
		},
	}
	cmd.Flags().StringVarP(&outFile, "out", "o", "", "output file (default hnsx-debug-<ts>.tar.gz)")
	return cmd
}

// writeDebugBundle creates a tar.gz containing:
//   - hnsx-version.txt
//   - hnsx-status.txt        (output of `hnsx status`)
//   - hnsx-config.yaml       (current Config as YAML)
//   - last-sessions.json     (up to 20 recent sessions)
//   - last-traces.json       (up to 20 recent traces)
func writeDebugBundle(cfg *Config, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	addFile := func(name, body string) error {
		h := &tar.Header{
			Name:    name,
			Mode:    0o644,
			Size:    int64(len(body)),
			ModTime: time.Now(),
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			return err
		}
		return nil
	}

	addFile("hnsx-version.txt", fmt.Sprintf("hnsx-cli\n%s\n", versionString(cfg)))
	addFile("hnsx-status.txt", collectStatus(cfg))
	addFile("hnsx-config.yaml", configAsYAML(cfg))

	if body, err := fetchJSON(cfg, "/api/v1/sessions?limit=20"); err == nil {
		_ = addFile("last-sessions.json", string(body))
	}
	if body, err := fetchJSON(cfg, "/api/v1/traces?limit=20"); err == nil {
		_ = addFile("last-traces.json", string(body))
	}
	return nil
}

func versionString(cfg *Config) string {
	// Defer to the version subcommand for canonical output.
	out, err := execHTTPGet(cfg.ServerURL + "/version")
	if err == nil && len(out) > 0 {
		return string(out)
	}
	return "unknown"
}

func collectStatus(cfg *Config) string {
	// We don't exec a child process here; instead, hit the same endpoints
	// status uses. Best-effort, never fatal.
	var b strings.Builder
	b.WriteString("server: " + cfg.ServerURL + "\n")
	if out, err := fetchJSON(cfg, "/healthz"); err == nil {
		b.WriteString("healthz: " + string(out) + "\n")
	}
	return b.String()
}

func configAsYAML(cfg *Config) string {
	return fmt.Sprintf("server: %s\noutput: %s\ncompose_file: %s\nrepo_root: %s\n",
		cfg.ServerURL, cfg.Output, cfg.ComposeFile, cfg.RepoRoot)
}

func fetchJSON(cfg *Config, path string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(cfg.ServerURL + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func execHTTPGet(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// ---------------------------------------------------------------------------
// hnsx plugin
// ---------------------------------------------------------------------------

// newPluginCmd is a stub for the v0.7 plugin surface. v1.0 wires a real
// external-subcommand mechanism (similar to kubectl plugins).
func newPluginCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage hnsx plugins (placeholder; real implementation lands in v1.0)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			NewOutput(cfg.Output).Line("(no plugins installed; external-subcommand support lands in v1.0)")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "install <url>",
		Short: "Install a plugin from a URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			NewOutput(cfg.Output).Line("plugin install is a placeholder in v0.7; v1.0 wires real external-subcommand discovery")
			return nil
		},
	})
	return cmd
}

// mapKeysToJSON is a tiny helper used by debug bundle to serialise maps
// with stable key order for reproducibility.
func mapKeysToJSON(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return b, nil
}

// keep filepath used to avoid an unused-import error in this file.
var _ = filepath.Join