package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTelemetry_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telemetry.yaml")
	t.Setenv("HNSX_TELEMETRY_FILE", path)

	// Default: no file → status reports 'unknown'.
	if _, err := readTelemetry(); err == nil {
		t.Fatal("expected error when no telemetry file exists")
	}

	// Enable.
	if err := writeTelemetry(&telemetryConfig{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	cfg, err := readTelemetry()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled {
		t.Fatal("expected enabled=true after write")
	}

	// Disable.
	if err := writeTelemetry(&telemetryConfig{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = readTelemetry()
	if cfg.Enabled {
		t.Fatal("expected enabled=false after second write")
	}
}

func TestTelemetryPath_Override(t *testing.T) {
	t.Setenv("HNSX_TELEMETRY_FILE", "/tmp/custom-tel.yaml")
	if got := telemetryPath(); got != "/tmp/custom-tel.yaml" {
		t.Fatalf("got %q", got)
	}
}

func TestTelemetryDirCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "telemetry.yaml")
	t.Setenv("HNSX_TELEMETRY_FILE", path)
	if err := writeTelemetry(&telemetryConfig{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not written: %v", err)
	}
}