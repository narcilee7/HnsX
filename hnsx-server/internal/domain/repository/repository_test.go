package repository

import (
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

var testTenant = tenant.DefaultID

func minimalSpec(id, version string) *domain.DomainSpec {
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

func TestInMemoryRepository_ListVersions(t *testing.T) {
	repo := NewInMemoryRepository()

	spec1 := minimalSpec("domain-a", "1.0.0")
	spec2 := minimalSpec("domain-a", "1.1.0")

	if err := repo.Save(testTenant, &model.RegisteredDomain{ID: "domain-a", Version: "1.0.0", Spec: spec1}); err != nil {
		t.Fatalf("save v1: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := repo.Save(testTenant, &model.RegisteredDomain{ID: "domain-a", Version: "1.1.0", Spec: spec2}); err != nil {
		t.Fatalf("save v2: %v", err)
	}

	versions, err := repo.ListVersions(testTenant, "domain-a")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].Version != "1.1.0" {
		t.Fatalf("expected newest version first, got %q", versions[0].Version)
	}
	if versions[1].Version != "1.0.0" {
		t.Fatalf("expected older version second, got %q", versions[1].Version)
	}

	if _, err := repo.ListVersions(testTenant, "missing"); err != model.ErrDomainNotFound {
		t.Fatalf("expected ErrDomainNotFound, got %v", err)
	}
}

func TestInMemoryRepository_GetVersion(t *testing.T) {
	repo := NewInMemoryRepository()

	spec1 := minimalSpec("domain-b", "1.0.0")
	spec2 := minimalSpec("domain-b", "2.0.0")

	if err := repo.Save(testTenant, &model.RegisteredDomain{ID: "domain-b", Version: "1.0.0", Spec: spec1}); err != nil {
		t.Fatalf("save v1: %v", err)
	}
	if err := repo.Save(testTenant, &model.RegisteredDomain{ID: "domain-b", Version: "2.0.0", Spec: spec2}); err != nil {
		t.Fatalf("save v2: %v", err)
	}

	got, err := repo.GetVersion(testTenant, "domain-b", "1.0.0")
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if got.Version != "1.0.0" {
		t.Fatalf("version = %q", got.Version)
	}

	if _, err := repo.GetVersion(testTenant, "domain-b", "9.9.9"); err != model.ErrDomainNotFound {
		t.Fatalf("expected ErrDomainNotFound, got %v", err)
	}
	if _, err := repo.GetVersion(testTenant, "missing", "1.0.0"); err != model.ErrDomainNotFound {
		t.Fatalf("expected ErrDomainNotFound, got %v", err)
	}
}
