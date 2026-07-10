// Package commands implements server-side application use cases. These
// commands operate on repository-backed services and are consumed by the HTTP
// API and gRPC control plane, not by the CLI.
package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/app"
	domainmodel "github.com/hnsx-io/hnsx/server/internal/domain/model"
	domainservice "github.com/hnsx-io/hnsx/server/internal/domain/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/local"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// DomainCommands exposes domain lifecycle use cases.
type DomainCommands struct {
	domainSvc *domainservice.Service
}

// NewDomainCommands constructs a DomainCommands backed by the supplied service.
func NewDomainCommands(domainSvc *domainservice.Service) *DomainCommands {
	return &DomainCommands{domainSvc: domainSvc}
}

// RegisterResult is returned by Register.
type RegisterResult struct {
	Domain    *app.RegisteredDomain
	CreatedAt time.Time
}

// Register parses a DomainSpec from a reader and registers it.
// Returns ErrDomainExists if the ID is already registered.
func (c *DomainCommands) Register(ctx context.Context, tenantID tenant.ID, r io.Reader, contentType string) (*RegisterResult, error) {
	if c.domainSvc == nil {
		return nil, errors.New("nil domain service")
	}
	s, err := local.DecodeDomainSpec(r, contentType)
	if err != nil {
		return nil, err
	}

	d, err := c.domainSvc.Register(s)
	if err != nil {
		if errors.Is(err, domainmodel.ErrDomainExists) {
			return nil, NewDomainExistsError(s.ID)
		}
		return nil, err
	}

	return &RegisterResult{
		Domain:    app.DomainFromModel(d),
		CreatedAt: d.CreatedAt,
	}, nil
}

// Update replaces the fields of an existing registered domain with the parsed
// body. Returns ErrDomainNotFound / ErrIDMismatch as appropriate.
func (c *DomainCommands) Update(ctx context.Context, tenantID tenant.ID, id string, r io.Reader, contentType string) (*app.RegisteredDomain, error) {
	if c.domainSvc == nil {
		return nil, errors.New("nil domain service")
	}
	s, err := local.DecodeDomainSpec(r, contentType)
	if err != nil {
		return nil, err
	}
	if s.ID != id {
		return nil, ErrIDMismatch
	}

	d, err := c.domainSvc.Update(id, s)
	if err != nil {
		if errors.Is(err, domainmodel.ErrDomainNotFound) {
			return nil, ErrDomainNotFound
		}
		return nil, err
	}
	return app.DomainFromModel(d), nil
}

// Delete removes a domain by ID.
func (c *DomainCommands) Delete(ctx context.Context, tenantID tenant.ID, id string) error {
	if c.domainSvc == nil {
		return errors.New("nil domain service")
	}
	if err := c.domainSvc.Delete(id); err != nil {
		if errors.Is(err, domainmodel.ErrDomainNotFound) {
			return ErrDomainNotFound
		}
		return err
	}
	return nil
}

// TriggerParams contains the inputs needed to start a session.
type TriggerParams struct {
	SessionID     string
	DomainID      string
	DomainVersion string
	Orchestration string
	Trigger       map[string]any
}

// NewSessionID generates a session ID for the given domain.
var NewSessionID = runtime.NewSessionID

// Error helpers used by HTTP handlers.
var (
	ErrDomainExists    = errors.New("domain already exists")
	ErrDomainNotFound  = errors.New("domain not found")
	ErrSessionExists   = errors.New("session already exists")
	ErrSessionNotFound = errors.New("session not found")
	ErrIDMismatch      = errors.New("domain id mismatch")
)

// NewDomainExistsError formats a domain-exists message.
func NewDomainExistsError(id string) error {
	return fmt.Errorf("domain %s already exists; use PUT to update", id)
}
