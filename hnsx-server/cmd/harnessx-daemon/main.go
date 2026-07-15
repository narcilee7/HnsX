// Package main is the HarnessX daemon entry point. It is a single Go
// binary that replaces Multica's Go daemon. The daemon:
//
//   - Connects to the HarnessX server (HnsX's native control plane or the
//     forked Multica server) over Multica's WebSocket protocol.
//   - Spawns agent CLIs (Claude Code, Codex, CodeBuddy, Copilot, Cursor,
//     etc.) as subprocesses and captures their stream-json output.
//   - Emits Multica TaskMessage frames back to the server for the live
//     observation stream consumed by Next.js and other consumers.
//
// In later roadmap phases the daemon grows an in-process Harness engine
// (Policy / Approval / Skill resolver / Eval) and an MCP server that wires
// the harness into the spawned CC subprocess. P0 keeps the daemon as a
// thin CLI wrapper: spawn → capture → forward.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/config"
	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/engine"
	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/wire"
)

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("harnessx-daemon", flag.ExitOnError)
	cfgPath := fs.String("config", "", "optional path to YAML config")
	serverURL := fs.String("server", "", "HarnessX server URL (e.g. http://127.0.0.1:50051)")
	authToken := fs.String("auth-token", "", "task-scoped auth token (mat_*)")
	workspaceID := fs.String("workspace-id", "", "workspace UUID the daemon belongs to")
	verbose := fs.Bool("verbose", false, "enable verbose logging")
	maxCostUSD := fs.Float64("max-cost-usd", 0, "per-session cost ceiling; over it triggers approval_required")
	daemonCommand := fs.String("command", "", "agent CLI binary (default: claude)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "harnessx-daemon: %v\n", err)
		return 1
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "harnessx-daemon: load config: %v\n", err)
		return 1
	}
	if *serverURL != "" {
		cfg.ServerURL = *serverURL
	}
	if *authToken != "" {
		cfg.AuthToken = *authToken
	}
	if *workspaceID != "" {
		cfg.WorkspaceID = *workspaceID
	}
	cfg.Verbose = cfg.Verbose || *verbose

	if cfg.ServerURL == "" {
		fmt.Fprintln(os.Stderr, "harnessx-daemon: --server is required (or set in config)")
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	d := wire.NewDaemon(cfg)

	// Wire the executor into the claim loop. When a task is claimed, the
	// engine runs policy checks, spawns the agent subprocess, streams
	// observations back, and marks the task complete/failed.
	exec := engine.NewExecutor(d, &engine.FlatPolicy{MaxCostUSD: *maxCostUSD}, engine.ExecutorDefaults{
		Command:          *daemonCommand,
		Args:             []string{"-p", "--output-format", "stream-json", "--verbose"},
		EstimatedCostUSD: 0.10,
	})
	d.OnTask = exec.Execute

	if err := d.Run(ctx); err != nil {
		log.Printf("harnessx-daemon: %v", err)
		return 1
	}
	return 0
}
