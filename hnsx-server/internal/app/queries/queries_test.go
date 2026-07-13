package queries

import (
	"testing"

	"github.com/hnsx-io/hnsx/server/internal/domain/repository"
	"github.com/hnsx-io/hnsx/server/internal/domain/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

func queryMinimalSpec(id, version string) *domain.DomainSpec {
	return &domain.DomainSpec{
		ID:      id,
		Version: version,
		Harness: domain.HarnessSpec{
			Agents: map[string]domain.AgentSpec{
				"agent": {
					ID:       "agent",
					Provider: "noop",
					Adapter:  domain.AdapterConfig{Kind: "noop"},
				},
			},
			Session: domain.SessionSpec{Mode: domain.Single, Agent: "agent"},
		},
	}
}

func TestQueries_ListDomainVersions(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := service.NewService(repo)
	q := NewQueries(svc, nil)

	if _, err := svc.Register(tenant.DefaultID, queryMinimalSpec("q-domain", "1.0.0")); err != nil {
		t.Fatalf("register v1: %v", err)
	}
	if _, err := svc.Update(tenant.DefaultID, "q-domain", queryMinimalSpec("q-domain", "1.1.0")); err != nil {
		t.Fatalf("update v2: %v", err)
	}

	versions, ok := q.ListDomainVersions(tenant.DefaultID, "q-domain")
	if !ok {
		t.Fatal("expected ok")
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].Version != "1.1.0" {
		t.Fatalf("expected newest first, got %q", versions[0].Version)
	}

	if _, ok := q.ListDomainVersions(tenant.DefaultID, "missing"); ok {
		t.Fatal("expected not ok for missing domain")
	}
}

func TestQueries_GetDomainSchema(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := service.NewService(repo)
	q := NewQueries(svc, nil)

	spec := queryMinimalSpec("schema-domain", "1.0.0")
	spec.Harness.Session.TriggerSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{"type": "string"},
		},
	}
	spec.Harness.Session.OutputSchema = "{}"
	if _, err := svc.Register(tenant.DefaultID, spec); err != nil {
		t.Fatalf("register: %v", err)
	}

	schema, ok := q.GetDomainSchema(tenant.DefaultID, "schema-domain")
	if !ok {
		t.Fatal("expected ok")
	}
	if schema.ID != "schema-domain" {
		t.Fatalf("id = %q", schema.ID)
	}
	if schema.Mode != string(spec.Harness.Session.Mode) {
		t.Fatalf("mode = %q", schema.Mode)
	}
	if schema.TriggerSchema == nil {
		t.Fatal("expected trigger schema")
	}

	if _, ok := q.GetDomainSchema(tenant.DefaultID, "missing"); ok {
		t.Fatal("expected not ok for missing domain")
	}
}

func TestQueries_GetDomainVersion(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := service.NewService(repo)
	q := NewQueries(svc, nil)

	if _, err := svc.Register(tenant.DefaultID, queryMinimalSpec("q-domain", "1.0.0")); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := svc.Update(tenant.DefaultID, "q-domain", queryMinimalSpec("q-domain", "2.0.0")); err != nil {
		t.Fatalf("update: %v", err)
	}

	d, ok := q.GetDomainVersion(tenant.DefaultID, "q-domain", "1.0.0")
	if !ok {
		t.Fatal("expected ok")
	}
	if d.Version != "1.0.0" {
		t.Fatalf("version = %q", d.Version)
	}
	if d.Harness == nil {
		t.Fatal("expected harness")
	}

	if _, ok := q.GetDomainVersion(tenant.DefaultID, "q-domain", "9.9.9"); ok {
		t.Fatal("expected not ok for missing version")
	}
	if _, ok := q.GetDomainVersion(tenant.DefaultID, "missing", "1.0.0"); ok {
		t.Fatal("expected not ok for missing domain")
	}
}
