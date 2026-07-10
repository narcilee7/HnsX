// Package model defines the AuditLog aggregate for the HnsX control plane.
package model

import (
	"errors"
	"time"
)

// Entry is a single immutable audit record.
type Entry struct {
	ID           string
	SessionID    string
	DomainID     string
	Action       string
	Actor        string
	ActorType    string
	Resource     string
	ResourceType string
	Decision     string
	Reason       string
	Details      map[string]any
	Timestamp    time.Time
}

// Validate returns an error if the entry is missing required fields.
func (e *Entry) Validate() error {
	if e.Action == "" {
		return errors.New("audit: action is required")
	}
	if e.Actor == "" {
		return errors.New("audit: actor is required")
	}
	if e.ActorType == "" {
		return errors.New("audit: actor_type is required")
	}
	return nil
}

// Common audit decisions.
const (
	DecisionAllow = "allow"
	DecisionDeny  = "deny"
	DecisionSkip  = "skip"
)

// Common actor types.
const (
	ActorTypeAgent = "agent"
	ActorTypeUser  = "user"
	ActorTypeSystem = "system"
)

// Common audit errors.
var (
	ErrAuditEntryNotFound = errors.New("audit: entry not found")
)
