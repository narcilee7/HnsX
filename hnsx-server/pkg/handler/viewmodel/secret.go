package viewmodel

import (
	"time"
)

// SecretListItem is the canonical list view of a stored secret.
// It intentionally omits any value/plaintext field.
type SecretListItem struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Kind        string    `json:"kind"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SecretList is a paginated list of secrets.
type SecretList struct {
	Items  []SecretListItem `json:"items"`
	Total  int              `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

// SecretDetail is the canonical detail view of a stored secret.
type SecretDetail struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Kind        string    `json:"kind"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SecretCreated is returned after a successful create.
type SecretCreated struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Fingerprint string `json:"fingerprint"`
}

// SecretUpdated is returned after a successful update.
type SecretUpdated struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Fingerprint string `json:"fingerprint"`
}
