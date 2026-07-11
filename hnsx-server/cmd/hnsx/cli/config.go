// Package cli implements the hnsx operator-facing command line.
//
// v0.3 ("Lifesaver") scope:
//   - cobra + pflag framework
//   - config layer: flag > HNSX_* env
//   - lifecycle commands (up/down/restart/status/doctor/logs/reset)
//   - discovery commands (try/examples/completion)
//   - preserved: validate / run / remote (remote emits deprecation warnings)
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Config holds CLI-level configuration. Server-level config (DB URL, ports,
// OTel exporter) is read directly from HNSX_* env by the server binary.
type Config struct {
	// Output controls rendering: "human" (default), "json", "quiet".
	Output string
	// Verbose enables verbose logging.
	Verbose bool
	// RepoRoot is the HnsX repo root, used to locate deployments/, example-domains/, etc.
	RepoRoot string
	// ComposeFile is the docker-compose file used by lifecycle commands.
	ComposeFile string
	// ServerURL is the base URL of the running hnsx-server (read by remote commands).
	ServerURL string
	// NoTui disables the default TUI when hnsx is run without arguments.
	NoTui bool
	// ConfigFile is the user-level config file path (currently informational; v0.5+).
	ConfigFile string
}

// Default returns a Config populated from HNSX_* environment variables.
func Default() Config {
	cwd, _ := os.Getwd()
	root := findRepoRoot(cwd)
	return Config{
		Output:      envOr("HNSX_OUTPUT", "human"),
		Verbose:     envOr("HNSX_VERBOSE", "false") == "true",
		NoTui:       envOr("HNSX_NO_TUI", "false") == "true",
		RepoRoot:    root,
		ComposeFile: envOr("HNSX_COMPOSE_FILE", filepath.Join(root, "deployments/local/docker-compose.yaml")),
		ServerURL:   envOr("HNSX_SERVER_URL", "http://127.0.0.1:50052"),
		ConfigFile:  defaultConfigFile(),
	}
}

// DefaultConfigFile returns ~/.config/hnsx/config.yaml, or empty if HOME is unset.
func defaultConfigFile() string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".config", "hnsx", "config.yaml")
	}
	return ""
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// findRepoRoot walks up from start looking for the HnsX repo root marker
// (the directory containing both deployments/ and example-domains/).
// Falls back to start if not found.
func findRepoRoot(start string) string {
	dir := start
	for i := 0; i < 6; i++ {
		if dir == "" || dir == "/" {
			break
		}
		if hasMarker(dir) {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	return start
}

func hasMarker(dir string) bool {
	for _, sub := range []string{"deployments", "example-domains"} {
		if _, err := os.Stat(filepath.Join(dir, sub)); err != nil {
			return false
		}
	}
	return true
}

// BindPersistentFlags attaches global flags to the root command.
func BindPersistentFlags(root *cobra.Command, cfg *Config) {
	pf := root.PersistentFlags()
	pf.StringVar(&cfg.Output, "output", cfg.Output, "output format: human|json|quiet")
	pf.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "enable verbose logging")
	pf.StringVar(&cfg.ServerURL, "server", cfg.ServerURL, "hnsx-server base URL")
	pf.StringVar(&cfg.ComposeFile, "compose-file", cfg.ComposeFile, "docker-compose file for lifecycle commands")
	pf.BoolVar(&cfg.NoTui, "no-tui", cfg.NoTui, "disable the default TUI when running without arguments")
}

// ResolveOutput returns the effective output mode, validating the flag value.
func (c *Config) ResolveOutput() (string, error) {
	switch c.Output {
	case "human", "json", "quiet":
		return c.Output, nil
	default:
		return "", fmt.Errorf("invalid --output %q (want human|json|quiet)", c.Output)
	}
}