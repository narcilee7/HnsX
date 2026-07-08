package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/hnsx-io/hnsx/go/internal/version"
	"github.com/hnsx-io/hnsx/go/pkg/adapter"
	"github.com/hnsx-io/hnsx/go/pkg/core"
	"github.com/hnsx-io/hnsx/go/pkg/runtime"
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

	spec, err := core.LoadDomain(*domainPath)
	if err != nil {
		if *jsonOutput {
			printJSON(map[string]interface{}{
				"valid": false,
				"error": err.Error(),
			})
		} else {
			fmt.Fprintf(os.Stderr, "✗ invalid domain: %v\n", err)
		}
		return 1
	}

	if *jsonOutput {
		printJSON(map[string]interface{}{
			"valid":       true,
			"id":          spec.ID,
			"version":     spec.Version,
			"mode":        spec.Harness.Session.Mode,
			"agent_count": len(spec.Harness.Agents),
			"step_count":  len(spec.Harness.Session.Workflow.Steps),
		})
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

	spec, err := core.LoadDomain(*domainPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load domain: %v\n", err)
		return 1
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(*trigger), &payload); err != nil {
		fmt.Fprintf(os.Stderr, "parse trigger: %v\n", err)
		return 1
	}

	var a runtime.Adapter
	switch *adapterKind {
	case "noop":
		a = adapter.NewNoopAdapter()
	case "echo":
		a = adapter.NewEchoAdapter()
	default:
		fmt.Fprintf(os.Stderr, "unknown adapter: %s\n", *adapterKind)
		return 1
	}

	runner := runtime.NewRunner(a)
	result, err := runner.Run(nil, spec, payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		return 1
	}

	if *jsonOutput {
		b, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(b))
	} else {
		fmt.Printf("[hnsx] domain '%s' completed in %s mode (adapter=%s)\n", spec.ID, spec.Harness.Session.Mode, *adapterKind)
		fmt.Printf("[hnsx] state: %s\n", result.State)
		b, _ := json.MarshalIndent(result.Output, "", "  ")
		fmt.Printf("[hnsx] output:\n%s\n", string(b))
	}
	return 0
}

func printJSON(v interface{}) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}
