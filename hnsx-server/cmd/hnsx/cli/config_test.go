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

func TestLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`output: json
verbose: true
server_url: http://example.com:8080
compose_file: /custom/compose.yaml
no_tui: true
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	fc, err := loadConfigFile(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if fc.Output != "json" {
		t.Fatalf("output = %q", fc.Output)
	}
	if !fc.Verbose {
		t.Fatal("verbose should be true")
	}
	if fc.ServerURL != "http://example.com:8080" {
		t.Fatalf("server_url = %q", fc.ServerURL)
	}
}

func TestLoadConfigFile_MissingIsNotError(t *testing.T) {
	fc, err := loadConfigFile(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if fc.Output != "" {
		t.Fatalf("expected empty output, got %q", fc.Output)
	}
}

func TestConfigPriority_EnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("output: json\nserver_url: http://file.example\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("HNSX_CONFIG", path)
	t.Setenv("HNSX_OUTPUT", "quiet")
	cfg := Default()
	if cfg.Output != "quiet" {
		t.Fatalf("env should override file: got %q", cfg.Output)
	}
	if cfg.ServerURL != "http://file.example" {
		t.Fatalf("file value should remain: got %q", cfg.ServerURL)
	}
}

func TestConfigGetSet(t *testing.T) {
	cfg := Default()
	if err := cfg.Set("output", "json"); err != nil {
		t.Fatalf("set output: %v", err)
	}
	v, err := cfg.Get("output")
	if err != nil {
		t.Fatalf("get output: %v", err)
	}
	if v != "json" {
		t.Fatalf("output = %q", v)
	}
	if err := cfg.Set("output", "invalid"); err == nil {
		t.Fatal("expected error for invalid output")
	}
	if err := cfg.Set("unknown", "x"); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestConfigSaveToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hnsx", "config.yaml")
	cfg := Config{
		ConfigFile:  path,
		Output:      "json",
		Verbose:     true,
		ServerURL:   "http://save.example",
		ComposeFile: "/save/compose.yaml",
		NoTui:       true,
		Token:       "secret-token",
	}
	if err := cfg.SaveToFile(); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := loadConfigFile(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.Output != "json" || !loaded.Verbose || loaded.ServerURL != "http://save.example" {
		t.Fatalf("saved values mismatch: %+v", loaded)
	}
	if loaded.Token != "secret-token" {
		t.Fatalf("token not saved: %q", loaded.Token)
	}
}

func TestMaskToken(t *testing.T) {
	if got := maskToken(""); got != "-" {
		t.Fatalf("empty token mask = %q", got)
	}
	if got := maskToken("short"); got != "***" {
		t.Fatalf("short token mask = %q", got)
	}
	if got := maskToken("abcdefghij"); got != "abcd...ghij" {
		t.Fatalf("long token mask = %q", got)
	}
}

func TestParseBool(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"true", true},
		{"1", true},
		{"yes", true},
		{"on", true},
		{"TRUE", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"", false},
	} {
		if got := parseBool(tc.in); got != tc.want {
			t.Fatalf("parseBool(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}