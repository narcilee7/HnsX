// hnsx is the operator-facing CLI for HnsX. It is designed to be friendly to
// AI agents as well as humans: every command supports `--json`, errors are
// machine-readable, and `validate` / `run` are safe to call from automation.
//
// Subcommands in this build:
//
//   - validate  : parse + structural-validate a DomainSpec YAML.
//   - run       : execute a single session locally (no control plane).
//   - version   : print version info.
//
// Subcommands planned for later phases:
//
//   - eval       : run an EvalSet against a DomainSpec.
//   - traces     : query local Session telemetry.
//   - domains    : register / list DomainSpecs in a Domain Registry.
//   - login      : authenticate against a remote Control Plane.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/hnsx-io/hnsx/core/adapter"
	"github.com/hnsx-io/hnsx/core/domain"
	"github.com/hnsx-io/hnsx/core/loader"
	"github.com/hnsx-io/hnsx/core/runtime"
	"github.com/hnsx-io/hnsx/core/version"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "validate":
		os.Exit(cmdValidate(os.Args[2:]))
	case "run":
		os.Exit(cmdRun(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println(version.String())
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`hnsx — Harness for Autonomous Agents

Usage:
  hnsx validate --domain <path> [--json]
  hnsx run     --domain <path> --adapter <kind> --trigger <json> [--json]
  hnsx version

Examples:
  hnsx validate --domain domains/customer-service/domain.yaml
  hnsx validate --domain domains/customer-service/domain.yaml --json
  hnsx run --domain domains/customer-service/domain.yaml --adapter noop --trigger '{"question":"hello"}'
`)
}

func cmdValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	domainPath := fs.String("domain", "", "path to domain YAML")
	jsonOutput := fs.Bool("json", false, "output as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		return 1
	}
	if *domainPath == "" {
		fmt.Fprintln(os.Stderr, "--domain is required")
		return 1
	}

	spec, err := loader.LoadFile(*domainPath)
	if err != nil {
		if *jsonOutput {
			printJSON(map[string]any{
				"valid": false,
				"error": err.Error(),
			})
		} else {
			fmt.Fprintf(os.Stderr, "✗ invalid domain: %v\n", err)
		}
		return 1
	}

	summary := map[string]any{
		"valid":       true,
		"id":          spec.ID,
		"version":     spec.Version,
		"mode":        spec.Harness.Session.Mode,
		"agent_count": len(spec.Harness.Agents),
	}
	if spec.Harness.Session.Workflow != nil {
		summary["step_count"] = len(spec.Harness.Session.Workflow.Steps)
	}
	if *jsonOutput {
		printJSON(summary)
	} else {
		fmt.Printf("✓ domain '%s' v%s is valid\n", spec.ID, spec.Version)
		fmt.Printf("  mode:    %s\n", spec.Harness.Session.Mode)
		fmt.Printf("  agents:  %d\n", len(spec.Harness.Agents))
		if spec.Harness.Session.Workflow != nil {
			fmt.Printf("  steps:   %d\n", len(spec.Harness.Session.Workflow.Steps))
		}
	}
	return 0
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	domainPath := fs.String("domain", "", "path to domain YAML")
	adapterKind := fs.String("adapter", "noop", "adapter kind: noop|echo")
	trigger := fs.String("trigger", "{}", "JSON trigger payload")
	jsonOutput := fs.Bool("json", false, "output as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		return 1
	}
	if *domainPath == "" {
		fmt.Fprintln(os.Stderr, "--domain is required")
		return 1
	}

	spec, err := loader.LoadFile(*domainPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load domain: %v\n", err)
		return 1
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(*trigger), &payload); err != nil {
		fmt.Fprintf(os.Stderr, "parse trigger: %v\n", err)
		return 1
	}

	a, err := pickAdapter(*adapterKind)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	runner := runtime.NewRunner(a)
	result, err := runner.Run(nil, spec, payload)
	if err != nil {
		if *jsonOutput {
			b, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		}
		return 1
	}

	if *jsonOutput {
		b, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(b))
	} else {
		fmt.Printf("[hnsx] domain '%s' completed in %s mode (adapter=%s)\n",
			spec.ID, spec.Harness.Session.Mode, *adapterKind)
		fmt.Printf("[hnsx] state: %s\n", result.State)
		b, _ := json.MarshalIndent(result.Output, "", "  ")
		fmt.Printf("[hnsx] output:\n%s\n", string(b))
	}
	return 0
}

func pickAdapter(kind string) (runtime.Adapter, error) {
	switch kind {
	case "noop", "":
		return adapter.NewNoopAdapter(), nil
	case "echo":
		return adapter.NewEchoAdapter(), nil
	default:
		return nil, fmt.Errorf("unknown adapter: %s (built-in: noop, echo)", kind)
	}
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

// Ensure domain package is referenced; silences unused-import lints when
// future subcommands land.
var _ = domain.DomainSpec{}
