// hnsx is the operator-facing CLI for HnsX. It exposes local commands that
// do not require a running control plane (validate / run / version) and remote
// subcommands that talk to the server API.
//
// Subcommands in this build:
//
//   - validate  : parse + structural-validate a DomainSpec YAML.
//   - run       : execute a single session locally (no control plane).
//   - remote    : talk to a running hnsx-server.
//   - version   : print version info.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/hnsx-io/hnsx/server/internal/client"
	"github.com/hnsx-io/hnsx/server/pkg/local"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
	"github.com/hnsx-io/hnsx/server/pkg/version"
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
	case "remote":
		os.Exit(cmdRemote(os.Args[2:]))
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
  hnsx remote  domains list
  hnsx remote  domains get <id>
  hnsx remote  domains register --file <path>
  hnsx remote  sessions list
  hnsx remote  sessions get <id>
  hnsx remote  sessions trigger --domain <id> [--trigger <json>]
  hnsx version

Examples:
  hnsx validate --domain domains/customer-service/domain.yaml
  hnsx run --domain domains/customer-service/domain.yaml --adapter noop --trigger '{"question":"hello"}'
  HNSX_SERVER_URL=http://127.0.0.1:58081 hnsx remote sessions list
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

	s, err := spec.LoadFile(*domainPath)
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

	count := len(s.Harness.Agents)
	steps := 0
	if s.Harness.Session.Workflow != nil {
		steps = len(s.Harness.Session.Workflow.Steps)
	}

	if *jsonOutput {
		printJSON(map[string]any{
			"valid":       true,
			"id":          s.ID,
			"version":     s.Version,
			"mode":        s.Harness.Session.Mode,
			"agent_count": count,
			"step_count":  steps,
		})
	} else {
		fmt.Printf("✓ domain '%s' v%s is valid\n", s.ID, s.Version)
		fmt.Printf("  mode:    %s\n", s.Harness.Session.Mode)
		fmt.Printf("  agents:  %d\n", count)
		if steps > 0 {
			fmt.Printf("  steps:   %d\n", steps)
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

	s, err := spec.LoadFile(*domainPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load domain: %v\n", err)
		return 1
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(*trigger), &payload); err != nil {
		fmt.Fprintf(os.Stderr, "parse trigger: %v\n", err)
		return 1
	}

	a, err := local.PickAdapter(*adapterKind)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	result, err := local.RunLocalSession(nil, s, payload, a)
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
			s.ID, s.Harness.Session.Mode, *adapterKind)
		fmt.Printf("[hnsx] state: %s\n", result.State)
		b, _ := json.MarshalIndent(result.Output, "", "  ")
		fmt.Printf("[hnsx] output:\n%s\n", string(b))
	}
	return 0
}

func cmdRemote(args []string) int {
	if len(args) < 1 {
		printRemoteUsage()
		return 1
	}
	switch args[0] {
	case "domains":
		return cmdRemoteDomains(args[1:])
	case "sessions":
		return cmdRemoteSessions(args[1:])
	default:
		printRemoteUsage()
		return 1
	}
}

func printRemoteUsage() {
	fmt.Print(`hnsx remote — talk to a running hnsx-server

Usage:
  hnsx remote domains list
  hnsx remote domains get <id>
  hnsx remote domains register --file <path>
  hnsx remote sessions list
  hnsx remote sessions get <id>
  hnsx remote sessions trigger --domain <id> [--trigger <json>]

Environment:
  HNSX_SERVER_URL   Server base URL (default http://127.0.0.1:50051)
`)
}

func cmdRemoteDomains(args []string) int {
	if len(args) < 1 {
		printRemoteUsage()
		return 1
	}
	c := client.New()
	switch args[0] {
	case "list":
		items, err := c.ListDomains()
		if err != nil {
			fmt.Fprintf(os.Stderr, "list domains: %v\n", err)
			return 1
		}
		printJSON(items)
	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "domain id required")
			return 1
		}
		d, err := c.GetDomain(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "get domain: %v\n", err)
			return 1
		}
		printJSON(d)
	case "register":
		fs := flag.NewFlagSet("register", flag.ExitOnError)
		path := fs.String("file", "", "path to domain YAML")
		_ = fs.Parse(args[1:])
		if *path == "" {
			fmt.Fprintln(os.Stderr, "--file is required")
			return 1
		}
		f, err := os.Open(*path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open file: %v\n", err)
			return 1
		}
		defer f.Close()
		d, err := c.RegisterDomain(f, "application/x-yaml")
		if err != nil {
			fmt.Fprintf(os.Stderr, "register domain: %v\n", err)
			return 1
		}
		printJSON(d)
	default:
		printRemoteUsage()
		return 1
	}
	return 0
}

func cmdRemoteSessions(args []string) int {
	if len(args) < 1 {
		printRemoteUsage()
		return 1
	}
	c := client.New()
	switch args[0] {
	case "list":
		items, err := c.ListSessions()
		if err != nil {
			fmt.Fprintf(os.Stderr, "list sessions: %v\n", err)
			return 1
		}
		printJSON(items)
	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "session id required")
			return 1
		}
		s, err := c.GetSession(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "get session: %v\n", err)
			return 1
		}
		printJSON(s)
	case "trigger":
		fs := flag.NewFlagSet("trigger", flag.ExitOnError)
		domainID := fs.String("domain", "", "domain id")
		trigger := fs.String("trigger", "{}", "JSON trigger payload")
		_ = fs.Parse(args[1:])
		if *domainID == "" {
			fmt.Fprintln(os.Stderr, "--domain is required")
			return 1
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(*trigger), &payload); err != nil {
			fmt.Fprintf(os.Stderr, "parse trigger: %v\n", err)
			return 1
		}
		s, err := c.TriggerSession(*domainID, payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "trigger session: %v\n", err)
			return 1
		}
		printJSON(s)
	default:
		printRemoteUsage()
		return 1
	}
	return 0
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

// Ensure spec package is referenced; silences unused-import lints when
// future subcommands land.
var _ = spec.DomainSpec{}
