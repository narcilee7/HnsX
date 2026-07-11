package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunCmdMissingDomain(t *testing.T) {
	cfg := &Config{Output: "human"}
	cmd := newRunCmd(cfg)
	cmd.SetArgs([]string{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected error for missing --domain")
	}
	if !contains(err.Error(), "--domain is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmdInvalidTrigger(t *testing.T) {
	dir := t.TempDir()
	domainPath := filepath.Join(dir, "domain.yaml")
	body := []byte(`
id: test-run
version: 0.1.0
harness:
  session:
    mode: single
  agents:
    a:
      id: a
      provider: noop
      adapter:
        kind: noop
`)
	if err := os.WriteFile(domainPath, body, 0o644); err != nil {
		t.Fatalf("write temp domain: %v", err)
	}

	cfg := &Config{Output: "human"}
	cmd := newRunCmd(cfg)
	cmd.SetArgs([]string{"--domain", domainPath, "--trigger", "not-json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid trigger")
	}
	if !contains(err.Error(), "parse trigger") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
