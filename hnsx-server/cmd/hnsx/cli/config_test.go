package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvOr(t *testing.T) {
	if got := envOr("HNSX_TEST_UNSET", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
	t.Setenv("HNSX_TEST_SET", "value")
	if got := envOr("HNSX_TEST_SET", "fallback"); got != "value" {
		t.Fatalf("expected env value, got %q", got)
	}
}

func TestDefault_ServerURL(t *testing.T) {
	c := Default()
	if c.ServerURL == "" {
		t.Fatal("ServerURL must have a default")
	}
	// The canonical local URL must be the docker-compose-mapped port (50052),
	// not the in-container port (50051).
	if c.ServerURL != "http://127.0.0.1:50052" {
		t.Fatalf("ServerURL default = %q, want http://127.0.0.1:50052", c.ServerURL)
	}
}

func TestDefault_ComposeFile(t *testing.T) {
	c := Default()
	if filepath.Base(c.ComposeFile) != "docker-compose.yaml" {
		t.Fatalf("ComposeFile = %q, expected docker-compose.yaml", c.ComposeFile)
	}
}

func TestFindRepoRoot(t *testing.T) {
	root := findRepoRoot(t.TempDir())
	if root == "" {
		t.Fatal("findRepoRoot returned empty for empty dir")
	}
	// From this package dir the repo root should resolve back to the HnsX root.
	got := findRepoRoot(filepath.Join(Default().RepoRoot, "hnsx-server", "cmd", "hnsx", "cli"))
	if !hasMarker(got) {
		t.Fatalf("findRepoRoot did not resolve to a marked root: %s", got)
	}
}

func TestResolveOutput(t *testing.T) {
	c := Default()
	for _, mode := range []string{"human", "json", "quiet"} {
		c.Output = mode
		got, err := c.ResolveOutput()
		if err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
		if got != mode {
			t.Fatalf("mode %q: got %q", mode, got)
		}
	}
	c.Output = "yaml"
	if _, err := c.ResolveOutput(); err == nil {
		t.Fatal("expected error for invalid output mode")
	}
}

func TestHasMarker(t *testing.T) {
	// The actual repo root must satisfy the marker check.
	if !hasMarker(Default().RepoRoot) {
		t.Skip("running outside HnsX repo; skipping marker check")
	}
	tmp := t.TempDir()
	if hasMarker(tmp) {
		t.Fatal("empty tmp dir should not have markers")
	}
}

func TestFileExists(t *testing.T) {
	if !fileExists(Default().ComposeFile) {
		t.Skip("compose file not present in this env")
	}
	if fileExists(filepath.Join(t.TempDir(), "nope")) {
		t.Fatal("expected false for non-existent path")
	}
	_ = os.Getenv // keep imports stable
}