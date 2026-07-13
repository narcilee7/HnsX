package handler

import (
	"context"
	"errors"
	"sort"

	"go.uber.org/zap"

	policymodel "github.com/hnsx-io/hnsx/server/internal/policy/model"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
)

// ---------------------------------------------------------------------------
// Re-exported errors
// ---------------------------------------------------------------------------

var (
	// ErrPolicyNotFound is re-exported from the policy model for HTTP/gRPC comparators.
	ErrPolicyNotFound = policymodel.ErrPolicyNotFound
	// ErrInvalidPolicyID is re-exported from the policy model for validation comparators.
	ErrInvalidPolicyID = policymodel.ErrInvalidPolicyID
)

// Handler-level validation errors.
var (
	ErrPolicyIDRequired = errors.New("policy: id is required")
	ErrPolicyIDMismatch = errors.New("policy: id in body must match id in url")
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

type ListPoliciesInput struct{}

type GetPolicyInput struct {
	ID string
}

type CreatePolicyInput struct {
	ID          string
	Name        string
	Description string
	Budget      policymodel.Budget
	Permissions policymodel.Permissions
	Guardrails  []policymodel.Guardrail
}

type UpdatePolicyInput struct {
	ID          string
	BodyID      string
	Name        string
	Description string
	Budget      policymodel.Budget
	Permissions policymodel.Permissions
	Guardrails  []policymodel.Guardrail
}

type DeletePolicyInput struct {
	ID string
}

type BindPolicyInput struct {
	DomainID string
	PolicyID string
}

type UnbindPolicyInput struct {
	DomainID string
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

type ListPoliciesOutput struct {
	Policies viewmodel.PolicyList
}

type GetPolicyOutput struct {
	Policy *viewmodel.PolicyDetail
}

type CreatePolicyOutput struct {
	Policy *viewmodel.PolicyDetail
}

type UpdatePolicyOutput struct {
	Policy *viewmodel.PolicyDetail
}

type BindPolicyOutput struct {
	Bound *viewmodel.PolicyBound
}

type UnbindPolicyOutput struct {
	Bound *viewmodel.PolicyBound
}

// ---------------------------------------------------------------------------
// Handler methods
// ---------------------------------------------------------------------------

// ListPolicies returns every named policy registered with the service.
func (h *Handler) ListPolicies(ctx context.Context, in ListPoliciesInput) (*ListPoliciesOutput, error) {
	defer h.hook(ctx, "policy.list")()

	if h.App == nil || h.App.PolicyService == nil {
		return nil, ErrPolicyNotFound
	}
	items, err := h.App.PolicyService.List()
	if err != nil {
		return nil, err
	}

	out := make([]viewmodel.PolicyListItem, 0, len(items))
	for _, it := range items {
		out = append(out, viewmodel.PolicyListItem{
			ID:          it.ID,
			Name:        it.Name,
			Description: it.Description,
			BoundDomain: it.BoundDomain,
			Budget:      it.Budget,
			Permissions: it.Permissions,
			Guardrails:  it.Guardrails,
			CreatedAt:   it.CreatedAt,
			UpdatedAt:   it.UpdatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	return &ListPoliciesOutput{Policies: viewmodel.PolicyList{
		Items:  out,
		Total:  len(out),
		Limit:  len(out),
		Offset: 0,
	}}, nil
}

// GetPolicy returns a single policy detail by id.
func (h *Handler) GetPolicy(ctx context.Context, in GetPolicyInput) (*GetPolicyOutput, error) {
	defer h.hook(ctx, "policy.get", zap.String("id", in.ID))()

	if h.App == nil || h.App.PolicyService == nil {
		return nil, ErrPolicyNotFound
	}
	items, err := h.App.PolicyService.List()
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.ID == in.ID {
			return &GetPolicyOutput{Policy: toPolicyDetail(&policymodel.Policy{
				ID:          it.ID,
				Name:        it.Name,
				Description: it.Description,
				BoundDomain: it.BoundDomain,
				Budget:      it.Budget,
				Permissions: it.Permissions,
				Guardrails:  it.Guardrails,
				CreatedAt:   it.CreatedAt,
				UpdatedAt:   it.UpdatedAt,
			})}, nil
		}
	}
	return nil, ErrPolicyNotFound
}

// CreatePolicy persists a new named policy.
func (h *Handler) CreatePolicy(ctx context.Context, in CreatePolicyInput) (*CreatePolicyOutput, error) {
	defer h.hook(ctx, "policy.create", zap.String("id", in.ID))()

	if h.App == nil || h.App.PolicyService == nil {
		return nil, ErrPolicyNotFound
	}
	if in.ID == "" {
		return nil, ErrPolicyIDRequired
	}
	name := in.Name
	if name == "" {
		name = in.ID
	}
	p := &policymodel.Policy{
		ID:          in.ID,
		Name:        name,
		Description: in.Description,
		Budget:      in.Budget,
		Permissions: in.Permissions,
		Guardrails:  in.Guardrails,
	}
	if err := h.App.PolicyService.CreateOrUpdate(p); err != nil {
		return nil, err
	}
	return &CreatePolicyOutput{Policy: toPolicyDetail(p)}, nil
}

// UpdatePolicy replaces an existing named policy.
func (h *Handler) UpdatePolicy(ctx context.Context, in UpdatePolicyInput) (*UpdatePolicyOutput, error) {
	defer h.hook(ctx, "policy.update", zap.String("id", in.ID))()

	if h.App == nil || h.App.PolicyService == nil {
		return nil, ErrPolicyNotFound
	}
	if in.BodyID != "" && in.BodyID != in.ID {
		return nil, ErrPolicyIDMismatch
	}
	name := in.Name
	if name == "" {
		name = in.ID
	}
	p := &policymodel.Policy{
		ID:          in.ID,
		Name:        name,
		Description: in.Description,
		Budget:      in.Budget,
		Permissions: in.Permissions,
		Guardrails:  in.Guardrails,
	}
	if err := h.App.PolicyService.CreateOrUpdate(p); err != nil {
		return nil, err
	}
	return &UpdatePolicyOutput{Policy: toPolicyDetail(p)}, nil
}

// DeletePolicy removes a policy by id.
func (h *Handler) DeletePolicy(ctx context.Context, in DeletePolicyInput) error {
	defer h.hook(ctx, "policy.delete", zap.String("id", in.ID))()

	if h.App == nil || h.App.PolicyService == nil {
		return ErrPolicyNotFound
	}
	return h.App.PolicyService.Delete(in.ID)
}

// BindPolicy associates an existing policy with a domain, enforcing a 1:1 binding.
func (h *Handler) BindPolicy(ctx context.Context, in BindPolicyInput) (*BindPolicyOutput, error) {
	defer h.hook(ctx, "policy.bind",
		zap.String("domain_id", in.DomainID),
		zap.String("policy_id", in.PolicyID),
	)()

	if h.App == nil || h.App.PolicyService == nil {
		return nil, ErrPolicyNotFound
	}
	if in.PolicyID == "" {
		return nil, ErrPolicyIDRequired
	}
	if err := h.App.PolicyService.BindDomain(in.PolicyID, in.DomainID); err != nil {
		return nil, err
	}
	return &BindPolicyOutput{Bound: &viewmodel.PolicyBound{
		DomainID: in.DomainID,
		PolicyID: in.PolicyID,
	}}, nil
}

// UnbindPolicy removes whichever policy is currently bound to the domain.
func (h *Handler) UnbindPolicy(ctx context.Context, in UnbindPolicyInput) (*UnbindPolicyOutput, error) {
	defer h.hook(ctx, "policy.unbind", zap.String("domain_id", in.DomainID))()

	if h.App == nil || h.App.PolicyService == nil {
		return nil, ErrPolicyNotFound
	}
	items, err := h.App.PolicyService.List()
	if err != nil {
		return nil, err
	}
	var policyID string
	for _, it := range items {
		if it.BoundDomain == in.DomainID {
			policyID = it.ID
			break
		}
	}
	if policyID == "" {
		return nil, ErrPolicyNotFound
	}
	if err := h.App.PolicyService.BindDomain(policyID, ""); err != nil {
		return nil, err
	}
	return &UnbindPolicyOutput{Bound: &viewmodel.PolicyBound{
		DomainID: in.DomainID,
		PolicyID: "",
	}}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toPolicyDetail(p *policymodel.Policy) *viewmodel.PolicyDetail {
	if p == nil {
		return nil
	}
	return &viewmodel.PolicyDetail{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		BoundDomain: p.BoundDomain,
		Budget:      p.Budget,
		Permissions: p.Permissions,
		Guardrails:  p.Guardrails,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// IsPolicyNotFound reports whether err is a policy-not-found error.
func IsPolicyNotFound(err error) bool {
	return errors.Is(err, policymodel.ErrPolicyNotFound)
}

// IsInvalidPolicyID reports whether err is an invalid-policy-id error.
func IsInvalidPolicyID(err error) bool {
	return errors.Is(err, policymodel.ErrInvalidPolicyID) || errors.Is(err, ErrPolicyIDRequired) || errors.Is(err, ErrPolicyIDMismatch)
}

// mapPolicyError normalizes policy errors for transport-agnostic callers.
func mapPolicyError(err error) error {
	if err == nil {
		return nil
	}
	if IsPolicyNotFound(err) {
		return ErrPolicyNotFound
	}
	return err
}
