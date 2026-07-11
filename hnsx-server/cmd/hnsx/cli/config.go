// Package cli implements the hnsx operator-facing command line.
//
// v0.3 ("Lifesaver") scope:
//   - cobra + pflag framework
//   - config layer: flag > HNSX_* env > ~/.config/hnsx/config.yaml
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
	"gopkg.in/yaml.v3"
)

// fileConfig is the on-disk shape of ~/.config/hnsx/config.yaml.
// Only user-overridable CLI knobs are listed; server-level config is
// read by the server binary from HNSX_* env directly.
type fileConfig struct {
	Output      string `yaml:"output,omitempty"`
	Verbose     bool   `yaml:"verbose,omitempty"`
	ServerURL   string `yaml:"server_url,omitempty"`
	ComposeFile string `yaml:"compose_file,omitempty"`
	NoTui       bool   `yaml:"no_tui,omitempty"`
	Token       string `yaml:"token,omitempty"`
}

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
	// ConfigFile is the user-level config file path.
	ConfigFile string
	// Token is the bearer token used to authenticate to hnsx-server.
	Token string
}

// Default returns a Config populated from (in order of precedence):
//  1. Built-in defaults.
//  2. ~/.config/hnsx/config.yaml (or HNSX_CONFIG / --config).
//  3. HNSX_* environment variables.
//
// Command-line flags, when parsed, override all of the above.
func Default() Config {
	cwd, _ := os.Getwd()
	root := findRepoRoot(cwd)
	cfg := Config{
		Output:      "human",
		ServerURL:   "http://127.0.0.1:50052",
		ComposeFile: filepath.Join(root, "deployments/local/docker-compose.yaml"),
		RepoRoot:    root,
		ConfigFile:  defaultConfigFile(),
	}

	// Allow HNSX_CONFIG or --config to change the file path before we load it.
	configPath := envOr("HNSX_CONFIG", cfg.ConfigFile)
	if v := findFlagValue(os.Args, "config"); v != "" {
		configPath = v
	}
	if configPath != "" {
		cfg.ConfigFile = configPath
		if loaded, err := loadConfigFile(configPath); err == nil {
			mergeFileConfig(&cfg, loaded)
		}
	}

	// Environment overrides config file.
	if v := strings.TrimSpace(os.Getenv("HNSX_OUTPUT")); v != "" {
		cfg.Output = v
	}
	if v := strings.TrimSpace(os.Getenv("HNSX_VERBOSE")); v == "true" {
		cfg.Verbose = true
	}
	if v := strings.TrimSpace(os.Getenv("HNSX_NO_TUI")); v == "true" {
		cfg.NoTui = true
	}
	if v := strings.TrimSpace(os.Getenv("HNSX_SERVER_URL")); v != "" {
		cfg.ServerURL = v
	}
	if v := strings.TrimSpace(os.Getenv("HNSX_COMPOSE_FILE")); v != "" {
		cfg.ComposeFile = v
	}

	return cfg
}

// loadConfigFile reads a YAML config file into fileConfig. Missing files are
// not an error so first-run behaviour stays silent.
func loadConfigFile(path string) (fileConfig, error) {
	var fc fileConfig
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fc, nil
		}
		return fc, err
	}
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return fc, fmt.Errorf("parse %s: %w", path, err)
	}
	return fc, nil
}

func mergeFileConfig(dst *Config, src fileConfig) {
	if src.Output != "" {
		dst.Output = src.Output
	}
	if src.Verbose {
		dst.Verbose = true
	}
	if src.ServerURL != "" {
		dst.ServerURL = src.ServerURL
	}
	if src.ComposeFile != "" {
		dst.ComposeFile = src.ComposeFile
	}
	if src.NoTui {
		dst.NoTui = true
	}
	if src.Token != "" {
		dst.Token = src.Token
	}
}

// findFlagValue performs a lightweight pre-scan of os.Args for --name=value
// or --name value. It is used before cobra parses flags so that config-file
// location can be discovered early.
func findFlagValue(args []string, name string) string {
	prefix := "--" + name + "="
	for i, a := range args {
		if strings.HasPrefix(a, prefix) {
			return strings.TrimPrefix(a, prefix)
		}
		if a == "--"+name {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
	}
	return ""
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
	pf.StringVar(&cfg.ConfigFile, "config", cfg.ConfigFile, "path to user config file")
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

// Get returns a single config value by dot-separated key. Supported keys:
// output, verbose, server_url, compose_file, no_tui, config_file.
func (c *Config) Get(key string) (string, error) {
	switch key {
	case "output":
		return c.Output, nil
	case "verbose":
		return fmt.Sprintf("%t", c.Verbose), nil
	case "server_url":
		return c.ServerURL, nil
	case "compose_file":
		return c.ComposeFile, nil
	case "no_tui":
		return fmt.Sprintf("%t", c.NoTui), nil
	case "config_file":
		return c.ConfigFile, nil
	case "token":
		return c.Token, nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

// Set updates a single config value in memory. The caller is responsible for
// persisting with SaveToFile.
func (c *Config) Set(key, value string) error {
	switch key {
	case "output":
		if _, err := (&Config{Output: value}).ResolveOutput(); err != nil {
			return err
		}
		c.Output = value
	case "verbose":
		c.Verbose = parseBool(value)
	case "server_url":
		c.ServerURL = value
	case "compose_file":
		c.ComposeFile = value
	case "no_tui":
		c.NoTui = parseBool(value)
	case "token":
		c.Token = value
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

// SaveToFile writes the current config back to its ConfigFile path.
func (c *Config) SaveToFile() error {
	if c.ConfigFile == "" {
		return fmt.Errorf("no config file path configured")
	}
	if err := os.MkdirAll(filepath.Dir(c.ConfigFile), 0o755); err != nil {
		return err
	}
	fc := fileConfig{
		Output:      c.Output,
		Verbose:     c.Verbose,
		ServerURL:   c.ServerURL,
		ComposeFile: c.ComposeFile,
		NoTui:       c.NoTui,
		Token:       c.Token,
	}
	data, err := yaml.Marshal(fc)
	if err != nil {
		return err
	}
	return os.WriteFile(c.ConfigFile, data, 0o644)
}

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}

// ToMap returns the config as a flat map for display.
func (c *Config) ToMap() map[string]string {
	return map[string]string{
		"output":       c.Output,
		"verbose":      fmt.Sprintf("%t", c.Verbose),
		"server_url":   c.ServerURL,
		"compose_file": c.ComposeFile,
		"no_tui":       fmt.Sprintf("%t", c.NoTui),
		"config_file":  c.ConfigFile,
		"token":        maskToken(c.Token),
	}
}

// maskToken returns a masked token for display, or "-" when empty.
func maskToken(token string) string {
	if token == "" {
		return "-"
	}
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
