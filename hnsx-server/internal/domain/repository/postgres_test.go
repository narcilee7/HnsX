package repository

import (
	"testing"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
	"github.com/hnsx-io/hnsx/server/internal/domain/model"
	"github.com/hnsx-io/hnsx/server/internal/testutil"
)

func TestPostgresRepository_RegisterAndGet(t *testing.T) {
	database := testutil.OpenTestDB(t)
	defer database.Close()

	repo := NewPostgresRepository(database)
	_ = repo.Delete("test-domain")

	spec := &spec.DomainSpec{
		ID:          "test-domain",
		Version:     "1.0.0",
		Description: "test",
		Harness: spec.HarnessSpec{
			Agents: map[string]spec.AgentSpec{
				"agent": {ID: "agent", Provider: "noop", Adapter: spec.AdapterConfig{Kind: "noop"}},
			},
			Session: spec.SessionSpec{Mode: "single", Agent: "agent"},
		},
	}

	if err := repo.Save(&model.RegisteredDomain{
		ID:          spec.ID,
		Version:     spec.Version,
		Description: spec.Description,
		Spec:        spec,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := repo.ByID("test-domain")
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.Version != "1.0.0" {
		t.Fatalf("version = %q", got.Version)
	}
	if got.Spec == nil || got.Spec.ID != "test-domain" {
		t.Fatal("spec not round-tripped")
	}

	exists, err := repo.Exists("test-domain")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Fatal("expected domain to exist")
	}

	list, err := repo.All()
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

	if err := repo.Delete("test-domain"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.ByID("test-domain"); err != model.ErrDomainNotFound {
		t.Fatalf("expected ErrDomainNotFound, got %v", err)
	}
}
