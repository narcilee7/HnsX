package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillResolver_LoadDirLayout(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "code-review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := `---
name: code-review
description: Reviews code changes for style and correctness
---
Review the diff for:
1. Style violations
2. Missing tests
3. Security smells
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewSkillResolver(dir)
	s, err := r.Load(context.Background(), "code-review")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Name != "code-review" {
		t.Errorf("Name = %q, want code-review", s.Name)
	}
	if !contains(s.Body, "Style violations") {
		t.Errorf("body missing expected content; got %q", s.Body)
	}
}

func TestSkillResolver_LoadFlatLayout(t *testing.T) {
	dir := t.TempDir()
	md := `---
name: triage
---
Triage incoming issues.
`
	if err := os.WriteFile(filepath.Join(dir, "triage.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewSkillResolver(dir)
	s, err := r.Load(context.Background(), "triage")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Name != "triage" {
		t.Errorf("Name = %q, want triage", s.Name)
	}
}

func TestSkillResolver_NotFound(t *testing.T) {
	dir := t.TempDir()
	r := NewSkillResolver(dir)
	if _, err := r.Load(context.Background(), "missing"); err != ErrSkillNotFound {
		t.Errorf("expected ErrSkillNotFound; got %v", err)
	}
}

func TestSkillResolver_List(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "SKILL.md"),
			[]byte("---\nname: "+name+"\n---\nbody"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A directory without SKILL.md should be ignored.
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := NewSkillResolver(dir)
	got, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 skills; got %v", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
