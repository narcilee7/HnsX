package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// newDeployCmd implements `hnsx deploy [path]`.
//
// This command is the user-facing entry point for pushing a local
// DomainSpec into either a running hnsx-server (`--target local`, the
// default) or the hosted HnsX Cloud (`--target cloud`, deferred).
//
// Today only `--target local` is implemented; the cloud path captures
// the OAuth handshake via `gh` and is wired up to fail loudly with a
// "not yet available" message so users get a clear status. See
// docs/ROADMAP.md § Phase 1 / W5-W6 for the cloud timeline.
//
// Local mode is intentionally thin: it delegates the actual upload to
// `hnsx domain register`, so deploy is just "check server is up,
// validate, register, summarize". A future improvement is to skip the
// re-upload when the spec is byte-identical to the latest registered
// version — see the `version` helper below.
func newDeployCmd(cfg *Config) *cobra.Command {
	var (
		target  string
		up      bool
		force   bool
		openUI  bool
		domain  string
	)

	cmd := &cobra.Command{
		Use:   "deploy [path/to/domain.yaml]",
		Short: "Deploy a DomainSpec to a local or hosted HnsX target",
		Long: `Deploy a DomainSpec from a local YAML file to a running HnsX target.

The default target is "local" — the CLI checks that hnsx-server is
reachable at $HNSX_SERVER_URL (default http://127.0.0.1:50052) and
then registers the domain. If the server is not reachable, pass
--up to run "hnsx up" first.

Pass --target cloud to push to the hosted HnsX Cloud (Phase 2; today
this requires gh auth login and prints a not-yet-implemented error).

Examples:
  hnsx deploy                        # deploy ./domain.yaml
  hnsx deploy my-cs/domain.yaml     # deploy a specific file
  hnsx deploy --up                  # start the local stack first
  hnsx deploy --target cloud        # push to hosted Cloud (Phase 2)
  hnsx deploy --open                # open the Console in browser after deploy`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "domain.yaml"
			if len(args) > 0 {
				path = args[0]
			}
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("domain spec not found: %s (run `hnsx new <id>` to scaffold one)", path)
			}

			switch target {
			case "local":
				return runDeployLocal(cmd, cfg, path, up, force, openUI)
			case "cloud":
				return runDeployCloud(cmd, cfg, path)
			default:
				return fmt.Errorf("unknown --target %q (want local or cloud)", target)
			}
		},
	}

	cmd.Flags().StringVar(&target, "target", "local", "deploy target: local | cloud")
	cmd.Flags().BoolVar(&up, "up", false, "run `hnsx up` first if server is unreachable")
	cmd.Flags().BoolVar(&force, "force", false, "re-register even if version is unchanged")
	cmd.Flags().BoolVar(&openUI, "open", false, "open the HnsX Console in browser after deploy")
	cmd.Flags().StringVar(&domain, "domain", "", "override the domain id parsed from domain.yaml (advanced)")

	return cmd
}

// ---------------------------------------------------------------------------
// Local target
// ---------------------------------------------------------------------------

func runDeployLocal(cmd *cobra.Command, cfg *Config, path string, up, force, openUI bool) error {
	out := NewOutputWriter(cfg.Output, cmd.OutOrStdout())

	if !serverReachable(cfg) {
		if !up {
			out.Line("✗ hnsx-server not reachable at %s", cfg.ServerURL)
			out.Line("  Pass --up to start the local stack automatically, or run:")
			out.Line("    hnsx up")
			return errors.New("server not reachable")
		}
		out.Line("→ hnsx-server not reachable; running hnsx up...")
		if err := runUpInline(cfg); err != nil {
			return fmt.Errorf("hnsx up failed: %w", err)
		}
		// Re-check after up.
		if !serverReachable(cfg) {
			return fmt.Errorf("hnsx-server still unreachable at %s after hnsx up", cfg.ServerURL)
		}
	}

	// Validate locally first — the server will validate too, but failing
	// here gives a faster error and avoids burning a request round-trip.
	if err := validateFile(path); err != nil {
		return fmt.Errorf("domain spec is invalid: %w", err)
	}

	// Determine whether the domain is already registered (so we can
	// surface a clear message and decide whether --force is needed).
	id, _, err := readDomainIDAndVersion(path)
	if err != nil {
		return err
	}

	if !force && serverDomainExists(cfg, id) {
		existing, _ := serverDomainVersion(cfg, id)
		out.Line("✗ domain %q is already registered (latest v%s)", id, existing)
		out.Line("  Pass --force to re-register, or bump the version in domain.yaml.")
		return fmt.Errorf("domain %q already exists at v%s; rerun with --force", id, existing)
	}

	// Upload via the REST endpoint (POST /api/v1/domains with a YAML body).
	// We don't reuse `hnsx domain register` here because that command
	// speaks Connect RPC (application/proto), which is awkward to mock
	// from a unit test and harder to debug when it fails.
	version, err := registerDomainREST(cfg, path)
	if err != nil {
		return fmt.Errorf("register %s: %w", path, err)
	}

	out.Line("")
	out.Line("✓ Deployed %s (v%s) to %s", id, version, cfg.ServerURL)
	out.Line("  Open the Console: %s", consoleURLFor(cfg))
	if openUI {
		openBrowser(consoleURLFor(cfg))
	}
	return nil
}

// registerDomainREST POSTs the YAML body to the server's REST endpoint
// and returns the registered version. Kept as a small helper so deploy
// doesn't grow a giant RunE.
func registerDomainREST(cfg *Config, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/v1/domains"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-yaml")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("POST %s: status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode response: %w (body: %s)", err, strings.TrimSpace(string(body)))
	}
	if payload.Version == "" {
		// Some servers omit version; fall back to the YAML's declared
		// version so the success message is never empty.
		_, payload.Version, _ = readDomainIDAndVersion(path)
	}
	return payload.Version, nil
}

// runUpInline shells out to `hnsx up` and waits for the health endpoint.
// We re-implement the up logic here rather than reusing the cobra command
// because cobra's RunE doesn't compose cleanly across commands without
// forking a child process; the up path already detaches so a subprocess
// is the right call.
func runUpInline(cfg *Config) error {
	cmd := exec.Command(os.Args[0], "up")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if serverReachable(cfg) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("server did not become reachable within 60s")
}

// serverReachable pings /healthz and returns true on 200.
func serverReachable(cfg *Config) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(cfg.ServerURL + "/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// serverDomainExists returns true if the server has a domain with the
// given id. A 404 or a network error returns false (we treat both as
// "doesn't exist" and let the actual register call surface the truth).
func serverDomainExists(cfg *Config, id string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(cfg.ServerURL + "/api/v1/domains/" + id)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// serverDomainVersion returns the latest registered version of id, or
// "" if unknown. Errors are swallowed — the caller uses this for
// informational output, not gating decisions.
func serverDomainVersion(cfg *Config, id string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(cfg.ServerURL + "/api/v1/domains/" + id)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.Version, nil
}

// consoleURLFor is the URL the user opens to interact with the deployed
// Domain. Today this is the API root; in W5 the Console will live on
// app.hnsx.io and we'll point users there instead.
func consoleURLFor(cfg *Config) string {
	return strings.TrimRight(cfg.ServerURL, "/") + "/"
}

// ---------------------------------------------------------------------------
// Cloud target (Phase 2 — W5/W6)
// ---------------------------------------------------------------------------

func runDeployCloud(cmd *cobra.Command, cfg *Config, path string) error {
	out := NewOutputWriter(cfg.Output, cmd.OutOrStdout())
	out.Line("→ target=cloud: checking GitHub auth via `gh`...")

	token, err := ghAuthToken()
	if err != nil {
		out.Line("")
		out.Line("✗ %v", err)
		out.Line("")
		out.Line("Cloud deploy is not yet implemented (Phase 2 / W5-W6).")
		out.Line("For now, use --target local:")
		out.Line("  hnsx deploy %s --target local --up", path)
		out.Line("")
		return err
	}

	out.Line("✓ GitHub token obtained (length=%d)", len(token))
	out.Line("✗ Cloud deploy not yet implemented (Phase 2 / W5-W6).")
	out.Line("  Fallback: hnsx deploy %s --target local --up", path)
	return errors.New("cloud deploy not yet implemented")
}

// ghAuthToken shells out to `gh auth token` to reuse the user's existing
// GitHub CLI authentication. We deliberately do not handle the OAuth
// dance ourselves in Phase 1 — `gh` is the de facto standard.
//
// The function returns an actionable error if `gh` is missing or the
// user hasn't authenticated yet, so deploy can print a "do this next"
// hint rather than just failing.
func ghAuthToken() (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh CLI not found on PATH — install GitHub CLI (https://cli.github.com) and run gh auth login")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "auth", "token")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh auth token failed — run gh auth login first")
	}
	tok := strings.TrimSpace(string(out))
	if tok == "" {
		return "", fmt.Errorf("gh auth token returned an empty token — run gh auth login")
	}
	return tok, nil
}

// ---------------------------------------------------------------------------
// Spec helpers
// ---------------------------------------------------------------------------

// readDomainIDAndVersion parses the top of a Domain YAML without doing
// full schema validation. It's a best-effort read used only for
// informational output and to format the "already exists" error.
func readDomainIDAndVersion(path string) (id, version string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "id:"):
			id = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		case strings.HasPrefix(line, "version:"):
			version = strings.TrimSpace(strings.TrimPrefix(line, "version:"))
		}
		if id != "" && version != "" {
			return id, version, nil
		}
	}
	if id == "" {
		return "", "", fmt.Errorf("domain id not found in %s (expected `id: ...` near the top)", path)
	}
	return id, version, nil
}

// validateFile parses the YAML and checks it against the DomainSpec
// schema. We import the parser directly rather than shelling out to
// the `hnsx validate` subcommand so deploy works under `go test` (where
// os.Args[0] is the test binary, not the hnsx binary) and so the error
// path is one frame shorter to debug.
func validateFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if _, err := domain.Parse(data); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// ensure absolute paths in error messages
var _ = filepath.IsAbs