package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCmd_Blank(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cmd := newInitCmd(&cfg)
	cmd.SetArgs([]string{"my-domain", "--template", "blank", "--output-dir", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	path := filepath.Join(dir, "domain.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated domain: %v", err)
	}
	if !strings.Contains(string(data), "id: my-domain") {
		t.Errorf("expected domain id in output, got:\n%s", string(data))
	}
}

func TestInitCmd_CustomerServiceWithSet(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cmd := newInitCmd(&cfg)
	cmd.SetArgs([]string{
		"cs",
		"--template", "customer-service",
		"--output-dir", dir,
		"--set", "company_name=TestCo",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	path := filepath.Join(dir, "domain.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated domain: %v", err)
	}
	rendered := string(data)
	if !strings.Contains(rendered, "id: cs") {
		t.Errorf("expected id cs, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Customer service triage for TestCo") {
		t.Errorf("expected --set company_name replacement, got:\n%s", rendered)
	}
}

func TestInitCmd_UnknownTemplate(t *testing.T) {
	cfg := Default()
	cmd := newInitCmd(&cfg)
	cmd.SetArgs([]string{"--template", "does-not-exist"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for unknown template")
	}
}

func TestInitCmd_InvalidSet(t *testing.T) {
	cfg := Default()
	cmd := newInitCmd(&cfg)
	cmd.SetArgs([]string{"--set", "noequals"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid --set")
	}
}
