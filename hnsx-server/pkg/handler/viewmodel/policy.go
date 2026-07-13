package viewmodel

import (
	"time"

	policymodel "github.com/hnsx-io/hnsx/server/internal/policy/model"
)

// PolicyListItem is the canonical list view of a named policy.
type PolicyListItem struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	BoundDomain string                  `json:"bound_domain,omitempty"`
	Budget      policymodel.Budget      `json:"budget"`
	Permissions policymodel.Permissions `json:"permissions"`
	Guardrails  []policymodel.Guardrail `json:"guardrails"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

// PolicyList is a paginated list of policies.
type PolicyList struct {
	Items  []PolicyListItem `json:"items"`
	Total  int              `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

// PolicyDetail is the canonical detail view of a named policy.
type PolicyDetail struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	BoundDomain string                  `json:"bound_domain,omitempty"`
	Budget      policymodel.Budget      `json:"budget"`
	Permissions policymodel.Permissions `json:"permissions"`
	Guardrails  []policymodel.Guardrail `json:"guardrails"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

// PolicyBound is returned after a successful bind or unbind operation.
type PolicyBound struct {
	DomainID string `json:"domain_id"`
	PolicyID string `json:"policy_id"`
}
