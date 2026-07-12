package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewCmd_BlankScaffoldsFolder verifies the happy path: `hnsx new`
// creates a per-domain folder with domain.yaml, README.md, and .gitignore.
func TestNewCmd_BlankScaffoldsFolder(t *testing.T) {
	parent := t.TempDir()
	cfg := Default()
	cmd := newNewCmd(&cfg)
	cmd.SetArgs([]string{"my-domain", "--output-dir", parent})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	dir := filepath.Join(parent, "my-domain")
	for _, name := range []string{"domain.yaml", "README.md", ".gitignore"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s to exist: %v", path, err)
		}
	}

	yaml, err := os.ReadFile(filepath.Join(dir, "domain.yaml"))
	if err != nil {
		t.Fatalf("read domain.yaml: %v", err)
	}
	if !strings.Contains(string(yaml), "id: my-domain") {
		t.Errorf("expected domain id in domain.yaml, got:\n%s", string(yaml))
	}

	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	if !strings.Contains(string(readme), "my-domain") {
		t.Errorf("expected domain id in README.md")
	}
	if !strings.Contains(string(readme), "hnsx new") {
		t.Errorf("expected README.md to mention hnsx new")
	}

	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(gi), ".eval-cache/") {
		t.Errorf("expected .gitignore to cover .eval-cache/")
	}
}

// TestNewCmd_CustomerServiceWithSet verifies --set propagates into the
// rendered domain.yaml.
func TestNewCmd_CustomerServiceWithSet(t *testing.T) {
	parent := t.TempDir()
	cfg := Default()
	cmd := newNewCmd(&cfg)
	cmd.SetArgs([]string{
		"cs",
		"--template", "customer-service",
		"--output-dir", parent,
		"--set", "company_name=TestCo",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	yaml, err := os.ReadFile(filepath.Join(parent, "cs", "domain.yaml"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	rendered := string(yaml)
	if !strings.Contains(rendered, "id: cs") {
		t.Errorf("expected id cs, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Customer service triage for TestCo") {
		t.Errorf("expected --set company_name replacement, got:\n%s", rendered)
	}
}

// TestNewCmd_UnknownTemplate verifies a clear error for bad template ids.
func TestNewCmd_UnknownTemplate(t *testing.T) {
	cfg := Default()
	cmd := newNewCmd(&cfg)
	cmd.SetArgs([]string{"x", "--template", "does-not-exist", "--output-dir", t.TempDir()})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for unknown template")
	}
}

// TestNewCmd_RejectsExistingDirWithoutForce verifies we don't silently
// overwrite an existing directory.
func TestNewCmd_RejectsExistingDirWithoutForce(t *testing.T) {
	parent := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parent, "my-domain"), 0o755); err != nil {
		t.Fatalf("pre-create: %v", err)
	}
	cfg := Default()
	cmd := newNewCmd(&cfg)
	cmd.SetArgs([]string{"my-domain", "--output-dir", parent})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when target dir already exists")
	}
}

// TestNewCmd_ForceOverwrites verifies --force succeeds when target exists.
func TestNewCmd_ForceOverwrites(t *testing.T) {
	parent := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parent, "my-domain"), 0o755); err != nil {
		t.Fatalf("pre-create: %v", err)
	}
	cfg := Default()
	cmd := newNewCmd(&cfg)
	cmd.SetArgs([]string{"my-domain", "--output-dir", parent, "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected --force to succeed: %v", err)
	}
}

// TestNewCmd_RejectsInvalidDomainID verifies path-traversal hardening.
func TestNewCmd_RejectsInvalidDomainID(t *testing.T) {
	cases := []string{
		"../etc",     // contains slash
		"..",         // reserved
		".",          // reserved
		"9-leading",  // must start with a letter
		"has space",  // contains space
		"UPPER",      // uppercase not allowed (k8s-style id)
		"slash/in/id", // contains slash
	}
	for _, id := range cases {
		t.Run(id, func(t *testing.T) {
			cfg := Default()
			cmd := newNewCmd(&cfg)
			cmd.SetArgs([]string{id, "--output-dir", t.TempDir()})
			if err := cmd.Execute(); err == nil {
				t.Errorf("expected error for invalid id %q", id)
			}
		})
	}
}

// TestNewCmd_RejectsInvalidSet verifies --set must be key=value.
func TestNewCmd_RejectsInvalidSet(t *testing.T) {
	cfg := Default()
	cmd := newNewCmd(&cfg)
	cmd.SetArgs([]string{"x", "--set", "noequals"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid --set")
	}
}

// TestNewCmd_ListTemplates verifies --list exits cleanly and lists built-ins.
func TestNewCmd_ListTemplates(t *testing.T) {
	cfg := Default()
	cmd := newNewCmd(&cfg)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	cmd.SetArgs([]string{"--list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--list should not error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"blank", "customer-service", "code-review", "research-assistant"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected --list output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestNewCmd_ListJSON confirms --list --output json returns valid JSON
// containing the built-in template ids.
func TestNewCmd_ListJSON(t *testing.T) {
	cfg := Default()
	cfg.Output = "json"
	cmd := newNewCmd(&cfg)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	cmd.SetArgs([]string{"--list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--list --output json should not error: %v", err)
	}

	var got struct {
		BuiltIn []map[string]string `json:"built_in"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("parse: %v\nraw:\n%s", err, buf.String())
	}
	if len(got.BuiltIn) == 0 {
		t.Errorf("expected non-empty built_in list, got: %v", got)
	}
}

// TestNewCmd_JSONOutput verifies the --output json mode emits a parseable
// object with the expected keys.
func TestNewCmd_JSONOutput(t *testing.T) {
	parent := t.TempDir()
	cfg := Default()
	cfg.Output = "json"
	cmd := newNewCmd(&cfg)

	// Capture cobra's stdout into a buffer so we can parse the JSON.
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	cmd.SetArgs([]string{"my-domain", "--output-dir", parent})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("parse json output: %v\nraw:\n%s", err, buf.String())
	}
	for _, key := range []string{"path", "template", "source", "domain", "files"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing key %q in json output: %v", key, got)
		}
	}
	if got["domain"] != "my-domain" {
		t.Errorf("domain = %v, want my-domain", got["domain"])
	}
}