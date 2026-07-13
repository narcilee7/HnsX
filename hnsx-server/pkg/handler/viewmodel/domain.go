package viewmodel

import (
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// DomainListItem is the canonical list view of a registered domain.
type DomainListItem struct {
	ID          string         `json:"id"`
	Version     string         `json:"version"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	Spec        *domain.DomainSpec `json:"-"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// DomainList is a paginated list of domains.
type DomainList struct {
	Items  []DomainListItem `json:"items"`
	Total  int              `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

// DomainDetail is the canonical detail view of a registered domain.
type DomainDetail struct {
	ID          string         `json:"id"`
	Version     string         `json:"version"`
	Description string         `json:"description"`
	Harness     any            `json:"harness,omitempty"`
	Spec        *domain.DomainSpec `json:"-"`
	Status      string         `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// DomainVersionItem is a single stored version of a domain.
type DomainVersionItem struct {
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	IsCurrent bool      `json:"is_current"`
}

// DomainVersionList is a paginated list of domain versions.
type DomainVersionList struct {
	Items  []DomainVersionItem `json:"items"`
	Total  int                 `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

// DomainSchemaView exposes the workspace-oriented schema of a domain.
type DomainSchemaView struct {
	ID            string `json:"id"`
	Version       string `json:"version"`
	Mode          string `json:"mode"`
	Agent         string `json:"agent"`
	TriggerSchema any    `json:"trigger_schema,omitempty"`
	OutputSchema  string `json:"output_schema,omitempty"`
}

// DomainRegistered is returned after a successful register/update.
type DomainRegistered struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// DomainValidationSummary is returned by the validate endpoint.
type DomainValidationSummary struct {
	Valid      bool   `json:"valid"`
	ID         string `json:"id"`
	Version    string `json:"version"`
	Mode       string `json:"mode"`
	AgentCount int    `json:"agent_count"`
	StepCount  int    `json:"step_count"`
	TenantID   string `json:"tenant_id,omitempty"`
}
