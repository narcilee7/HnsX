package auth

import (
	"fmt"
	"net/http"

	"github.com/hnsx-io/hnsx/server/internal/config"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

const apiKeyHeader = "X-HnsX-Api-Key"

// APIKeyAuthenticator validates static API keys configured in AuthConfig.
type APIKeyAuthenticator struct {
	cfg *config.AuthConfig
}

// NewAPIKeyAuthenticator constructs an API-key authenticator.
func NewAPIKeyAuthenticator(cfg *config.AuthConfig) *APIKeyAuthenticator {
	if cfg == nil {
		cfg = &config.AuthConfig{}
	}
	return &APIKeyAuthenticator{cfg: cfg}
}

// Authenticate implements Authenticator.
func (a *APIKeyAuthenticator) Authenticate(r *http.Request) (*Identity, error) {
	key := r.Header.Get(apiKeyHeader)
	if key == "" {
		return nil, fmt.Errorf("%w: missing %s header", ErrUnauthorized, apiKeyHeader)
	}
	entry, ok := a.cfg.APIKeys[key]
	if !ok {
		return nil, fmt.Errorf("%w: unknown API key", ErrForbidden)
	}
	tid := tenant.ID(entry.TenantID)
	if tid == "" {
		tid = tenant.DefaultID
	}
	role := normalizeRole(entry.Role)
	if role == "" {
		role = RoleOperator
	}
	return &Identity{TenantID: tid, Role: role}, nil
}
