package spec

import (
	"strings"
	"testing"
)

const baseSpec = `
id: customer-service
version: 0.2.0
description: Routes customer questions to the right specialist agent.
harness:
  agents:
    triage:
      id: triage
      provider: anthropic
      model: claude-haiku-4-5
      adapter:
        kind: anthropic
  prompts:
    triage-prompt:
      type: system
      template: "You are a triage agent."
  session:
    mode: workflow
    workflow:
      entry: classify
      steps:
        - id: classify
          agent: triage
          next: respond
          output: classification
        - id: respond
          agent: triage
`

func TestParse_OK(t *testing.T) {
	spec, err := Parse([]byte(baseSpec))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if spec.ID != "customer-service" {
		t.Errorf("ID = %q, want customer-service", spec.ID)
	}
	if got := spec.Harness.Session.Mode; got != "workflow" {
		t.Errorf("Mode = %q, want workflow", got)
	}
	if len(spec.Harness.Agents) != 1 {
		t.Errorf("len(agents) = %d, want 1", len(spec.Harness.Agents))
	}
}

func TestParse_MissingID(t *testing.T) {
	bad := `
version: 0.1.0
harness:
  agents:
    a:
      provider: anthropic
      model: m
      adapter:
        kind: anthropic
  session:
    mode: single
`
	_, err := Parse([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "domain.id") {
		t.Fatalf("expected id error, got %v", err)
	}
}

func TestParse_MissingAgents(t *testing.T) {
	bad := `
id: x
version: 0.1.0
harness:
  agents: {}
  session:
    mode: single
`
	_, err := Parse([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "agents cannot be empty") {
		t.Fatalf("expected agents error, got %v", err)
	}
}

func TestParse_UnknownSessionMode(t *testing.T) {
	bad := strings.Replace(baseSpec, "mode: workflow", "mode: wizardry", 1)
	_, err := Parse([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "unknown session mode") {
		t.Fatalf("expected mode error, got %v", err)
	}
}

func TestParse_WorkflowReferencesUnknownAgent(t *testing.T) {
	bad := strings.Replace(baseSpec, "agent: triage", "agent: unknown-agent", 1)
	_, err := Parse([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "references unknown agent") {
		t.Fatalf("expected agent-ref error, got %v", err)
	}
}

func TestParse_WorkflowEntryMissing(t *testing.T) {
	// entry = "ghost" not present in steps
	bad := strings.Replace(baseSpec, "entry: classify", "entry: ghost", 1)
	_, err := Parse([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "entry") {
		t.Fatalf("expected entry error, got %v", err)
	}
}

func TestValidate_NilSpec(t *testing.T) {
	if err := Validate(nil); err == nil {
		t.Fatal("expected nil error")
	}
}

func TestValidate_FieldChecks(t *testing.T) {
	cases := []struct {
		name string
		spec *DomainSpec
		want string
	}{
		{
			name: "missing id",
			spec: &DomainSpec{Version: "0.1.0", Harness: HarnessSpec{
				Agents:  map[string]AgentSpec{"a": {Provider: "anthropic", Model: "m"}},
				Session: SessionSpec{Mode: "single"},
			}},
			want: "domain.id is required",
		},
		{
			name: "missing version",
			spec: &DomainSpec{ID: "x", Harness: HarnessSpec{
				Agents:  map[string]AgentSpec{"a": {Provider: "anthropic", Model: "m"}},
				Session: SessionSpec{Mode: "single"},
			}},
			want: "domain.version is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.spec)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate(%s) = %v, want %q", tc.name, err, tc.want)
			}
		})
	}
}

func TestMustParse_PanicOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got none")
		}
	}()
	MustParse([]byte("not yaml: : :"))
}
