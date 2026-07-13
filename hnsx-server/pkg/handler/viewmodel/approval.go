package viewmodel

import "time"

// ApprovalListItem is the canonical list view of an approval gate.
// It omits the Context map to keep list payloads small.
type ApprovalListItem struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	DomainID    string    `json:"domain_id"`
	Action      string    `json:"action"`
	Resource    string    `json:"resource"`
	RiskLevel   string    `json:"risk_level"`
	Status      string    `json:"status"`
	RequestedBy string    `json:"requested_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ApprovalList is the non-paginated list of approvals returned by the API.
type ApprovalList struct {
	Items []ApprovalListItem `json:"items"`
	Total int                `json:"total"`
}

// ApprovalDetail is the canonical detail view of an approval gate.
type ApprovalDetail struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"session_id"`
	DomainID    string         `json:"domain_id"`
	Action      string         `json:"action"`
	Resource    string         `json:"resource"`
	RiskLevel   string         `json:"risk_level"`
	Context     map[string]any `json:"context,omitempty"`
	Status      string         `json:"status"`
	RequestedBy string         `json:"requested_by"`
	ReviewedBy  string         `json:"reviewed_by,omitempty"`
	Comment     string         `json:"comment,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	ResolvedAt  *time.Time     `json:"resolved_at,omitempty"`
}

// ApprovalCreated is returned after a successful create/approve/reject.
// It carries the same shape as ApprovalDetail because the approval endpoints
// return the full record on mutation.
type ApprovalCreated struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"session_id"`
	DomainID    string         `json:"domain_id"`
	Action      string         `json:"action"`
	Resource    string         `json:"resource"`
	RiskLevel   string         `json:"risk_level"`
	Context     map[string]any `json:"context,omitempty"`
	Status      string         `json:"status"`
	RequestedBy string         `json:"requested_by"`
	ReviewedBy  string         `json:"reviewed_by,omitempty"`
	Comment     string         `json:"comment,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	ResolvedAt  *time.Time     `json:"resolved_at,omitempty"`
}
