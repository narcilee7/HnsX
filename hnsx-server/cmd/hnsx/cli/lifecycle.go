package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Lifecycle helpers (process orchestration)
// ---------------------------------------------------------------------------

// runCompose executes `docker compose -f <file> ...args`. stdout/stderr are
// streamed to the user's terminal unless the CLI is in --output json|quiet.
func runCompose(cfg *Config, args ...string) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found on PATH; install Docker Desktop or set --compose-file to a remote driver")
	}
	cmd := exec.Command("docker", append([]string{"compose", "-f", cfg.ComposeFile}, args...)...)
	if cfg.Output == "quiet" {
		cmd.Stdout = nil
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	cmd.Env = append(os.Environ(), "COMPOSE_PROJECT_NAME=hnsx")
	return cmd.Run()
}

// containerStatus inspects a docker-compose service by name and reports its
// state ("running"/"exited"/"not found"). Best-effort; missing docker is
// reported as "unreachable".
func containerStatus(cfg *Config, service string) string {
	if _, err := exec.LookPath("docker"); err != nil {
		return "unreachable"
	}
	cmd := exec.Command("docker", "compose", "-f", cfg.ComposeFile,
		"ps", "--format", "json", service)
	out, err := cmd.Output()
	if err != nil {
		return "not found"
	}
	out = bytes.TrimSpace(out)
	if len(out) == 0 || string(out) == "[]" || string(out) == "null" {
		return "not found"
	}
	// Compose --format json emits one JSON object per line.
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			continue
		}
		if name, _ := row["Name"].(string); name != "" && strings.Contains(name, service) {
			if state, _ := row["State"].(string); state != "" {
				return state
			}
		}
	}
	return "unknown"
}

// httpHealth pings the server's /healthz and returns nil if 2xx.
func httpHealth(ctx context.Context, base string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("healthz returned %d", resp.StatusCode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// hnsx up
// ---------------------------------------------------------------------------

func newUpCmd(cfg *Config) *cobra.Command {
	var detach bool
	var withTelemetry bool
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start the local HnsX stack (postgres + server + worker)",
		Long: `Bring up the local HnsX stack defined in deployments/local/docker-compose.yaml.

By default the command blocks until the server passes /healthz. Use --detach
to return immediately. Use --with-telemetry to also start Tempo + Grafana.

Examples:
  hnsx up
  hnsx up --detach
  hnsx up --with-telemetry
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := NewOutput(cfg.Output)
			if !fileExists(cfg.ComposeFile) {
				return fmt.Errorf("compose file not found: %s (run from the HnsX repo root or set --compose-file)", cfg.ComposeFile)
			}

			services := []string{"postgres", "server", "worker"}
			if withTelemetry {
				services = append(services, "tempo", "grafana")
			}

			out.Line("→ starting %s ...", strings.Join(services, ", "))
			upArgs := []string{"up"}
			if detach {
				upArgs = append(upArgs, "-d")
			}
			upArgs = append(upArgs, services...)
			if err := runCompose(cfg, upArgs...); err != nil {
				return fmt.Errorf("docker compose up failed: %w", err)
			}

			if detach {
				out.Line("✓ detach mode; check `hnsx status` for readiness")
				return nil
			}

			// Block on /healthz with a 60s budget.
			out.Line("→ waiting for server healthz at %s ...", cfg.ServerURL)
			ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
			defer cancel()
			for {
				if err := httpHealth(ctx, cfg.ServerURL); err == nil {
					out.Line("✓ server is healthy")
					out.Line("→ next: `hnsx examples` and `hnsx try <name>`")
					return nil
				}
				select {
				case <-ctx.Done():
					return fmt.Errorf("server did not become healthy in 60s: %w", ctx.Err())
				case <-time.After(2 * time.Second):
				}
			}
		},
	}
	cmd.Flags().BoolVar(&detach, "detach", false, "return immediately after starting containers")
	cmd.Flags().BoolVar(&withTelemetry, "with-telemetry", false, "also start Tempo + Grafana")
	return cmd
}

// ---------------------------------------------------------------------------
// hnsx down
// ---------------------------------------------------------------------------

func newDownCmd(cfg *Config) *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop the local HnsX stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := NewOutput(cfg.Output)
			if !fileExists(cfg.ComposeFile) {
				return fmt.Errorf("compose file not found: %s", cfg.ComposeFile)
			}
			out.Line("→ stopping stack ...")
			downArgs := []string{"down"}
			if purge {
				downArgs = append(downArgs, "--volumes")
			}
			if err := runCompose(cfg, downArgs...); err != nil {
				return fmt.Errorf("docker compose down failed: %w", err)
			}
			out.Line("✓ stack stopped")
			return nil
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also remove named volumes (postgres data)")
	return cmd
}

// ---------------------------------------------------------------------------
// hnsx restart
// ---------------------------------------------------------------------------

func newRestartCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the local HnsX stack (down + up)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runCompose(cfg, "down"); err != nil {
				// Non-fatal: stack may not be running.
				_ = err
			}
			return runCompose(cfg, "up", "-d", "postgres", "server", "worker")
		},
	}
	return cmd
}

// ---------------------------------------------------------------------------
// hnsx status
// ---------------------------------------------------------------------------

func newStatusCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the local HnsX stack status",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := NewOutput(cfg.Output)
			services := []string{"postgres", "server", "worker", "tempo", "grafana"}
			rows := make([][]string, 0, len(services))
			allOK := true
			for _, s := range services {
				st := containerStatus(cfg, s)
				if st != "running" {
					allOK = false
				}
				rows = append(rows, []string{s, st})
			}
			out.Table([]string{"SERVICE", "STATE"}, rows)

			// health probe
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
			defer cancel()
			hErr := httpHealth(ctx, cfg.ServerURL)
			out.Line("server healthz: %s", healthLabel(hErr))
			if !allOK || hErr != nil {
				out.Line("\n💡 run `hnsx up` to start missing services, `hnsx doctor` for diagnosis")
			}
			return nil
		},
	}
	return cmd
}

func healthLabel(err error) string {
	if err == nil {
		return "✓ ok"
	}
	return "✗ " + err.Error()
}

// ---------------------------------------------------------------------------
// hnsx doctor
// ---------------------------------------------------------------------------

func newDoctorCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose common problems with the local HnsX stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := NewOutput(cfg.Output)

			type check struct {
				name   string
				status string
				detail string
			}
			var checks []check

			// docker on PATH
			if p, err := exec.LookPath("docker"); err != nil {
				checks = append(checks, check{"docker", "fail", "not found on PATH"})
			} else {
				checks = append(checks, check{"docker", "ok", p})
			}

			// compose file present
			if fileExists(cfg.ComposeFile) {
				checks = append(checks, check{"compose-file", "ok", cfg.ComposeFile})
			} else {
				checks = append(checks, check{"compose-file", "fail", cfg.ComposeFile})
			}

			// repo root
			if cfg.RepoRoot != "" && hasMarker(cfg.RepoRoot) {
				checks = append(checks, check{"repo-root", "ok", cfg.RepoRoot})
			} else {
				checks = append(checks, check{"repo-root", "warn", "could not locate deployments/ and example-domains/ markers"})
			}

			// server health
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
			defer cancel()
			if err := httpHealth(ctx, cfg.ServerURL); err != nil {
				checks = append(checks, check{"server-health", "fail", err.Error()})
			} else {
				checks = append(checks, check{"server-health", "ok", cfg.ServerURL})
			}

			if cfg.Output == "json" {
				type row struct {
					Name   string `json:"name"`
					Status string `json:"status"`
					Detail string `json:"detail,omitempty"`
				}
				rs := make([]row, 0, len(checks))
				for _, c := range checks {
					rs = append(rs, row{c.name, c.status, c.detail})
				}
				out.Print(rs)
			} else if cfg.Output != "quiet" {
				rows := make([][]string, 0, len(checks))
				for _, c := range checks {
					rows = append(rows, []string{c.name, c.status, c.detail})
				}
				out.Table([]string{"CHECK", "STATUS", "DETAIL"}, rows)
			}

			failed := false
			for _, c := range checks {
				if c.status == "fail" {
					failed = true
				}
			}
			if failed {
				out.Line("\n💡 some checks failed. Common fixes:")
				out.Line("  • install Docker Desktop: https://docs.docker.com/desktop/")
				out.Line("  • cd into the HnsX repo root, or pass --compose-file")
				out.Line("  • run `hnsx up` to start the stack")
				return fmt.Errorf("doctor found issues")
			}
			return nil
		},
	}
	return cmd
}

// ---------------------------------------------------------------------------
// hnsx logs
// ---------------------------------------------------------------------------

func newLogsCmd(cfg *Config) *cobra.Command {
	var follow bool
	var service string
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Tail logs from the local HnsX stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := NewOutput(cfg.Output)
			if !fileExists(cfg.ComposeFile) {
				return fmt.Errorf("compose file not found: %s", cfg.ComposeFile)
			}
			logArgs := []string{"logs"}
			if follow {
				logArgs = append(logArgs, "-f")
			}
			if service != "" {
				logArgs = append(logArgs, service)
			}
			out.Line("→ docker compose %s", strings.Join(logArgs, " "))
			return runCompose(cfg, logArgs...)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	cmd.Flags().StringVarP(&service, "service", "s", "", "service name (postgres|server|worker|tempo|grafana)")
	return cmd
}

// ---------------------------------------------------------------------------
// hnsx reset
// ---------------------------------------------------------------------------

func newResetCmd(cfg *Config) *cobra.Command {
	var hard bool
	var yes bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset the local HnsX stack to a clean state",
		Long: "Stop the stack and remove its data.\n\n" +
			"  --hard    also remove the example-domains/ registration cache\n" +
			"            (next hnsx up will re-seed from example-domains/).\n" +
			"  --yes     skip the confirmation prompt (CI-friendly).",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := NewOutput(cfg.Output)
			if !yes {
				out.Line("⚠ this will delete all local data. Re-run with --yes to confirm.")
				return nil
			}
			if err := runCompose(cfg, "down", "--volumes"); err != nil {
				out.Line("docker compose down --volumes returned: %v", err)
			}
			if hard {
				cacheDir := filepath.Join(cfg.RepoRoot, ".hnsx-cache")
				if err := os.RemoveAll(cacheDir); err != nil {
					return fmt.Errorf("clean cache: %w", err)
				}
				out.Line("✓ cleared %s", cacheDir)
			}
			out.Line("✓ reset complete. Run `hnsx up` to start fresh.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&hard, "hard", false, "also wipe the local example-domains cache")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

// ---------------------------------------------------------------------------
// small helper
// ---------------------------------------------------------------------------

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}