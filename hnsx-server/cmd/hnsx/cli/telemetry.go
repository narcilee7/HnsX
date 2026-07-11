package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// telemetryConfig is the opt-in / opt-out flag for client-side telemetry.
// v1.0 ships this as a config-file backed setting; no telemetry is currently
// emitted, so the flag is a no-op until the optional telemetry pipeline
// lands in a follow-up release.
type telemetryConfig struct {
	Enabled bool `yaml:"enabled"`
}

// newTelemetryCmd exposes the opt-in toggle. The setting is persisted to
// ~/.config/hnsx/config.yaml and is honoured by every future code path that
// emits client-side metrics.
func newTelemetryCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Manage opt-in client-side telemetry",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "on",
		Short: "Enable anonymous usage telemetry",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := writeTelemetry(&telemetryConfig{Enabled: true}); err != nil {
				return err
			}
			fmt.Println("✓ telemetry enabled")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "off",
		Short: "Disable all client-side telemetry (default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := writeTelemetry(&telemetryConfig{Enabled: false}); err != nil {
				return err
			}
			fmt.Println("✓ telemetry disabled")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show whether telemetry is enabled",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := readTelemetry()
			if err != nil {
				fmt.Println("telemetry: unknown (no config file)")
				return nil
			}
			state := "off"
			if cfg.Enabled {
				state = "on"
			}
			fmt.Printf("telemetry: %s\n", state)
			return nil
		},
	})
	return cmd
}

func writeTelemetry(c *telemetryConfig) error {
	path := telemetryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	contents := fmt.Sprintf("enabled: %t\n", c.Enabled)
	return os.WriteFile(path, []byte(contents), 0o600)
}

func readTelemetry() (*telemetryConfig, error) {
	b, err := os.ReadFile(telemetryPath())
	if err != nil {
		return nil, err
	}
	var c telemetryConfig
	// Minimal parser — enough for the single boolean we emit.
	if _, err := fmt.Sscanf(string(b), "enabled: %t", &c.Enabled); err != nil {
		return nil, err
	}
	return &c, nil
}

func telemetryPath() string {
	if p := os.Getenv("HNSX_TELEMETRY_FILE"); p != "" {
		return p
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".config", "hnsx", "telemetry.yaml")
	}
	return ".hnsx-telemetry.yaml"
}