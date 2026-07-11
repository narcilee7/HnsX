package repository

import (
	"testing"

	"github.com/hnsx-io/hnsx/server/internal/domain/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/internal/testutil"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

var pgTestTenant = tenant.DefaultID

func TestPostgresRepository_RegisterAndGet(t *testing.T) {
	database := testutil.OpenTestDB(t)
	defer database.Close()

	repo := NewPostgresRepository(database)
	_ = repo.Delete(pgTestTenant, "test-domain")

	spec := &spec.DomainSpec{
		ID:          "test-domain",
		Version:     "1.0.0",
		Description: "test",
		Harness: spec.HarnessSpec{
			Agents: map[string]spec.AgentSpec{
				"agent": {ID: "agent", Provider: "noop", Adapter: spec.AdapterConfig{Kind: "noop"}},
			},
			Session: spec.SessionSpec{Mode: spec.Single, Agent: "agent"},
		},
	}

	if err := repo.Save(pgTestTenant, &model.RegisteredDomain{
		ID:          spec.ID,
		Version:     spec.Version,
		Description: spec.Description,
		Spec:        spec,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := repo.ByID(pgTestTenant, "test-domain")
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.Version != "1.0.0" {
		t.Fatalf("version = %q", got.Version)
	}
	if got.Spec == nil || got.Spec.ID != "test-domain" {
		t.Fatal("spec not round-tripped")
	}

	exists, err := repo.Exists(pgTestTenant, "test-domain")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Fatal("expected domain to exist")
	}

	list, err := repo.All(pgTestTenant)
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	found := false
	for _, item := range list {
		if item.ID == "test-domain" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("domain not in list")
	}

	if err := repo.Delete(pgTestTenant, "test-domain"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.ByID(pgTestTenant, "test-domain"); err != model.ErrDomainNotFound {
		t.Fatalf("expected ErrDomainNotFound, got %v", err)
	}
}
