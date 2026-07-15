// Package config holds HarnessX daemon configuration.
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the daemon's runtime configuration. It is resolved from (in
// order): an optional YAML file, environment variables prefixed with
// HARNESSX_DAEMON_, and built-in defaults.
type Config struct {
	// ServerURL is the base URL of the HarnessX server's REST + WS surface.
	ServerURL string `yaml:"server_url"`

	// AuthToken is the workspace-scoped auth token. The Multica server
	// issues mat_* tokens per daemon registration.
	AuthToken string `yaml:"auth_token"`

	// WorkspaceID is the workspace the daemon registers under.
	WorkspaceID string `yaml:"workspace_id"`

	// DaemonID, when non-empty, identifies the daemon across restarts. The
	// server may overwrite it on Register with a server-assigned id.
	DaemonID string `yaml:"daemon_id"`

	// RuntimeProfiles declares the agent CLIs the daemon can spawn. Each
	// profile maps a logical name ("claude", "codex", "codebuddy") to the
	// CLI binary path and any extra env / args.
	RuntimeProfiles []RuntimeProfile `yaml:"runtime_profiles"`

	// Verbose enables per-observation debug logging.
	Verbose bool `yaml:"verbose"`
}

// RuntimeProfile describes one agent CLI the daemon can spawn.
type RuntimeProfile struct {
	// Name is the logical id reported to the server (e.g. "claude").
	Name string `yaml:"name"`
	// Command is the binary path. Empty means "auto-detect via PATH".
	Command string `yaml:"command"`
	// Args is the default argv (stream-json output flags, model, etc.).
	Args []string `yaml:"args"`
	// Env carries extra environment variables passed to the subprocess.
	Env map[string]string `yaml:"env"`
}

// Load reads the YAML config file at path (when set) and merges HARNESSX_DAEMON_*
// env overrides on top. Returns defaults when path is empty.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}
	if v := os.Getenv("HARNESSX_DAEMON_SERVER_URL"); v != "" {
		cfg.ServerURL = v
	}
	if v := os.Getenv("HARNESSX_DAEMON_AUTH_TOKEN"); v != "" {
		cfg.AuthToken = v
	}
	if v := os.Getenv("HARNESSX_DAEMON_WORKSPACE_ID"); v != "" {
		cfg.WorkspaceID = v
	}
	if v := os.Getenv("HARNESSX_DAEMON_DAEMON_ID"); v != "" {
		cfg.DaemonID = v
	}
	return cfg, nil
}

// Default returns a Config populated with sensible defaults.
func Default() *Config {
	return &Config{
		ServerURL:   "http://127.0.0.1:50051",
		DaemonID:    "harnessx-local",
		WorkspaceID: "00000000-0000-0000-0000-000000000000",
		RuntimeProfiles: []RuntimeProfile{
			{Name: "claude", Command: "", Args: []string{"-p", "--output-format", "stream-json", "--verbose"}},
			{Name: "codex", Command: "", Args: []string{"exec", "--json"}},
			{Name: "codebuddy", Command: ""},
			{Name: "copilot", Command: ""},
			{Name: "cursor", Command: ""},
		},
	}
}
