// Package service implements the policy application use cases.
//
// It wraps pkg/policy.Engine with domain policy loading and exposes the
// enforcement points used by the executor and tool gateway.
package service

import (
	"github.com/hnsx-io/hnsx/server/internal/policy/model"
	"github.com/hnsx-io/hnsx/server/internal/policy/repository"
	"github.com/hnsx-io/hnsx/server/pkg/policy"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// Service implements policy enforcement for a domain.
type Service struct {
	repo repository.Repository
}

// NewService constructs a Service backed by the supplied repository.
func NewService(repo repository.Repository) *Service {
	return &Service{repo: repo}
}

// LoadDomainPolicy persists the policy derived from a DomainSpec.
//
// Backward-compat path: Domain YAML's harness.policy is treated as a
// policy whose ID equals the domain_id and which is bound to the
// same domain. This preserves the existing LoadDomainPolicy contract
// callers (RegisterDomain → LoadDomainPolicy on validator/bootstrap)
// rely on, while letting the new /api/v1/policies endpoints also
// manage named policies independently.
func (s *Service) LoadDomainPolicy(domainID string, ds *spec.DomainSpec) error {
	if ds == nil {
		return nil
	}
	return s.repo.Save(&model.Policy{
		ID:   domainID,
		Name: domainID,
		Budget: model.Budget{
			MaxCostUSD: ds.Harness.Policy.Budget.MaxCostUSD,
			MaxTurns:   ds.Harness.Policy.Budget.MaxTurns,
			MaxTokens:  ds.Harness.Policy.Budget.MaxTokens,
		},
		Permissions: model.Permissions{
			AllowFileWrite:  ds.Harness.Policy.Permissions.AllowFileWrite,
			AllowFileDelete: ds.Harness.Policy.Permissions.AllowFileDelete,
			AllowNetwork:    ds.Harness.Policy.Permissions.AllowNetwork,
			AllowShell:      ds.Harness.Policy.Permissions.AllowShell,
		},
		Guardrails:  convertGuardrails(ds.Harness.Policy.Guardrails),
		BoundDomain: domainID,
	})
}

// List returns every policy registered with the service.
func (s *Service) List() ([]model.ListItem, error) {
	return s.repo.List()
}

// CreateOrUpdate idempotently persists the supplied policy.
func (s *Service) CreateOrUpdate(p *model.Policy) error {
	if p == nil || p.ID == "" {
		return model.ErrInvalidPolicyID
	}
	return s.repo.Save(p)
}

// Delete removes the policy by id.
func (s *Service) Delete(id string) error {
	return s.repo.Delete(id)
}

// BindDomain associates an existing policy with the named domain. If
// the domain is already bound to another policy, that binding is
// cleared so the 1:1 invariant holds.
func (s *Service) BindDomain(policyID, domainID string) error {
	return s.repo.BindDomain(policyID, domainID)
}

// SessionEngine returns a fresh, session-scoped policy.Engine for the named
// domain. The sessionID is reserved for future per-session policy caches; the
// current implementation returns a fresh engine per call.
func (s *Service) SessionEngine(domainID, sessionID string) (*policy.Engine, error) {
	return s.Engine(domainID)
}

// Engine returns a fresh policy.Engine for the named domain.
func (s *Service) Engine(domainID string) (*policy.Engine, error) {
	p, err := s.repo.ByDomain(domainID)
	if err != nil {
		// No policy configured -> permissive engine.
		return policy.NewEngine(spec.PolicySpec{}), nil
	}
	return policy.NewEngine(spec.PolicySpec{
		Budget: spec.BudgetSpec{
			MaxCostUSD: p.Budget.MaxCostUSD,
			MaxTurns:   p.Budget.MaxTurns,
			MaxTokens:  p.Budget.MaxTokens,
		},
		Permissions: spec.PermissionSpec{
			AllowFileWrite:  p.Permissions.AllowFileWrite,
			AllowFileDelete: p.Permissions.AllowFileDelete,
			AllowNetwork:    p.Permissions.AllowNetwork,
			AllowShell:      p.Permissions.AllowShell,
		},
		Guardrails: convertSpecGuardrails(p.Guardrails),
	}), nil
}

func convertGuardrails(in []spec.GuardrailSpec) []model.Guardrail {
	out := make([]model.Guardrail, 0, len(in))
	for _, g := range in {
		out = append(out, model.Guardrail{
			ID:      g.ID,
			Type:    g.Type,
			On:      g.On,
			Action:  g.Action,
			Schema:  g.Schema,
			Message: g.Message,
			Config:  g.Config,
		})
	}
	return out
}

func convertSpecGuardrails(in []model.Guardrail) []spec.GuardrailSpec {
	out := make([]spec.GuardrailSpec, 0, len(in))
	for _, g := range in {
		out = append(out, spec.GuardrailSpec{
			ID:      g.ID,
			Type:    g.Type,
			On:      g.On,
			Action:  g.Action,
			Schema:  g.Schema,
			Message: g.Message,
			Config:  g.Config,
		})
	}
	return out
}
