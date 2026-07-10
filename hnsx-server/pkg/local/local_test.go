package local

import (
	"strings"
	"testing"
)

func TestValidateDomainYAML(t *testing.T) {
	body := `
id: customer-service
version: "0.1.0"
description: Test domain
harness:
  session:
    mode: single
  agents:
    support:
      id: support
      provider: noop
      adapter:
        kind: noop
`
	summary, err := ValidateDomain(strings.NewReader(body), "application/yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !summary.Valid {
		t.Fatal("expected valid")
	}
	if summary.ID != "customer-service" {
		t.Fatalf("expected id customer-service, got %s", summary.ID)
	}
	if summary.Mode != "single" {
		t.Fatalf("expected mode single, got %s", summary.Mode)
	}
	if summary.AgentCount != 1 {
		t.Fatalf("expected 1 agent, got %d", summary.AgentCount)
	}
}

func TestValidateDomainJSON(t *testing.T) {
	body := `{"id":"test","version":"1.0.0","harness":{"session":{"mode":"single"},"agents":{"a":{"id":"a","provider":"noop","adapter":{"kind":"noop"}}}}}`
	summary, err := ValidateDomain(strings.NewReader(body), "application/json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.ID != "test" {
		t.Fatalf("expected id test, got %s", summary.ID)
	}
	if summary.Mode != "single" {
		t.Fatalf("expected mode single, got %s", summary.Mode)
	}
}

func TestValidateDomainInvalid(t *testing.T) {
	body := `not-valid-yaml-or-json`
	_, err := ValidateDomain(strings.NewReader(body), "")
	if err == nil {
		t.Fatal("expected error for invalid body")
	}
}

func TestPickAdapter(t *testing.T) {
	for _, kind := range []string{"noop", ""} {
		a, err := PickAdapter(kind)
		if err != nil {
			t.Fatalf("pick noop adapter: %v", err)
		}
		if a == nil {
			t.Fatal("expected non-nil adapter")
		}
	}
	if _, err := PickAdapter("unknown"); err == nil {
		t.Fatal("expected error for unknown adapter")
	}
}
