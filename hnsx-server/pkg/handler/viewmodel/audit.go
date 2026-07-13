package viewmodel

import "time"

// AuditListItem is the canonical list view of a single audit entry.
type AuditListItem struct {
	ID           string         `json:"id"`
	SessionID    string         `json:"session_id"`
	DomainID     string         `json:"domain_id"`
	Action       string         `json:"action"`
	Actor        string         `json:"actor"`
	ActorType    string         `json:"actor_type"`
	Resource     string         `json:"resource"`
	ResourceType string         `json:"resource_type"`
	Decision     string         `json:"decision"`
	Reason       string         `json:"reason"`
	Details      map[string]any `json:"details,omitempty"`
	Timestamp    time.Time      `json:"timestamp"`
}

// AuditList is a paginated list of audit entries.
type AuditList struct {
	Items  []AuditListItem `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}
