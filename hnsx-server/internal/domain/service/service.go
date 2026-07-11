// Package service implements the domain application use cases.
//
// It sits between the infrastructure adapters (HTTP/gRPC handlers) and the
// repository, enforcing invariants such as idempotency, validation, and
// timestamp bookkeeping.
package service

import (
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/model"
	"github.com/hnsx-io/hnsx/server/internal/domain/repository"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// Service implements the domain application use cases.
type Service struct {
	repo repository.Repository
}

// NewService constructs a Service backed by the supplied repository.
func NewService(repo repository.Repository) *Service {
	return &Service{repo: repo}
}

// Register validates and stores a new domain spec scoped to a tenant.
//
// If a domain with the same ID already exists, model.ErrDomainExists is returned.
// The spec is validated through the canonical loader before persistence.
func (s *Service) Register(tenantID tenant.ID, ds *spec.DomainSpec) (*model.RegisteredDomain, error) {
	if ds == nil {
		return nil, model.ErrInvalidSpec
	}
	if err := spec.Validate(ds); err != nil {
		return nil, err
	}
	if exists, err := s.repo.Exists(tenantID, ds.ID); err != nil {
		return nil, err
	} else if exists {
		return nil, model.ErrDomainExists
	}

	now := time.Now().UTC()
	d := &model.RegisteredDomain{
		ID:          ds.ID,
		Version:     ds.Version,
		Description: ds.Description,
		Spec:        ds,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.Save(tenantID, d); err != nil {
		return nil, err
	}
	return d, nil
}

// Update replaces an existing domain spec scoped to a tenant.
//
// The ID in the spec must match the URL/id parameter; mismatches return
// model.ErrInvalidSpec.
func (s *Service) Update(tenantID tenant.ID, id string, ds *spec.DomainSpec) (*model.RegisteredDomain, error) {
	if ds == nil || ds.ID != id {
		return nil, model.ErrInvalidSpec
	}
	if err := spec.Validate(ds); err != nil {
		return nil, err
	}
	existing, err := s.repo.ByID(tenantID, id)
	if err != nil {
		return nil, err
	}

	existing.Version = ds.Version
	existing.Description = ds.Description
	existing.Spec = ds
	existing.UpdatedAt = time.Now().UTC()

	if err := s.repo.Save(tenantID, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// Get returns a single domain by ID scoped to a tenant.
func (s *Service) Get(tenantID tenant.ID, id string) (*model.RegisteredDomain, error) {
	return s.repo.ByID(tenantID, id)
}

// ListVersions returns every stored version for a domain, newest first.
func (s *Service) ListVersions(tenantID tenant.ID, id string) ([]repository.VersionRecord, error) {
	return s.repo.ListVersions(tenantID, id)
}

// GetVersion returns the spec for a specific domain version.
func (s *Service) GetVersion(tenantID tenant.ID, id, version string) (*spec.DomainSpec, error) {
	return s.repo.GetVersion(tenantID, id, version)
}

// List returns every registered domain for a tenant.
func (s *Service) List(tenantID tenant.ID) ([]*model.RegisteredDomain, error) {
	return s.repo.All(tenantID)
}

// Delete removes a domain by ID scoped to a tenant.
func (s *Service) Delete(tenantID tenant.ID, id string) error {
	return s.repo.Delete(tenantID, id)
}

// ValidateSpec runs the canonical loader validation without persisting.
func (s *Service) ValidateSpec(ds *spec.DomainSpec) error {
	if ds == nil {
		return model.ErrInvalidSpec
	}
	return spec.Validate(ds)
}
