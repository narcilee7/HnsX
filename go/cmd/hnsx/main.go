package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hnsx-io/hnsx/go/pkg/core/loader"
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
	case "version":
		fmt.Println("hnsx 0.2.0-go")
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`hnsx — Harness for Autonomous Agents

Usage:
  hnsx validate --domain <path>
  hnsx run     --domain <path> --adapter <kind> --trigger <json>
  hnsx version
`)
}

func cmdValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	domainPath := fs.String("domain", "", "path to domain YAML")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		return 1
	}
	if *domainPath == "" {
		fmt.Fprintln(os.Stderr, "--domain is required")
		return 1
	}

	spec, err := loader.LoadDomain(*domainPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ invalid domain: %v\n", err)
		return 1
	}

	fmt.Printf("✓ domain '%s' v%s is valid\n", spec.ID, spec.Version)
	fmt.Printf("  mode:    %s\n", spec.Harness.Session.Mode)
	fmt.Printf("  agents:  %d\n", len(spec.Harness.Agents))
	if spec.Harness.Session.Workflow != nil {
		fmt.Printf("  steps:   %d\n", len(spec.Harness.Session.Workflow.Steps))
	}
	return 0
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	domainPath := fs.String("domain", "", "path to domain YAML")
	adapter := fs.String("adapter", "hnsx", "adapter kind: hnsx|genai|noop")
	trigger := fs.String("trigger", "{}", "JSON trigger payload")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		return 1
	}
	if *domainPath == "" {
		fmt.Fprintln(os.Stderr, "--domain is required")
		return 1
	}

	spec, err := loader.LoadDomain(*domainPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load domain: %v\n", err)
		return 1
	}

	_ = *adapter
	_ = *trigger

	fmt.Printf("[hnsx] running domain '%s' in %s mode (adapter=%s)\n", spec.ID, spec.Harness.Session.Mode, *adapter)
	fmt.Println("[hnsx] noop output: done")
	return 0
}
