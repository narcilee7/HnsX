package service

import (
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/model"
	"github.com/hnsx-io/hnsx/server/internal/domain/repository"
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

func TestService_RegisterAndGet(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := NewService(repo)

	spec := minimalSpec("test-domain", "1.0.0")
	spec.Description = "test"

	d, err := svc.Register(testTenant, spec)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if d.ID != "test-domain" {
		t.Fatalf("id = %q", d.ID)
	}
	if d.CreatedAt.IsZero() || d.UpdatedAt.IsZero() {
		t.Fatal("missing timestamps")
	}

	got, err := svc.Get(testTenant, "test-domain")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Version != "1.0.0" {
		t.Fatalf("version = %q", got.Version)
	}
}

func TestService_RegisterDuplicate(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := NewService(repo)

	spec := minimalSpec("dup-domain", "1.0.0")
	if _, err := svc.Register(testTenant, spec); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if _, err := svc.Register(testTenant, spec); err != model.ErrDomainExists {
		t.Fatalf("expected ErrDomainExists, got %v", err)
	}
}

func TestService_Update(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := NewService(repo)

	before := time.Now().UTC().Add(-time.Second)
	spec := minimalSpec("update-domain", "1.0.0")
	if _, err := svc.Register(testTenant, spec); err != nil {
		t.Fatalf("register: %v", err)
	}

	updated := minimalSpec("update-domain", "1.1.0")
	d, err := svc.Update(testTenant, "update-domain", updated)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if d.Version != "1.1.0" {
		t.Fatalf("version = %q", d.Version)
	}
	if !d.UpdatedAt.After(before) {
		t.Fatalf("updated_at not refreshed: %v", d.UpdatedAt)
	}
}

func TestService_ListAndDelete(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	svc := NewService(repo)

	for _, id := range []string{"a", "b", "c"} {
		spec := minimalSpec(id, "1.0.0")
		if _, err := svc.Register(testTenant, spec); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}

	list, err := svc.List(testTenant)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len(list) = %d", len(list))
	}

	if err := svc.Delete(testTenant, "b"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(testTenant, "b"); err != model.ErrDomainNotFound {
		t.Fatalf("expected ErrDomainNotFound, got %v", err)
	}
}

func TestRegisteredDomain_Validate(t *testing.T) {
	if err := (&model.RegisteredDomain{ID: "x", Spec: &domain.DomainSpec{}}).Validate(); err != nil {
		t.Fatalf("valid domain: %v", err)
	}
	if err := (&model.RegisteredDomain{ID: "", Spec: &domain.DomainSpec{}}).Validate(); err != model.ErrInvalidSpec {
		t.Fatalf("expected ErrInvalidSpec for empty id, got %v", err)
	}
	if err := (&model.RegisteredDomain{ID: "x"}).Validate(); err != model.ErrInvalidSpec {
		t.Fatalf("expected ErrInvalidSpec for nil spec, got %v", err)
	}
}
